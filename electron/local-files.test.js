const { test } = require('node:test')
const assert = require('node:assert/strict')
const fs = require('node:fs')
const vm = require('node:vm')

const source = fs.readFileSync(require.resolve('./main.js'), 'utf8')
const rule = source.slice(source.indexOf('function blocksLocalFileRead('), source.indexOf('\nfunction configureLibroSession('))

test('local pages load their own assets but cannot read other local files', () => {
  const context = vm.createContext({})
  vm.runInContext(rule, context)
  for (const resourceType of ['mainFrame', 'stylesheet', 'script', 'image', 'font', 'media']) {
    assert.equal(context.blocksLocalFileRead({ resourceType }), false, resourceType)
  }
  for (const resourceType of ['xhr', 'subFrame', 'object']) {
    assert.equal(context.blocksLocalFileRead({ resourceType }), true, resourceType)
  }
  assert.match(source, /onBeforeRequest\(\{ urls: \['file:\/\/\/\*'\] \}/)
})
