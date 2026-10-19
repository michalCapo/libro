const { test } = require('node:test')
const assert = require('node:assert/strict')
const fs = require('node:fs/promises')
const path = require('node:path')
const { capturePageArea } = require('./page-area')

test('captures, scales and clamps the selected area for backend storage', async () => {
  let crop
  const target = {
    capturePage: async () => ({
      getSize: () => ({ width: 300, height: 200 }),
      isEmpty: () => false,
      crop: rect => { crop = rect; return { toPNG: () => Buffer.from('cropped PNG') } },
    }),
  }
  const data = await capturePageArea(target, { x: 100, y: 50, width: 200, height: 100, pixelRatio: 1.5 })
  assert.deepEqual(crop, { x: 150, y: 75, width: 150, height: 125 })
  assert.equal(Buffer.from(data.split(',')[1], 'base64').toString(), 'cropped PNG')
  await assert.rejects(capturePageArea(target, { x: 400, y: 0, width: 10, height: 10, pixelRatio: 1 }))
  await assert.rejects(capturePageArea(target, { x: 0, y: 0, width: NaN, height: 10, pixelRatio: 1 }))
  await assert.rejects(capturePageArea(target, { x: 0, y: 0, width: 10, height: 10 }), /Invalid selected area/)
  await assert.rejects(capturePageArea({ ...target, capturePage: async () => { throw new Error('capture failed') } }, { x: 0, y: 0, width: 10, height: 10, pixelRatio: 1 }), /capture failed/)
})


test('whole-page annotations release their debugger on success and failure', async () => {
  const vm = require('node:vm')
  const source = await fs.readFile(path.join(__dirname, 'page-area.js'), 'utf8')
  const exported = {exports:{}}
  let resized, clip
  const image = {resize: size => { resized = size; return image }, toPNG: () => Buffer.from('full PNG')}
  vm.runInNewContext(source, {Buffer, module:exported, require: name => name === 'electron' ? {nativeImage:{createFromBuffer: () => image}} : require(name)})
  for (const alreadyAttached of [false, true]) for (const oversized of [false, true]) for (const captureFails of [false, true]) {
    let attached = alreadyAttached, attaches = 0, detaches = 0
    const target = {
      isDestroyed: () => false,
      getZoomFactor: () => 1.2,
      debugger: {
        isAttached: () => attached,
        attach: () => { attached = true; attaches++ },
        detach: () => { attached = false; detaches++ },
        sendCommand: async (method, params) => {
          if (method === 'Page.getLayoutMetrics') return {contentSize:{x:0,y:0,width:1440,height:oversized ? 36000 : 3240}}
          if (method === 'Runtime.evaluate') return {result:{value:1.8}}
          assert.equal(method, 'Page.captureScreenshot')
          clip = params.clip
          if (captureFails) throw new Error('capture failed')
          return {data:Buffer.from('PNG').toString('base64')}
        },
      },
    }
    const capture = exported.exports.capturePageArea(target, {fullPage:true})
    if (oversized || captureFails) await assert.rejects(capture, oversized ? /Page exceeds/ : /capture failed/)
    else {
      assert.equal(Buffer.from((await capture).split(',')[1], 'base64').toString(), 'full PNG')
      // 1440 device pixels at ratio 1.8 is 800 CSS pixels, clipped at zoom 1.2.
      assert.deepEqual({...resized}, {width:800, height:1800})
      assert.deepEqual({...clip}, {x:0, y:0, width:960, height:2160, scale:1})
    }
    assert.equal(attached, alreadyAttached)
    assert.equal(attaches, alreadyAttached ? 0 : 1)
    assert.equal(detaches, alreadyAttached ? 0 : 1)
  }
})
