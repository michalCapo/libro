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
