package libro

import (
	"os/exec"
	"strings"
	"testing"
)

func TestProjectDialogFilterSurvivesDialogReplacement(t *testing.T) {
	script := projectDialogJS("test-session")

	if !strings.Contains(script, "document.addEventListener('input'") {
		t.Fatal("project filtering must use a document listener so replacing the dialog does not detach it")
	}
	if strings.Contains(script, "inp.addEventListener('input',filter)") {
		t.Fatal("project filtering is still bound to the replaceable input element")
	}
}

func TestProjectDialogSearchMatching(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is required to test project dialog JavaScript")
	}
	script := projectDialogJS("test-session")
	extract := func(start, end string) string {
		t.Helper()
		first := strings.Index(script, start)
		if first < 0 {
			t.Fatalf("missing %s", start)
		}
		last := strings.Index(script[first:], end)
		if last < 0 {
			t.Fatalf("missing %s", end)
		}
		return script[first : first+last]
	}
	js := `
const assert = require('node:assert/strict');
var q='', filtered=[], selectedIdx=0;
var window={__libroProjects:[
 {name:'ide',path:'/home/michal/code/github/ide'},
 {name:'libro',path:'/home/michal/code/github/libro'},
 {name:'nirio',path:'/home/michal/code/private/nirio'},
 {name:'nisa',path:'/home/michal/code/live/nisa',isActive:true},
 {name:'id',displayName:'editor',path:'/tmp/id'},
 {name:'feature',branch:'fix/search',path:'/tmp/feature'}
]};
function query(){return q;}
function isPathQuery(q){return q.startsWith('/');}
function scheduleLookup(){}
function render(){}
` + extract("function fuzzyMatch(", "function escapeHtml(") + extract("function sortProjects(", "function hideCreateConfirm(") + `
function search(value){q=value;filter();return filtered.map(item=>item.name);}
assert.deepEqual(search('ide'),['ide']);
assert.deepEqual(search('IDE'),['ide']);
assert.deepEqual(search('nro'),['nirio']);
assert.deepEqual(search('fix/search'),['feature']);
assert.deepEqual(search('github'),['ide','libro']);
assert.deepEqual(search('missing'),[]);
assert.deepEqual(search('/tmp/'),[]);
search('');
assert.equal(filtered[selectedIdx].name,'nisa');
search('i');
assert.equal(selectedIdx,0);
`
	if output, err := exec.Command(node, "-e", js).CombinedOutput(); err != nil {
		t.Fatalf("project search failed: %v\n%s", err, output)
	}
}
