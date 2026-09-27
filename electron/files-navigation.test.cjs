const {test}=require('node:test');
const assert=require('node:assert/strict');
const {readFileSync}=require('node:fs');
const {JSDOM}=require('jsdom');

function fixture(t, scoped=false) {
  const dom=new JSDOM(`<div data-workspace-project="test"><div data-files="files"><div class="ws-file-preview"><div class="ws-file-path"></div><div class="ws-file-image-tools"><button class="ws-file-image-reset"></button></div><label class="ws-file-render"><input type="checkbox"></label><label class="ws-file-wrap"><input type="checkbox" checked></label><div class="ws-file-text"></div><div class="ws-file-context"></div><div class="ws-file-media" hidden></div></div><div class="ws-file-sidebar"><input class="ws-file-filter"><label class="ws-file-hidden"><input type="checkbox"></label><div class="ws-file-tree" tabindex="0"></div><div class="ws-file-shortcuts"><span>Copy line</span><kbd>yy</kbd></div><div role="status"></div></div></div></div>`,{url:'http://localhost',runScripts:'outside-only',pretendToBeVisual:true});
  const w=dom.window,calls=[],copied=[];t.after(()=>w.close());
  let refresh;const interval=w.setInterval.bind(w);w.setInterval=(fn,ms)=>{if(ms===3000)refresh=fn;return interval(fn,ms);};
  w.HTMLElement.prototype.scrollIntoView=()=>{};
  w.Range.prototype.getClientRects=()=>[];
  w.Range.prototype.getBoundingClientRect=()=>({left:0,right:0,top:0,bottom:0,width:0,height:0});
  Object.defineProperty(w.navigator,'clipboard',{value:{writeText:async text=>copied.push(text)}});
  w.__ws={call:(action,data)=>calls.push({action,data})};
  w.eval(readFileSync('internal/file-editor.bundle.js','utf8'));
  const create=w.libroFileEditor.create;
  let editor;
  w.libroFileEditor={...w.libroFileEditor,create:options=>{
    editor=create(options);
    // jsdom has no line layout; provide the vertical coordinate adapter.
    editor.view.cm.findPosV=(start,amount)=>({line:Math.max(0,Math.min(editor.view.state.doc.lines-1,start.line+amount)),ch:start.ch});
    return editor;
  }};
  const filesScript=readFileSync('internal/files.js','utf8');
  w.eval(scoped?'(function(libroFileEditor){delete window.libroFileEditor;'+filesScript+'})(window.libroFileEditor);':filesScript);w.libroFiles.init();
  function reply(result,call=calls.at(-1)){w.libroFiles.receive({id:'files',request:call.data.request,...result});}
  function key(key,opts={}){w.document.activeElement.dispatchEvent(new w.KeyboardEvent('keydown',{key,bubbles:true,cancelable:true,...opts}));}
  function keys(value){for(const keyName of value)key(keyName);}
  const source='// function fake() {}\nfunction first(a) {\n  return a + 1;\n}\n\nconst second = () => {\n  return 2;\n};\n'+Array.from({length:40},(_,i)=>'// line '+i).join('\n');
  function open(path='main.js',text=source){reply({directory:true,entries:[{name:path,path,dir:false}]});w.document.querySelector('.ws-file-tree').focus();key('Enter');reply({path,text});}
  return {w,calls,copied,reply,key,keys,open,refresh:()=>{w.document.querySelector('[data-files]').getClientRects=()=>[{}];refresh();},get editor(){return editor;}};
}

test('read-only Vim motions, counts, selections, clipboard, brackets and symbols',t=>{
  const f=fixture(t);f.open();
  assert.equal(f.editor.position().line,1);
  f.keys('10j');assert.equal(f.editor.position().line,11);
  f.key('d',{ctrlKey:true});assert.equal(f.editor.position().line,26);
  f.key('u',{ctrlKey:true});assert.equal(f.editor.position().line,11);
  f.keys('gg');f.keys(']f');assert.equal(f.editor.position().line,2);
  f.keys(']f');assert.equal(f.editor.position().line,6);
  f.keys('[f');assert.equal(f.editor.position().line,2);
  assert.deepEqual(Array.from(f.editor.symbols(),s=>s.line),[2,6]);
  f.keys('yy');assert.equal(f.copied.at(-1),'function first(a) {\n');
  f.keys('vj');f.keys('y');assert.match(f.copied.at(-1),/function first/);
  const original=f.editor.view.state.doc.toString();
  f.keys('gg');f.keys('dd');f.keys('i');f.keys('text');f.key('Escape');
  f.editor.view.dispatch({changes:{from:0,to:3,insert:'edited'}});
  assert.equal(f.editor.view.state.doc.toString(),original);
  f.keys('gg');f.keys('go');assert.ok(f.w.document.querySelector('.ws-file-picker'));
});

