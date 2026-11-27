package libro

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"html"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	r "github.com/michalCapo/g-sui/ui"
	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// A thread's task is the full copy of the email, card or issue the user linked.
// The agent downloads it; Libro keeps it with its images, because shared links
// expire, and shows it in a Browser panel beside the agent.

const (
	maxTaskFile   = 20 << 20
	maxTaskImage  = 10 << 20
	maxTaskImages = 60 << 20 // downloaded, and embedded with every use counted
	maxTaskFetch  = 200
	// The agent's control request gives up after 30 seconds.
	taskImageTime = 15 * time.Second
)

// The copy loads nothing from the network and runs no scripts.
const taskPolicy = "default-src 'none'; img-src data:; style-src 'unsafe-inline'; font-src data:; form-action 'none'"

// Threads are keyed by ID, checkouts and worktrees by path, like their description.
func threadTaskKey(state *AppState, workspace string) string {
	if thread := state.thread(workspace); thread != nil {
		return "thread:" + thread.ID
	}
	if _, path := threadProjectContext(state, workspace); path != "" {
		return applicationPath(path)
	}
	return ""
}

func threadTaskPath(key string) (string, error) {
	dir, err := libroDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "tasks", fmt.Sprintf("%x", sha256.Sum256([]byte(key)))[:32], "task.html"), nil
}

// savedThreadTask returns the task file of a key, or "" when none is saved.
func savedThreadTask(key string) string {
	if key == "" {
		return ""
	}
	path, err := threadTaskPath(key)
	if err != nil {
		return ""
	}
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	return path
}

func removeThreadTask(key string) {
	if path, err := threadTaskPath(key); err == nil && key != "" {
		_ = os.RemoveAll(filepath.Dir(path))
	}
}

type threadTask struct {
	page   []byte
	images int
	failed int
}

// prepareThreadTask reads the agent's file and builds the page to keep. It runs
// before the backend lock because it downloads images.
func prepareThreadTask(ctx context.Context, file, source string) (*threadTask, error) {
	if file == "" || !filepath.IsAbs(file) {
		return nil, errors.New("task needs file: the absolute path of the downloaded email, card or issue")
	}
	var base *url.URL
	if source != "" {
		parsed, err := url.Parse(source)
		if err != nil || parsed.Scheme != "http" && parsed.Scheme != "https" || parsed.Host == "" {
			return nil, errors.New("url must be the http or https link the user gave")
		}
		base = parsed
	}
	data, err := readTaskFile(file)
	if err != nil {
		return nil, err
	}
	ext := strings.ToLower(filepath.Ext(file))
	if ext == ".html" || ext == ".htm" || strings.HasPrefix(http.DetectContentType(data), "text/html") {
		return taskPage(ctx, data, base)
	}
	if !utf8.Valid(data) {
		return nil, errors.New("task file must be HTML or UTF-8 text")
	}
	page := `<!doctype html><html><head><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="` + taskPolicy + `"><title>Task</title></head>` +
		`<body style="margin:0;padding:16px 20px;font:14px/1.55 system-ui,sans-serif">` + taskSource(base) +
		`<pre style="margin:0;white-space:pre-wrap;word-wrap:break-word;font:inherit">` + html.EscapeString(string(data)) + `</pre></body></html>`
	return &threadTask{page: []byte(page)}, nil
}

// readTaskFile reads one regular file, checked on the opened handle.
func readTaskFile(file string) ([]byte, error) {
	if info, err := os.Lstat(file); err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("task file must be a regular file")
	}
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	if info, err := f.Stat(); err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("task file must be a regular file")
	}
	data, err := io.ReadAll(io.LimitReader(f, maxTaskFile+1))
	if err == nil && len(data) > maxTaskFile {
		err = fmt.Errorf("task file must be at most %d MB", maxTaskFile>>20)
	}
	return data, err
}

func taskSource(base *url.URL) string {
	line := "Saved " + time.Now().Format("2006-01-02 15:04")
	if base != nil {
		link := html.EscapeString(base.String())
		line = `<a href="` + link + `" style="color:inherit">` + link + `</a> · ` + line
	}
	return `<div style="margin:0 0 12px;padding:6px 10px;border-radius:6px;background:#8881;color:#777;font:12px/1.4 system-ui,sans-serif;overflow-wrap:anywhere">` + line + `</div>`
}

// Elements that run code, load other pages or change where links lead.
var taskDropped = map[atom.Atom]bool{atom.Script: true, atom.Noscript: true, atom.Iframe: true, atom.Frame: true, atom.Frameset: true, atom.Object: true, atom.Embed: true, atom.Applet: true, atom.Base: true, atom.Link: true}

