package components

import (
	"reflect"
	"strings"
	"testing"
)

func TestTerminalLocalURLs(t *testing.T) {
	tm := NewTerminalManager()
	log := &terminalLog{}
	tm.logs["app"] = log
	for _, chunk := range []string{
		"LINK: \x1b[32mhttp://local",
		"host:43655/link/\x1b[0m\r\nADMIN: http://localhost:43657/\r\n",
		"FLOW: http://0.0.0.0:43656/\n",
		"Again: http://127.0.0.1:43655/link/\nIPv6: https://[::1]:8443/view\n",
		"Request: http://localhost:43655/static/icons/mail.svg\n",
		"Ignore: https://example.com:1234/ http://localhost:0/ http://localhost:65536/ http://localhost:3000.evil/\n",
		"Last: http://localhost:5000",
	} {
		log.append([]byte(chunk))
	}
	want := []string{"http://localhost:43655/link/", "http://localhost:43657/", "http://localhost:43656/", "https://localhost:8443/view", "http://localhost:5000"}
	if got := tm.LocalURLs("app"); !reflect.DeepEqual(got, want) {
		t.Fatalf("URLs = %v, want %v", got, want)
	}
	// A query must not save an incomplete URL from a split output chunk.
	log.append([]byte("/ready\n"))
	want[len(want)-1] += "/ready"
	log.append([]byte(strings.Repeat("more output\n", terminalLogLimit)))
	if got := tm.LocalURLs("app"); !reflect.DeepEqual(got, want) {
		t.Fatalf("startup URLs lost after log rollover: %v", got)
	}
	if got := tm.LocalURLs("other"); len(got) != 0 {
		t.Fatalf("URLs leaked to another terminal: %v", got)
	}
	tm.Stop("app")
	if got := tm.LocalURLs("app"); len(got) != 0 {
		t.Fatalf("stop kept stale URLs: %v", got)
	}
	tm.logs["app"] = &terminalLog{}
	tm.logs["app"].append([]byte("http://localhost:6000/\n"))
	if got := tm.LocalURLs("app"); !reflect.DeepEqual(got, []string{"http://localhost:6000/"}) {
		t.Fatalf("new launch kept stale URLs: %v", got)
	}
}
