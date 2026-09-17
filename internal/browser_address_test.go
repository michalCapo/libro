package libro

import (
	"os/exec"
	"strings"
	"testing"
)

func TestAddressHistory(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	const harness = `
const assert=require('node:assert/strict'),vm=require('node:vm');
let script='';process.stdin.on('data',d=>script+=d);process.stdin.on('end',()=>{
 const storage=new Map();let navigated='',focused=false;
 function element(){return {value:'',dataset:{},children:[],classList:{add(){},remove(){}},setAttribute(){},removeAttribute(){},append(...rows){this.children.push(...rows)},appendChild(row){this.children.push(row)},replaceChildren(){this.children=[]},addEventListener(n,f){this[n]=f},scrollIntoView(){},focus(){focused=true},select(){}}}
 function boot(){
  const input=element(),results=element(),dialog=element(),address=element();address.value='about:blank';
  const nodes={'url-popup':dialog,'url-popup-input':input,'url-popup-results':results,'urlinput-a':address,'frame-a':{querySelector(){return element()}}};
  const window={__libroSelectedApp:'a',__libroNavigateAddress(id,url){navigated=url;return true}};
  vm.runInNewContext(script,{window,document:{getElementById(id){return nodes[id]},createElement:element},localStorage:{getItem:k=>storage.get(k),setItem:(k,v)=>storage.set(k,v)},Date});
  return {window,input,results};
 }
 let app=boot();app.window.__libroRememberURL('http://localhost:1411/');app.window.__libroRememberURL('https://example.com/');app.window.__libroRememberURL('http://localhost:1411/');
 assert.equal(JSON.parse(storage.get('libro.browser.history')).length,2);
 app=boot();app.window.__libroOpenURLPopupFor('a');assert.ok(focused);assert.equal(app.input.value,'');
 app.input.value='1411';app.input.input();assert.equal(app.results.children.length,1);
 app.input.keydown({key:'Enter',preventDefault(){},stopImmediatePropagation(){}});assert.equal(navigated,'http://localhost:1411/');
 storage.set('libro.browser.history',JSON.stringify([{url:'http://old/',time:Date.now()-31*86400000}]));app.input.value='';app.input.input();assert.equal(app.results.children.length,0);
 storage.set('libro.browser.history','broken');app.input.input();assert.equal(app.results.children.length,0);
});`
	cmd := exec.Command(node, "-e", harness)
	cmd.Stdin = strings.NewReader(urlPopupJS(""))
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("address history: %v\n%s", err, output)
	}
}
