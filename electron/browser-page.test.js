const {test} = require('node:test')
const {EventEmitter} = require('node:events')
const assert = require('node:assert/strict')
const vm = require('node:vm')
const {pageAction} = require('./browser-page')
const {performAction} = require('./browser-control')

// Execute the actual isolated-world functions against DOM-shaped nodes. AX text
// must stay a Text node here: treating every backend node as an Element hid this bug.
function fixture() {
  const card = {nodeType:1,isConnected:true,scrollIntoView(){},checkVisibility:()=>true,getBoundingClientRect:()=>({x:10,y:20,width:100,height:40})}
  const text = {nodeType:3,isConnected:true,parentElement:card}
  const nodes = new Map([[1,{nodeType:9,isConnected:true}],[2,card],[3,text]])
  const objects = new Map(), events = []
  let nextObject = 0
  function remote(value) {
    if (value == null) return {subtype:'null',value:null}
    const objectId = String(++nextObject)
    objects.set(objectId,value)
    return {objectId}
  }
  const target = Object.assign(new EventEmitter(),{
    session:new EventEmitter(),getURL:()=> 'https://fixture.test',
    executeJavaScriptInIsolatedWorld:async (_world,scripts)=>scripts[0].code.includes('width:innerWidth') ? {width:300,height:200} : null,
    debugger:Object.assign(new EventEmitter(),{isAttached:()=>true,sendCommand:async (method,params)=>{
      switch (method) {
        case 'Network.enable': case 'Runtime.enable': return {}
        case 'Page.getFrameTree': return {frameTree:{frame:{id:'main'}}}
        case 'Page.createIsolatedWorld': return {executionContextId:1}
        case 'Accessibility.getFullAXTree': return {nodes:[
          {backendDOMNodeId:1,role:{value:'RootWebArea'}},
          {backendDOMNodeId:2,role:{value:'button'},name:{value:'Job card'}},
          {backendDOMNodeId:3,role:{value:'StaticText'},name:{value:'Job card'}},
        ]}
        case 'DOM.resolveNode': {
          if (!nodes.has(params.backendNodeId)) throw new Error('No node with given id found')
          return {object:remote(nodes.get(params.backendNodeId))}
        }
        case 'Runtime.callFunctionOn': {
          const fn = vm.runInNewContext('('+params.functionDeclaration+')',{getComputedStyle:()=>({visibility:'visible'})})
          try {
            const value = await fn.apply(objects.get(params.objectId),(params.arguments || []).map(a=>a.value))
            return {result:params.returnByValue ? {value} : remote(value)}
          } catch (error) { return {exceptionDetails:{exception:{description:error.message}}} }
        }
        case 'Runtime.releaseObject': objects.delete(params.objectId);return {}
        case 'Input.dispatchMouseEvent': events.push(params);return {}
        default: throw new Error('Unexpected CDP command: '+method)
      }
    }}),
  })
  return {target,card,text,nodes,objects,events}
}

test('AX text refs click and wait on their containing element', async () => {
  const {target,objects,events} = fixture()
  const snapshot = await pageAction(target,{action:'snapshot'})
  assert.equal(snapshot.nodes[0].ref,undefined,'documents must not advertise element refs')
  for (const node of snapshot.nodes.slice(1)) {
    await performAction(target,{action:'click',ref:node.ref})
    assert.equal((await pageAction(target,{action:'wait',ref:node.ref,timeoutMs:0})).ok,true)
  }
  assert.deepEqual(events.map(e=>e.type),['mouseMoved','mousePressed','mouseReleased','mouseMoved','mousePressed','mouseReleased'])
  assert.ok(events.every(e=>e.x===60 && e.y===40))
  assert.equal(objects.size,0,'release both raw node and normalized element handles')
})

test('replaced nodes fail safely and explain how to recover', async () => {
  const {target,card,text,nodes,objects,events} = fixture()
  const snapshot = await pageAction(target,{action:'snapshot'})
  card.isConnected = false
  text.isConnected = false
  await assert.rejects(performAction(target,{action:'click',ref:snapshot.nodes[2].ref}),/Stale element reference.*take a new snapshot/)
  assert.equal((await pageAction(target,{action:'wait',ref:snapshot.nodes[2].ref,state:'detached',timeoutMs:0})).ok,true)
  text.parentElement = null
  await assert.rejects(performAction(target,{action:'click',ref:snapshot.nodes[2].ref}),/Stale element reference; take a new snapshot/)
  nodes.delete(2)
  await assert.rejects(performAction(target,{action:'click',ref:snapshot.nodes[1].ref}),/Stale element reference; take a new snapshot/)
  assert.equal(objects.size,0)
  assert.equal(events.length,0,'never click coordinates from a replaced node')
})