test('existing tree fuzzy results, history, location copy and project search',async t=>{
  const f=fixture(t);f.open();
  f.w.document.querySelector('.ws-file-tree').focus();assert.equal(f.w.document.activeElement.className,'ws-file-tree');
  f.key('Escape');assert.equal(f.w.document.activeElement.classList.contains('cm-content'),true);
  f.keys(' yy');assert.equal(f.copied.at(-1),'main.js:1');
  f.keys('  ');
  const filter=f.w.document.activeElement;assert.equal(filter.className,'ws-file-filter');
  filter.value='nested';filter.dispatchEvent(new f.w.Event('input'));
  await new Promise(r=>setTimeout(r,180));
  assert.equal(f.calls.at(-1).action,'files.index');
  f.reply({entries:[{name:'other.js',path:'src/nested/other.js'},{name:'skip.js',path:'skip.js'}]});
  assert.equal(f.w.document.querySelectorAll('.ws-file-row').length,1);
  f.key('Enter');f.reply({path:'src/nested/other.js',text:'function other() {}'});
  f.key('o',{ctrlKey:true});f.reply({path:'main.js',text:'function first() {}'});
  assert.equal(f.w.document.querySelector('.ws-file-path').textContent,'main.js');
  f.keys(' ss');const input=f.w.document.activeElement;
  input.value='other';input.dispatchEvent(new f.w.Event('input'));
  await new Promise(r=>setTimeout(r,240));
  const old=f.calls.at(-1);
  input.value='first';input.dispatchEvent(new f.w.Event('input'));
  await new Promise(r=>setTimeout(r,240));
  f.reply({matches:[{path:'wrong.js',line:1,text:'other'}]},old);
  assert.equal(f.w.document.querySelectorAll('.ws-file-result').length,0);
  f.reply({matches:[{path:'main.js',line:1,column:9,text:'function first() {}'}]});
  f.key('Enter');assert.equal(f.editor.position().column,9);
  assert.equal(f.w.document.querySelector('.ws-file-picker'),null);
});

test('symbol lists use language syntax rather than comment or string matches',t=>{
  for(const [path,source,count] of [
    ['main.go','package main\n// func fake() {}\ntype Thing struct {}\nfunc (t Thing) Run() {}\nfunc main() {}',3],
    ['main.ts','// function fake() {}\ninterface Thing {}\nclass Task { run() {} }\nconst next = () => 1;',4],
    ['main.py','# def fake():\nclass Task:\n  def run(self):\n    pass',2],
    ['main.cpp','// void fake() {}\nclass Task { public: void run() {} };\nint main() {return 0;}',3],
    ['main.rs','// fn fake() {}\nstruct Task {}\nfn main() {}',2],
  ]){
    const f=fixture(t);f.open(path,source);
    assert.equal(f.editor.symbols().length,count,path+': '+JSON.stringify(f.editor.symbols()));
  }
});

test('custom line boundaries, word search and matching brackets',t=>{
  const f=fixture(t);f.open('test.js','  function one() {\n    return 1;\n}\nfunction two() {}');
  f.keys('00');assert.equal(f.editor.position().column,2);
  f.keys('44');assert.equal(f.editor.position().column,17);
  f.keys('5');assert.equal(f.editor.position().line,3);
  f.keys('gb');assert.equal(f.editor.position().line,4);
  f.keys('gg');f.keys('ff');assert.equal(f.editor.position().line,4);
});

test('refresh preserves source position without stealing tree focus; recent files remain available',t=>{
  const f=fixture(t);f.open();f.keys('10j');
  const tree=f.w.document.querySelector('.ws-file-tree');
  f.refresh();const request=f.calls.at(-1);
  assert.equal(request.action,'files.read');
  f.w.document.querySelector('.ws-file-tree').focus();
  f.reply({path:'main.js',text:f.editor.view.state.doc.toString()+'\n// changed'},request);
  assert.equal(f.w.document.activeElement,tree);
  assert.equal(f.editor.position().line,11);
  f.key('Enter');assert.equal(f.editor.position().line,11);
  f.keys(' ,');assert.match(f.w.document.querySelector('.ws-file-results').textContent,/main.js/);
  f.key('Escape');assert.ok(f.w.document.activeElement.classList.contains('cm-content'));
});