// taskPage removes scripts and embeds the page's images, so the copy shows
// the same after its links expire.
func taskPage(ctx context.Context, data []byte, base *url.URL) (*threadTask, error) {
	doc, err := xhtml.Parse(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	type imageRef struct {
		node *xhtml.Node
		attr int
	}
	images := map[string][]imageRef{}
	var head, body *xhtml.Node
	var walk func(*xhtml.Node)
	walk = func(node *xhtml.Node) {
		for child := node.FirstChild; child != nil; {
			next := child.NextSibling
			if child.Type == xhtml.ElementNode && (taskDropped[child.DataAtom] || child.DataAtom == atom.Meta && taskAttr(child, "http-equiv") != "") {
				node.RemoveChild(child)
				child = next
				continue
			}
			if child.Type == xhtml.ElementNode {
				switch child.DataAtom {
				case atom.Head:
					head = child
				case atom.Body:
					body = child
				}
				// An image given only by srcset keeps its first candidate.
				if child.DataAtom == atom.Img && taskAttr(child, "src") == "" {
					if fields := strings.Fields(taskAttr(child, "srcset")); len(fields) > 0 {
						child.Attr = append(child.Attr, xhtml.Attribute{Key: "src", Val: strings.TrimSuffix(fields[0], ",")})
					}
				}
				attrs := child.Attr[:0]
				for _, attr := range child.Attr {
					value := strings.ToLower(strings.TrimSpace(attr.Val))
					if strings.HasPrefix(attr.Key, "on") || attr.Key == "srcset" || strings.HasPrefix(value, "javascript:") {
						continue
					}
					// The copy is a local file, so links lead only to the web or within the page.
					if attr.Key == "href" && (child.DataAtom == atom.A || child.DataAtom == atom.Area) {
						if attr.Val = taskLink(attr.Val, base); attr.Val == "" {
							continue
						}
					}
					attrs = append(attrs, attr)
				}
				child.Attr = attrs
				for index, attr := range child.Attr {
					if child.DataAtom == atom.Img && attr.Key == "src" || attr.Key == "background" {
						if address := taskImageURL(attr.Val, base); address != "" {
							images[address] = append(images[address], imageRef{child, index})
						}
					}
				}
			}
			walk(child)
			child = next
		}
	}
	walk(doc)
	task := &threadTask{images: len(images)}
	addresses := make([]string, 0, len(images))
	for address := range images {
		addresses = append(addresses, address)
	}
	embedded := fetchTaskImages(ctx, addresses, base)
	var size int
	for address, refs := range images {
		uri, ok := embedded[address]
		if size += len(uri) * len(refs); !ok || size > maxTaskImages*4/3 {
			task.failed++
			continue
		}
		for _, ref := range refs {
			ref.node.Attr[ref.attr].Val = uri
		}
	}
	if head != nil {
		// Inserted at the start in reverse, so the charset comes first.
		for _, meta := range []string{`<meta http-equiv="Content-Security-Policy" content="` + taskPolicy + `">`, `<meta charset="utf-8">`} {
			nodes, err := xhtml.ParseFragment(strings.NewReader(meta), head)
			if err == nil && len(nodes) == 1 {
				head.InsertBefore(nodes[0], head.FirstChild)
			}
		}
	}
	if body != nil {
		if nodes, err := xhtml.ParseFragment(strings.NewReader(taskSource(base)), body); err == nil && len(nodes) == 1 {
			body.InsertBefore(nodes[0], body.FirstChild)
		}
	}
	var out bytes.Buffer
	if err := xhtml.Render(&out, doc); err != nil {
		return nil, err
	}
	task.page = out.Bytes()
	return task, nil
}

func taskAttr(node *xhtml.Node, key string) string {
	for _, attr := range node.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

// taskLink keeps fragments and web or mail links, resolved against the source.
func taskLink(value string, base *url.URL) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "#") {
		return value
	}
	address, err := url.Parse(value)
	if err != nil {
		return ""
	}
	if base != nil {
		address = base.ResolveReference(address)
	}
	if address.Scheme == "mailto" || (address.Scheme == "http" || address.Scheme == "https") && address.Host != "" {
		return address.String()
	}
	return ""
}

// taskImageURL resolves an image address. Embedded images stay as they are.
func taskImageURL(value string, base *url.URL) string {
	address, err := url.Parse(strings.TrimSpace(value))
	if err != nil || value == "" {
		return ""
	}
	if base != nil {
		address = base.ResolveReference(address)
	}
	if address.Scheme != "http" && address.Scheme != "https" || address.Host == "" {
		return ""
	}
	return address.String()
}

