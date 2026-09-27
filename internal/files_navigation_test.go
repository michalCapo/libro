package libro

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNavigationLocations(t *testing.T) {
	t.Cleanup(closeNavigationServers)
	root := t.TempDir()
	project := filepath.Join(root, "project")
	if err := os.Mkdir(project, 0700); err != nil {
		t.Fatal(err)
	}
	local := filepath.Join(project, "source with space.go")
	dependency := filepath.Join(root, "dependency.go")
	for _, path := range []string{local, dependency} {
		if err := os.WriteFile(path, []byte("package p\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	data, err := json.Marshal([]any{
		map[string]any{"uri": fileNavigationURI(local), "range": map[string]any{"start": map[string]int{"line": 3, "character": 7}}},
		map[string]any{"targetUri": fileNavigationURI(dependency), "targetSelectionRange": map[string]any{"start": map[string]int{"line": 2, "character": 4}}},
		map[string]any{"uri": "https://example.com/source", "range": map[string]any{"start": map[string]int{"line": 1}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := navigationLocations(project, data)
	if err != nil || len(result.Matches) != 2 {
		t.Fatalf("locations: %+v %v", result, err)
	}
	if result.Matches[0].Path != "source with space.go" || result.Matches[0].Line != 4 || result.Matches[0].Column != 7 {
		t.Fatalf("local: %+v", result.Matches[0])
	}
	if result.Matches[1].Parents != 1 || result.Matches[1].Path != "dependency.go" {
		t.Fatalf("external: %+v", result.Matches[1])
	}
	single, err := navigationLocations(project, json.RawMessage(`{"uri":`+mustJSON(t, fileNavigationURI(local))+`,"range":{"start":{"line":0,"character":0}}}`))
	if err != nil || len(single.Matches) != 1 {
		t.Fatalf("single location: %+v %v", single, err)
	}
}
func mustJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestGoFileNavigation(t *testing.T) {
	t.Cleanup(closeNavigationServers)
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not installed")
	}
	root := t.TempDir()
	for name, text := range map[string]string{"go.mod": "module example.com/navigation\n\ngo 1.26\n", "definition.go": "package navigation\nfunc Target() {}\n", "use.go": "package navigation\nfunc Use() { Target() }\n"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	result, err := navigateProjectFile(root, "use.go", "definition", "", 0, 2, 13)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Matches) != 1 || result.Matches[0].Path != "definition.go" || result.Matches[0].Line != 2 {
		t.Fatalf("definition: %+v", result)
	}
	result, err = navigateProjectFile(root, "definition.go", "references", "", 0, 2, 6)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Matches) != 2 {
		t.Fatalf("references: %+v", result)
	}
	if _, err = navigateProjectFile(root, "../outside.go", "definition", "", 0, 1, 0); err == nil {
		t.Fatal("accepted path traversal")
	}
	if _, err = navigateProjectFile(root, "use.go", "definition", "stale", 0, 2, 13); err == nil {
		t.Fatal("accepted stale preview")
	}
}

func TestOtherLanguageNavigation(t *testing.T) {
	t.Cleanup(closeNavigationServers)
	cases := []struct {
		name, server, file, source string
		line, column               int
		wantLine                   int
	}{
		{"typescript", "typescript-language-server", "source.ts", "function target() {}\ntarget();\n", 2, 1, 1},
		{"cpp", "clangd", "source.cpp", "void target() {}\nint main() { target(); }\n", 2, 14, 1},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			if _, err := exec.LookPath(item.server); err != nil {
				t.Skip(item.server + " not installed")
			}
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, item.file), []byte(item.source), 0600); err != nil {
				t.Fatal(err)
			}
			result, err := navigateProjectFile(root, item.file, "definition", "", 0, item.line, item.column)
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Matches) != 1 || result.Matches[0].Path != item.file || result.Matches[0].Line != item.wantLine {
				t.Fatalf("definition: %+v", result)
			}
		})
	}
}