test('out-of-order file responses cannot replace the latest selection',t=>{
  const f=fixture(t);
  f.reply({directory:true,entries:[{name:'first.js',path:'first.js'},{name:'second.js',path:'second.js'}]});
  f.w.document.querySelector('.ws-file-tree').focus();f.key('Enter');const first=f.calls.at(-1);
  f.key('j');f.key('Enter');const second=f.calls.at(-1);
  f.reply({path:'second.js',text:'const second=2'},second);
  f.reply({path:'first.js',text:'const first=1'},first);
  assert.equal(f.w.document.querySelector('.ws-file-path').textContent,'second.js');
});

test('Files scrolling takes priority over workspace tool shortcuts',t=>{
  const f=fixture(t);f.open();
  const settings=f.w.document.createElement('div');settings.id='workspace-settings';settings.hidden=true;f.w.document.body.append(settings);
  const source=readFileSync('internal/workspace.js','utf8');
  const start=source.indexOf("  window.addEventListener('keydown', event => {\n    const input = event.target.closest?.('[data-tool-key]');");
  const end=source.indexOf("  window.addEventListener('keydown', event => {",start+10);
  assert.ok(start>=0 && end>start);
  f.w.openedTools=[];
  f.w.eval(`const toolKeys={lazydata:'Ctrl+D'};const shortcut=event=>event.ctrlKey?'Ctrl+'+event.key.toUpperCase():'';const tool=id=>window.openedTools.push(id);\n`+source.slice(start,end));
  f.key('d',{ctrlKey:true});assert.equal(f.editor.position().line,16);
  f.key('u',{ctrlKey:true});assert.equal(f.editor.position().line,1);
  f.keys('2');f.key('d',{ctrlKey:true});assert.equal(f.editor.position().line,31);
  assert.equal(f.w.openedTools.length,0);
  const outside=f.w.document.createElement('button');f.w.document.body.append(outside);outside.focus();
  f.key('d',{ctrlKey:true});assert.deepEqual(Array.from(f.w.openedTools),['lazydata']);
});

test('semantic navigation shortcuts jump to a definition and preserve history',t=>{
 const f=fixture(t);f.open();f.keys('j');
 f.keys('gd');const lookup=f.calls.at(-1);
 assert.equal(lookup.action,'files.navigate');assert.equal(lookup.data.kind,'definition');assert.equal(lookup.data.line,2);
 assert.match(f.w.document.querySelector('.ws-file-result-status').textContent,/Finding definitions/);
 f.reply({matches:[{path:'definition.js',line:3,column:9}]},lookup);
 assert.equal(f.calls.at(-1).data.path,'definition.js');
 f.reply({path:'definition.js',text:'// header\n\nfunction target() {}'});
 assert.equal(f.editor.position().line,3);assert.equal(f.editor.position().column,9);
 f.key('o',{ctrlKey:true});assert.equal(f.calls.at(-1).data.path,'main.js');
 f.reply({path:'main.js',text:'// comment\nfunction first() {}'});
 assert.equal(f.editor.position().line,2);
});

