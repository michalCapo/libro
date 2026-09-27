import {EditorState, Compartment} from '@codemirror/state';
import {EditorView, Decoration, lineNumbers, highlightActiveLine, highlightActiveLineGutter, drawSelection} from '@codemirror/view';
import {syntaxTree, ensureSyntaxTree, StreamLanguage, syntaxHighlighting, HighlightStyle, bracketMatching} from '@codemirror/language';
import {search} from '@codemirror/search';
import {tags, highlightTree, classHighlighter} from '@lezer/highlight';
import {vim, Vim, getCM} from '@replit/codemirror-vim';
import {javascript} from '@codemirror/lang-javascript';
import {go} from '@codemirror/lang-go';
import {cpp} from '@codemirror/lang-cpp';
import {python} from '@codemirror/lang-python';
import {rust} from '@codemirror/lang-rust';
import {java} from '@codemirror/lang-java';
import {json} from '@codemirror/lang-json';
import {html} from '@codemirror/lang-html';
import {css} from '@codemirror/lang-css';
import {markdown} from '@codemirror/lang-markdown';

import {xml} from '@codemirror/lang-xml';
import {yaml} from '@codemirror/lang-yaml';
import {sql} from '@codemirror/lang-sql';
import {php} from '@codemirror/lang-php';
import {vue} from '@codemirror/lang-vue';
import {less} from '@codemirror/lang-less';
import {sass} from '@codemirror/lang-sass';
import {shell} from '@codemirror/legacy-modes/mode/shell';
import {csharp,kotlin,objectiveC,objectiveCpp} from '@codemirror/legacy-modes/mode/clike';
import {ruby} from '@codemirror/legacy-modes/mode/ruby';
import {perl} from '@codemirror/legacy-modes/mode/perl';
import {lua} from '@codemirror/legacy-modes/mode/lua';
import {r} from '@codemirror/legacy-modes/mode/r';
import {swift} from '@codemirror/legacy-modes/mode/swift';
import {diff} from '@codemirror/legacy-modes/mode/diff';
import {properties} from '@codemirror/legacy-modes/mode/properties';
import {vb} from '@codemirror/legacy-modes/mode/vb';
import {wast} from '@codemirror/legacy-modes/mode/wast';

