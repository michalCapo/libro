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

func TestProjectDialogCancelsLookup(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is required")
	}
	script := projectDialogJS("test-session")
	start := strings.Index(script, "function cancelLookup()")
	end := strings.Index(script[start:], "function sortProjects") + start
	closeStart := strings.Index(script, "function closePopup()")
	closeEnd := strings.Index(script[closeStart:], "function launchProject()") + closeStart
	js := `
const assert=require('node:assert/strict');
var lookupTimer=0,lookupSeq=0,lookupController=null,lookupLoading=false;
var dirMatches=[],filtered=[],hoverEnabled=false,q='nisa';
var dlg={classList:{add(){}}},inp={value:q};
function getDlg(){return dlg;} function getInp(){return inp;}
function query(){return q;} function isPathQuery(){return false;}
function render(){} function hideCreateConfirm(){} function getResults(){return null;}
var requests=[],results=[];
var window={__libroProjectDialogSetDirMatches(p){results.push(p);}};
function fetch(url,options){return new Promise(resolve=>requests.push({url,options,resolve}));}
` + script[start:end] + script[closeStart:closeEnd] + `
(async()=>{
 scheduleLookup(); closePopup();
 await new Promise(r=>setTimeout(r,110));
 assert.equal(requests.length,0,'close cancels debounce');
 lookup('nisa');
 closePopup();
 assert.equal(requests[0].options.signal.aborted,true,'close aborts active request');
 requests[0].resolve({ok:true,json:async()=>[{name:'nisa'}]});
 await new Promise(r=>setImmediate(r));
 assert.equal(results.length,0,'late results ignored');
 lookup('old');
 q='new'; scheduleLookup();
 assert.equal(requests[1].options.signal.aborted,true,'new query aborts previous search');
 closePopup();
})().catch(e=>{console.error(e);process.exitCode=1;});
`
	if output, err := exec.Command(node, "-e", js).CombinedOutput(); err != nil {
		t.Fatalf("lookup cancellation: %v\n%s", err, output)
	}
}
