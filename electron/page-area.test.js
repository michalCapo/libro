const { test } = require('node:test')
const assert = require('node:assert/strict')
const fs = require('node:fs/promises')
const os = require('node:os')
const path = require('node:path')
const { capturePageArea } = require('./page-area')

test('captures, zooms and clamps the selected area, and saves its PNG', async () => {
  const dir = await fs.mkdtemp(path.join(os.tmpdir(), 'libro-capture-test-'))
  try {
    let crop
    const target = {
      getZoomFactor: () => 1.5,
      capturePage: async () => ({
        getSize: () => ({ width: 300, height: 200 }),
        isEmpty: () => false,
        crop: rect => { crop = rect; return { toPNG: () => Buffer.from('cropped PNG') } },
      }),
    }
    const filename = await capturePageArea(target, { x: 100, y: 50, width: 200, height: 100 }, dir)
    assert.deepEqual(crop, { x: 150, y: 75, width: 150, height: 125 })
    assert.equal(await fs.readFile(filename, 'utf8'), 'cropped PNG')
    await assert.rejects(capturePageArea(target, { x: 400, y: 0, width: 10, height: 10 }, dir))
    await assert.rejects(capturePageArea(target, { x: 0, y: 0, width: NaN, height: 10 }, dir))
    await assert.rejects(capturePageArea({ ...target, capturePage: async () => { throw new Error('capture failed') } }, { x: 0, y: 0, width: 10, height: 10 }, dir), /capture failed/)
  } finally {
    await fs.rm(dir, { recursive: true, force: true })
  }
})

test('whole-page annotations release their debugger on success and failure', async () => {
  const vm = require('node:vm')
  const dir = await fs.mkdtemp(path.join(os.tmpdir(), 'libro-page-test-'))
  const source = await fs.readFile(path.join(__dirname, 'page-area.js'), 'utf8')
  const exported = {exports:{}}
  const image = {resize: () => image, toPNG: () => Buffer.from('full PNG')}
  vm.runInNewContext(source, {Buffer, module:exported, require: name => name === 'electron' ? {nativeImage:{createFromBuffer: () => image}} : require(name)})
  try {
    for (const alreadyAttached of [false, true]) for (const oversized of [false, true]) for (const captureFails of [false, true]) {
      let attached = alreadyAttached, attaches = 0, detaches = 0
      const target = {
        isDestroyed: () => false,
        debugger: {
          isAttached: () => attached,
          attach: () => { attached = true; attaches++ },
          detach: () => { attached = false; detaches++ },
          sendCommand: async method => {
            if (method === 'Page.getLayoutMetrics') return {cssContentSize:{x:0,y:0,width:800,height:oversized ? 20000 : 1800}}
            assert.equal(method, 'Page.captureScreenshot')
            if (captureFails) throw new Error('capture failed')
            return {data:Buffer.from('PNG').toString('base64')}
          },
        },
      }
      const capture = exported.exports.capturePageArea(target, {fullPage:true}, dir)
      if (oversized || captureFails) await assert.rejects(capture, oversized ? /Page exceeds/ : /capture failed/)
      else assert.equal(await fs.readFile(await capture, 'utf8'), 'full PNG')
      assert.equal(attached, alreadyAttached)
      assert.equal(attaches, alreadyAttached ? 0 : 1)
      assert.equal(detaches, alreadyAttached ? 0 : 1)
    }
  } finally {
    await fs.rm(dir, {recursive:true, force:true})
  }
})