const editors = new WeakMap();
const languages = {js:()=>javascript(), jsx:()=>javascript({jsx:true}), ts:()=>javascript({typescript:true}), tsx:()=>javascript({typescript:true,jsx:true}), go, c:cpp, h:cpp, cpp, cc:cpp, hpp:cpp, py:python, rs:rust, java, json, html, htm:html, css, md:markdown, markdown};
export function languageFor(path) {
  const name=path.split('/').pop().toLowerCase();
  if(name==='gemfile'||name==='rakefile')return StreamLanguage.define(ruby);
  const ext = path.split('.').pop().toLowerCase();
  const legacy={sh:shell,bash:shell,zsh:shell,cs:csharp,kt:kotlin,kts:kotlin,m:objectiveC,mm:objectiveCpp,rb:ruby,pl:perl,lua,r,swift,diff,patch:diff,ini:properties,conf:properties,properties,vb,vbnet:vb,wasm:wast};
  if(legacy[ext])return StreamLanguage.define(legacy[ext]);
  return ({xml,svg:xml,yaml,yml:yaml,sql,php,vue,less,scss:sass,sass,jsonc:json,jsonl:json,ndjson:json,cxx:cpp,pyi:python,mdx:markdown}[ext]?.() || languages[ext]?.()) || (/^[cm][jt]s$/.test(ext) ? javascript({typescript:ext.endsWith('ts')}) : []);
}
// Keep the bundled Highlight.js coverage for formats without a CM parser.
let fallbackHighlighter;
function fallbackSyntax(view,path,source,compartment) {
  const name=path.split('/').pop().toLowerCase(),ext=name.split('.').pop();
  const language=/^(gnu)?makefile$/.test(name)?'makefile':({gql:'graphql',graphql:'graphql',svelte:'xml'})[ext];
  if(!language || source.length>256*1024 || source.split('\n').some(line=>line.length>1000))return;
  fallbackHighlighter ||= new Promise((resolve,reject)=>{
    if(window.hljs){resolve(window.hljs);return;}
    const script=document.createElement('script');script.src='/assets/highlight/highlight.min.js';
    script.onload=()=>resolve(window.hljs);script.onerror=()=>{script.remove();fallbackHighlighter=null;reject(new Error('Syntax highlighting unavailable'));};document.head.append(script);
  });
  fallbackHighlighter.then(highlighter=>{
    if(!view.dom.isConnected)return;
    const content=document.createElement('div');content.innerHTML=highlighter.highlight(source,{language,ignoreIllegals:true}).value;
    const walker=document.createTreeWalker(content,NodeFilter.SHOW_TEXT),marks=[];let offset=0,node;
    while((node=walker.nextNode())){
      let parent=node.parentElement,classes=[];
      while(parent && parent!==content){if(parent.className)classes.push(parent.className);parent=parent.parentElement;}
      if(classes.length && node.length)marks.push(Decoration.mark({class:classes.join(' ')}).range(offset,offset+node.length));
      offset+=node.length;
    }
    view.dispatch({effects:compartment.reconfigure(EditorView.decorations.of(Decoration.set(marks)))});
  }).catch(()=>{});
}
const symbolKinds = /^(FunctionDeclaration|FunctionDecl|MethodDecl|MethodDeclaration|MethodDefinition|FunctionDefinition|ClassDeclaration|ClassDefinition|ClassSpecifier|InterfaceDeclaration|TypeAliasDeclaration|TypeSpec|StructItem|EnumItem|TraitItem|FunctionItem)$/;
const functionKinds = /Function|Method/;
export function symbolsFor(state) {
  const tree = ensureSyntaxTree(state, state.doc.length, 100) || syntaxTree(state), result = [];
  tree.iterate({enter(node) {
    let declaration = symbolKinds.test(node.name);
    if (node.name === 'VariableDeclaration') declaration = true;
    if (!declaration) return;
    const line = state.doc.lineAt(node.from);
    const header = state.doc.sliceString(node.from, Math.min(node.to, node.from + 180)).split('\n')[0].trim();
    const match=header.match(/^(?:export\s+)?(?:default\s+)?(?:async\s+)?(const|let|var|function|class|interface|type|enum|struct|func)\s+([\w$]+)/);
    const name=match?.[2] || header.match(/^(?:async\s+)?([\w$]+)\s*\(/)?.[1] || header.split(/[({=:]/)[0].trim();
    const kind=match?.[1] || (functionKinds.test(node.name)?'func':'type');
    let depth=0;for(let parent=node.node.parent;parent;parent=parent.parent)if(symbolKinds.test(parent.name))depth++;
    result.push({label:header,name,kind:kind==='function'?'func':kind,depth, line:line.number, from:node.from, to:node.to, function:functionKinds.test(node.name) || node.name === 'VariableDeclaration' && /ArrowFunction|FunctionExpression/.test(node.node.toString())});
  }});
  return result;
}

for (const [from,to] of [['gb','G'],['ff','*'],['fb','#']]) Vim.map(from,to,'normal');
Vim.defineMotion('libroScroll', (cm, start, args) => ({line:Math.max(0,Math.min(cm.lastLine(),start.line + args.direction * 15 * (args.repeat || 1))),ch:0}));
Vim.mapCommand('<C-d>','motion','libroScroll',{direction:1},{});
Vim.mapCommand('<C-u>','motion','libroScroll',{direction:-1},{});
Vim.defineMotion('libroFunction', (cm, start, args) => {
  const api = editors.get(cm), positions = api.symbols().filter(s=>s.function);
  let line = start.line + 1;
  for (let i=0;i<(args.repeat || 1);i++) {
    const next = args.direction > 0 ? positions.find(s=>s.line>line) : positions.findLast(s=>s.line<line);
    if (!next) break;
    line = next.line;
  }
  return {line:line-1,ch:0};
});
Vim.mapCommand(']f','motion','libroFunction',{direction:1},{});
Vim.mapCommand('[f','motion','libroFunction',{direction:-1},{});
Vim.defineAction('libroSymbols', cm => editors.get(cm)?.onSymbols());
Vim.mapCommand('go','action','libroSymbols',{},{});
for(const [keys,kind] of [['gd','definition'],['gr','references'],['gD','declaration'],['gi','implementation'],['gt','typeDefinition']]){
  Vim.defineAction('libro'+kind,cm=>editors.get(cm)?.onNavigate(kind));
  Vim.mapCommand(keys,'action','libro'+kind,{},{});
}
Vim.defineOperator('yank', (cm,args,ranges,anchor) => {
  const value = cm.getSelection();
  Vim.getRegisterController().pushText(args.registerName === '+' ? undefined : args.registerName,'yank',value,args.linewise,cm.state.vim.visualBlock);
  editors.get(cm)?.copy(value);
  return cm.state.vim.visualMode ? cm.getCursor('start') : anchor;
});
const colors = HighlightStyle.define([
  {tag:[tags.keyword,tags.operator],color:'var(--ws-code-keyword)'},
  {tag:[tags.string,tags.regexp],color:'var(--ws-code-string)'},
  {tag:[tags.comment],color:'var(--ws-code-comment)'},
  {tag:[tags.number,tags.bool,tags.null,tags.typeName],color:'var(--ws-code-constant)'},
  {tag:[tags.function(tags.variableName),tags.className,tags.heading],color:'var(--ws-code-title)'},
  {tag:[tags.propertyName,tags.attributeName],color:'var(--ws-code-property)'},
]);
export function create({element,path,source,wrap,onSymbols,onNavigate,onChange,copy}) {
  const wrapping = new Compartment(),fallback=new Compartment();
  const view = new EditorView({parent:element,state:EditorState.create({doc:source,extensions:[
    vim({status:false}), EditorState.readOnly.of(true),
    // Also reject programmatic Vim/IME/paste changes. Refresh replaces the state instead.
    EditorState.changeFilter.of(()=>false),
    lineNumbers(),highlightActiveLine(),highlightActiveLineGutter(),drawSelection(),bracketMatching(),search(),
    languageFor(path),fallback.of([]),syntaxHighlighting(colors),wrapping.of(wrap ? EditorView.lineWrapping : []),
    EditorView.contentAttributes.of({'aria-label':'Source preview','aria-readonly':'true'}),
    EditorView.updateListener.of(update=>{if(update.selectionSet) onChange?.();}),
    EditorView.theme({'&':{height:'100%',backgroundColor:'var(--ws-bg)',color:'var(--ws-fg)'},'.cm-scroller':{overflow:'auto',fontFamily:'ui-monospace,monospace',fontSize:'12px',lineHeight:'1.6'},'.cm-gutters':{backgroundColor:'var(--ws-chrome)',color:'var(--ws-muted)',borderRight:'1px solid var(--ws-line)'},'.cm-activeLine, .cm-activeLineGutter':{backgroundColor:'var(--ws-hover)'},'.cm-cursor':{borderLeftColor:'var(--ws-fg)'},'&.cm-focused .cm-selectionBackground, .cm-selectionBackground':{backgroundColor:'color-mix(in srgb, var(--ws-accent) 25%, transparent)'},'.cm-panels':{backgroundColor:'var(--ws-chrome)',color:'var(--ws-fg)'}})
  ]})});
  const cm = getCM(view);
  // Vim treats digits as counts before consulting mappings. Handle these personal
  // numeric mappings first, while keeping counts such as 10j and 40j intact.
  let digit='', digitTimer;
  const flushDigit=()=>{const value=digit;digit='';clearTimeout(digitTimer);if(value)Vim.handleKey(cm,value);};
  view.dom.addEventListener('keydown',event=>{
    if(event.target!==view.contentDOM || event.isComposing)return;
    if(digit){
      const first=digit;digit='';clearTimeout(digitTimer);
      if(event.key===first){event.preventDefault();event.stopImmediatePropagation();Vim.handleKey(cm,first==='0'?'^':'$');return;}
      if(event.key!=='Escape')Vim.handleKey(cm,first);
    }
    if(event.ctrlKey || event.altKey || event.metaKey || cm.state.vim.insertMode)return;
    const state=cm.state.vim.inputState;
    if(state.prefixRepeat.length || state.motionRepeat.length || state.keyBuffer.length)return;
    if(event.key==='0' || event.key==='4'){
      event.preventDefault();event.stopImmediatePropagation();digit=event.key;digitTimer=setTimeout(flushDigit,400);
    }else if(event.key==='5'){
      event.preventDefault();event.stopImmediatePropagation();Vim.handleKey(cm,'%');
    }
  },true);
  view.contentDOM.addEventListener('blur',()=>{digit='';clearTimeout(digitTimer);});
  let symbols;
  const api = {
    view,onSymbols,onNavigate,copy,
    symbols:()=>symbols ||= symbolsFor(view.state),
    position:()=>{const pos=view.state.selection.main.head,line=view.state.doc.lineAt(pos);return {line:line.number,column:pos-line.from,top:view.scrollDOM.scrollTop,left:view.scrollDOM.scrollLeft};},
    jump:(line,column=0)=>{const target=view.state.doc.line(Math.max(1,Math.min(view.state.doc.lines,line))); const pos=Math.min(target.to,target.from+column);Vim.exitVisualMode(cm); view.dispatch({selection:{anchor:pos},effects:EditorView.scrollIntoView(pos,{y:'center'})});},
    selection:()=>view.state.sliceDoc(view.state.selection.main.from,view.state.selection.main.to),
    word:()=>{const range=view.state.wordAt(view.state.selection.main.head);return range ? view.state.sliceDoc(range.from,range.to):'';},
    focus:()=>view.focus(),
    wrap:value=>view.dispatch({effects:wrapping.reconfigure(value ? EditorView.lineWrapping : [])}),
    destroy:()=>{clearTimeout(digitTimer);editors.delete(cm);view.destroy();},
  };
  editors.set(cm,api);
  fallbackSyntax(view,path,source,fallback);
  return api;
}

// Static highlighted context avoids creating an editor for every result.
export function highlightContext(element,path,source,hitFrom,hitTo){
  const state=EditorState.create({doc:source,extensions:[languageFor(path)]});
  const tree=ensureSyntaxTree(state,source.length,50)||syntaxTree(state);
  let position=0;
  function append(from,to,classes=''){
    if(to<=from)return;
    const boundaries=[from,...[hitFrom,hitTo].filter(n=>n>from&&n<to),to].sort((a,b)=>a-b);
    for(let i=0;i<boundaries.length-1;i++){
      const a=boundaries[i],b=boundaries[i+1],span=document.createElement('span');
      span.className=classes+(a>=hitFrom&&a<hitTo?' ws-file-reference-hit':'');
      span.textContent=source.slice(a,b);element.append(span);
    }
  }
  highlightTree(tree,classHighlighter,(from,to,classes)=>{append(position,from);append(from,to,classes);position=to;});
  append(position,source.length);
}