test('reference lists navigate; closed lookups cannot steal focus',t=>{
 const f=fixture(t);f.open();
 f.keys('gr');const lookup=f.calls.at(-1);
 assert.equal(lookup.data.kind,'references');
 f.reply({matches:[{path:'first.js',line:2,column:0},{path:'other.js',line:5,column:3,parents:1}]},lookup);
 assert.equal(f.w.document.querySelector('.ws-file-navigation input'),null);
 f.key('n',{ctrlKey:true});
 f.key('Enter');assert.equal(f.calls.at(-1).data.path,'other.js');assert.equal(f.calls.at(-1).data.parents,1);
 f.reply({path:'other.js',text:'1\n2\n3\n4\nfunction other() {}'});
 assert.equal(f.editor.position().line,5);
 for(const [keys,kind] of [['gD','declaration'],['gi','implementation'],['gt','typeDefinition']]){
  f.keys(keys);const call=f.calls.at(-1);assert.equal(call.data.kind,kind);
  f.key('Escape');f.reply({matches:[{path:'late.js',line:1,column:0}]},call);
  assert.equal(f.w.document.querySelector('.ws-file-picker'),null);
  assert.equal(f.w.document.querySelector('.ws-file-path').textContent,'other.js');
 }
 f.keys('gd');f.reply({error:'Install the language server'});
 assert.match(f.w.document.querySelector('.ws-file-result-status').textContent,/Install the language server/);
});

 test('language server restart is available from keyboard',t=>{
 const f=fixture(t);f.open();
 f.keys(' lr');
 assert.equal(f.calls.at(-1).data.kind,'restart');
 assert.match(f.w.document.querySelector('.ws-file-result-status').textContent,/Restarting language server/);
 f.reply({});
 assert.match(f.w.document.querySelector('.ws-file-result-status').textContent,/Language server restarted/);
 f.key('Escape');f.keys(' lr');
 assert.equal(f.calls.at(-1).data.kind,'restart');
 f.reply({error:'Server unavailable'});
 assert.match(f.w.document.querySelector('.ws-file-result-status').textContent,/Server unavailable/);
 });

test('navigation buffer shows highlighted grouped context and Neovim controls with a scoped editor bundle',t=>{
 const f=fixture(t,true);f.open();f.keys('gr');
 f.reply({matches:[{path:'a.ts',line:2,column:6,startLine:1,context:'// context\nconst target = 1;\ntarget();'},{path:'b.ts',line:4,column:0,startLine:4,context:'target();'}]});
 assert.ok(f.w.document.querySelector('.ws-file-navigation'));
 assert.equal(f.w.document.querySelectorAll('.ws-file-reference-file').length,2);
 assert.equal(f.w.document.querySelector('.ws-file-reference-hit').textContent,'target');
 assert.ok(f.w.document.querySelector('.tok-keyword'));
 assert.equal(f.w.document.querySelector('.ws-file-reference-numbers').textContent,'1\n2\n3');
 f.key('n',{ctrlKey:true});assert.match(f.w.document.querySelector('[aria-selected=true].ws-file-result').getAttribute('aria-label'),/b.ts:4:/);
 f.keys('[e');assert.match(f.w.document.querySelector('[aria-selected=true].ws-file-result').getAttribute('aria-label'),/a.ts:2:/);
 f.keys(']e');f.key('p',{ctrlKey:true});f.key('n',{ctrlKey:true});f.key('Enter');assert.equal(f.calls.at(-1).data.path,'b.ts');
 f.reply({path:'b.ts',text:'\n\n\ntarget();'});f.keys('gr');f.reply({matches:[]});
 f.keys('q');assert.equal(f.w.document.querySelector('.ws-file-navigation'),null);
 assert.equal(f.editor.position().line,4);
});

test('Space p toggles preview',t=>{
 const f=fixture(t);f.open('example.md','# Title');
 const preview=f.w.document.querySelector('.ws-file-render input');
 assert.equal(preview.checked,false);
 f.keys(' p');assert.equal(preview.checked,true);
 f.w.document.querySelector('.ws-file-tree').focus();f.keys(' p');assert.equal(preview.checked,false);
});

test('go opens compact symbols with short names and two-stage Escape',t=>{
 const f=fixture(t);f.open('symbols.ts','const timestamp = () => new Date();\nfunction makeCommon(config: string) { return config; }');f.keys('go');
 const dialog=f.w.document.querySelector('.ws-file-symbol-dialog');assert.ok(dialog);
 assert.equal(dialog.getAttribute('role'),'dialog');
 const rows=dialog.querySelectorAll('.ws-file-result');
 assert.equal(rows[0].textContent,'const timestamp');assert.equal(rows[1].textContent,'func makeCommon');
 f.key('Escape');assert.equal(f.w.document.activeElement,dialog.querySelector('.ws-file-results'));
 f.key('/');assert.equal(f.w.document.activeElement,dialog.querySelector('input'));
 const input=f.w.document.activeElement;input.value='make';input.dispatchEvent(new f.w.Event('input'));
 assert.equal(dialog.querySelectorAll('.ws-file-result').length,1);
 f.key('Enter');assert.equal(f.editor.position().line,2);assert.equal(f.w.document.querySelector('.ws-file-symbol-dialog'),null);
 f.keys('go');f.key('Escape');f.key('Escape');assert.equal(f.w.document.querySelector('.ws-file-symbol-dialog'),null);
});