// fetchTaskImages downloads each image once, four at a time, within one time
// and size limit, and returns data URIs for those that arrived.
func fetchTaskImages(ctx context.Context, addresses []string, base *url.URL) map[string]string {
	ctx, cancel := context.WithTimeout(ctx, taskImageTime)
	defer cancel()
	client := &http.Client{Transport: taskTransport(base)}
	var mu sync.Mutex
	var total int
	result := map[string]string{}
	queue := make(chan string)
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for address := range queue {
				mu.Lock()
				full := total > maxTaskImages
				mu.Unlock()
				if full || ctx.Err() != nil {
					continue
				}
				data, kind := fetchTaskImage(ctx, client, address)
				mu.Lock()
				if total += len(data); data != nil && total <= maxTaskImages {
					result[address] = "data:" + kind + ";base64," + base64.StdEncoding.EncodeToString(data)
				}
				mu.Unlock()
			}
		})
	}
	for _, address := range addresses[:min(len(addresses), maxTaskFetch)] {
		queue <- address
	}
	close(queue)
	wg.Wait()
	return result
}

// taskTransport reaches local and private addresses only on the task's own
// host, so a page cannot make Libro call other services on this network.
func taskTransport(base *url.URL) *http.Transport {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	transport := &http.Transport{Proxy: nil, TLSHandshakeTimeout: 5 * time.Second}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil || len(ips) == 0 {
			return nil, fmt.Errorf("cannot resolve %s", host)
		}
		ip := ips[0].IP
		trusted := base != nil && strings.EqualFold(base.Hostname(), host)
		if !trusted && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast()) {
			return nil, fmt.Errorf("image address %s is on a local network", host)
		}
		return dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
	}
	return transport
}

func fetchTaskImage(ctx context.Context, client *http.Client, address string) ([]byte, string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, ""
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, ""
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, ""
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxTaskImage+1))
	if err != nil || len(data) > maxTaskImage {
		return nil, ""
	}
	kind, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if !strings.HasPrefix(kind, "image/") {
		kind = http.DetectContentType(data)
	}
	if !strings.HasPrefix(kind, "image/") {
		return nil, ""
	}
	return data, kind
}

// saveThreadTask keeps the prepared page for the calling agent's workspace and
// shows it beside the agent.
func saveThreadTask(sid, panel string, task *threadTask) (r.Result, any, error) {
	if task == nil {
		return r.Result{}, nil, errors.New("the task was not prepared")
	}
	if panel == "" {
		return r.Result{}, nil, errors.New("only an agent panel started by Libro can save a task")
	}
	_, workspace, _, found := sm.workspaceApp(sid, panel)
	if !found {
		return r.Result{}, nil, errors.New("this agent panel is no longer open")
	}
	key := threadTaskKey(sm.Get(sid), workspace)
	if key == "" {
		return r.Result{}, nil, errors.New("this workspace cannot keep a task")
	}
	path, err := threadTaskPath(key)
	if err == nil {
		err = os.MkdirAll(filepath.Dir(path), 0o700)
	}
	if err == nil {
		err = writeTaskFile(path, task.page)
	}
	if err != nil {
		return r.Result{}, nil, err
	}
	message := "Task saved"
	if task.failed > 0 {
		message += fmt.Sprintf("; %d of %d images could not be downloaded", task.failed, task.images)
	}
	// Every agent panel of the thread shows the Task button.
	var panels []string
	for _, app := range sm.Get(sid).workspaceApps(workspace) {
		panels = append(panels, app.ID)
	}
	js := clientScript("props[0].forEach(function(id){window.libroPrompts?.setTask(id,true);});", panels)
	opened, _, err := openThreadTask(sid, panel, path)
	if err != nil {
		message += ". It shows from the Task button above the agent: " + err.Error()
	}
	return r.Merge(js, opened), map[string]string{"result": message}, nil
}

func writeTaskFile(path string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".task-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(file.Name()) }()
	_, err = file.Write(data)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

// openThreadTask shows the task; the query makes a reused panel load the new copy.
func openThreadTask(sid, panel, path string) (r.Result, any, error) {
	return openFilePanel(sid, panel, fileURL(path)+"?saved="+fmt.Sprint(time.Now().UnixMilli()))
}

type actionThreadTaskInput struct {
	SID  string `json:"sid"`
	ID   string `json:"id"`
	Open bool   `json:"open"`
}

// The strip above an agent asks whether its thread has a task and opens it.
func registerThreadTask(app *r.App) {
	registerWorkspaceAction(app, "thread.task", func(_ *r.Context, in actionThreadTaskInput) (r.Result, error) {
		sid := inputSID(in.SID)
		_, workspace, _, found := sm.workspaceApp(sid, in.ID)
		if !found {
			return r.Result{}, nil
		}
		path := savedThreadTask(threadTaskKey(sm.Get(sid), workspace))
		js := clientScript("window.libroPrompts?.setTask(props[0],props[1]);", in.ID, path != "")
		if path == "" || !in.Open {
			return js, nil
		}
		opened, _, err := openThreadTask(sid, in.ID, path)
		if err != nil {
			return r.Merge(js, r.Result{}.Run(r.Notify("error", "Could not open the task: "+err.Error()))), nil //nolint:nilerr // the error is shown to the user
		}
		return r.Merge(js, opened), nil
	})
}