func TestGoImplementationAndTypeNavigation(t *testing.T) {
	t.Cleanup(closeNavigationServers)
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not installed")
	}
	root := t.TempDir()
	source := "package navigation\ntype Runner interface { Run() }\ntype Worker struct{}\nfunc (Worker) Run() {}\nfunc Use(r Runner) { r.Run() }\n"
	for name, text := range map[string]string{"go.mod": "module example.com/navigation\n\ngo 1.26\n", "source.go": source} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	result, err := navigateProjectFile(root, "source.go", "implementation", "", 0, 2, strings.Index(strings.Split(source, "\n")[1], "Run()"))
	if err != nil || len(result.Matches) != 1 || result.Matches[0].Line != 4 {
		t.Fatalf("implementation: %+v %v", result, err)
	}
	result, err = navigateProjectFile(root, "source.go", "typeDefinition", "", 0, 5, strings.Index(strings.Split(source, "\n")[4], "r.Run()"))
	if err != nil || len(result.Matches) != 1 || result.Matches[0].Line != 2 {
		t.Fatalf("type definition: %+v %v", result, err)
	}
}

func TestNavigationTimeout(t *testing.T) {
	t.Cleanup(closeNavigationServers)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	_, err = requestFileNavigation(ctx, navigationLanguage{command: executable, args: []string{"-test.run=TestNavigationHangingServer", "--", "navigation-hang"}}, t.TempDir(), "source.go", "package p", "textDocument/definition", 1, 0)
	if err == nil || ctx.Err() == nil {
		t.Fatalf("expected bounded timeout, got %v", err)
	}
}
func TestNavigationHangingServer(_ *testing.T) {
	if os.Args[len(os.Args)-1] != "navigation-hang" {
		return
	}
	time.Sleep(time.Minute)
	os.Exit(0)
}

func TestNavigationFromSymlinkedProject(t *testing.T) {
	t.Cleanup(closeNavigationServers)
	root := t.TempDir()
	project := filepath.Join(root, "actual", "project")
	if err := os.MkdirAll(project, 0700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(project, alias); err != nil {
		t.Skip("symlinks unavailable")
	}
	dependency := filepath.Join(root, "actual", "dependency.go")
	if err := os.WriteFile(dependency, []byte("package p"), 0600); err != nil {
		t.Fatal(err)
	}
	raw := json.RawMessage(`{"uri":` + mustJSON(t, fileNavigationURI(dependency)) + `,"range":{"start":{"line":0,"character":0}}}`)
	result, err := navigationLocations(alias, raw)
	if err != nil || len(result.Matches) != 1 {
		t.Fatalf("locations: %+v %v", result, err)
	}
	location := result.Matches[0]
	opened, err := projectFileToOpen(filesRoot(alias, location.Parents), location.Path)
	if err != nil || opened != dependency {
		t.Fatalf("target resolves incorrectly: %s %v", opened, err)
	}
}

func TestTypeScriptImportedMethodNavigation(t *testing.T) {
	t.Cleanup(closeNavigationServers)
	if _, err := exec.LookPath("typescript-language-server"); err != nil {
		t.Skip("typescript-language-server not installed")
	}
	root := t.TempDir()
	for path, content := range map[string]string{
		"tsconfig.json": `{"compilerOptions":{"allowImportingTsExtensions":true,"noEmit":true}}`,
		"app.ts":        "export class App {\n  admin() { return this; }\n  link() { return this; }\n}\n",
		"dev.ts":        "import { App } from './app.ts';\nnew App().admin().link();\n",
	} {
		if err := os.WriteFile(filepath.Join(root, path), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, kind := range []string{"definition", "declaration", "references", "implementation", "typeDefinition"} {
		t.Run(kind, func(t *testing.T) {
			result, err := navigateProjectFile(root, "dev.ts", kind, "", 0, 2, 18)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, match := range result.Matches {
				if match.Path == "app.ts" && match.Line == 3 {
					found = true
				}
			}
			if !found {
				t.Fatalf("missing imported method: %+v", result.Matches)
			}
			if kind == "references" && len(result.Matches) != 2 {
				t.Fatalf("references: %+v", result.Matches)
			}
		})
	}
}

func TestNavigationIncludesSourceContext(t *testing.T) {
	root := t.TempDir()
	source := "// before\nexport const target = 1;\n// after\n"
	path := filepath.Join(root, "source.ts")
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	raw := json.RawMessage(`{"uri":` + mustJSON(t, fileNavigationURI(path)) + `,"range":{"start":{"line":1,"character":13}}}`)
	result, err := navigationLocations(root, raw)
	if err != nil || len(result.Matches) != 1 {
		t.Fatalf("context: %+v %v", result, err)
	}
	if result.Matches[0].StartLine != 1 || result.Matches[0].Context != source {
		t.Fatalf("wrong context: %+v", result.Matches[0])
	}
}
