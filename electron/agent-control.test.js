const {test} = require('node:test')
const assert = require('node:assert/strict')
const fs = require('node:fs')
const os = require('node:os')
const path = require('node:path')
const {createController, startControlServer} = require('./agent-control')

test('local bridge rejects unauthenticated and web-origin requests and cleans up', async () => {
  const dir=fs.mkdtempSync(path.join(os.tmpdir(),'libro-control-test-'))
  const stop=await startControlServer(dir,'test', async command=>command)
  const descriptor=path.join(dir,'desktop-control-test.json')
  try {
    const {port,token}=JSON.parse(fs.readFileSync(descriptor,'utf8'))
    const url=`http://127.0.0.1:${port}/`
    assert.equal((await fetch(url,{method:'POST',body:'{}'})).status,403)
    assert.equal((await fetch(url,{method:'POST',headers:{Authorization:`Bearer ${token}`,Origin:'https://example.test'},body:'{}'})).status,403)
    const response=await fetch(url,{method:'POST',headers:{Authorization:`Bearer ${token}`},body:JSON.stringify({action:'list'})})
    assert.deepEqual(await response.json(),{result:{action:'list'}})
    if(process.platform!=='win32') assert.equal(fs.statSync(descriptor).mode&0o777,0o600)
  } finally {stop();assert.equal(fs.existsSync(descriptor),false);fs.rmSync(dir,{recursive:true,force:true})}
})

test('dispatches application and issues commands, and rejects browser automation', async () => {
  const calls = []
  const win = {isDestroyed: () => false, webContents: {executeJavaScript: async script => { calls.push(script); return {ok:true} }}}
  const control = createController(() => win)
  await control({action:'application', operation:'status', project:'/project'})
  await control({action:'issues', command:{action:'list', project:'/project'}})
  assert.match(calls[0], /window.libroWorkspace.applicationControl/)
  assert.match(calls[1], /window.libroNotes.control/)
  await assert.rejects(control({action:'snapshot', panel:'panel'}), /Unknown Libro action/)
  await assert.rejects(control({action:'application', operation:'invalid', project:'/project'}), /Invalid application/)
  await assert.rejects(control({action:'issues', command:{action:'list'}}), /Invalid issues/)
  await control({action:'application', operation:'status', project:'/project'})
  assert.equal(calls.length, 3)
})
