const { test } = require('node:test')
const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')

const source = fs.readFileSync(path.join(__dirname, 'main.js'), 'utf8')
const setup = source.slice(source.indexOf('const instance ='), source.indexOf('let browserController'))
function profile(env) {
  const result = {}
  vm.runInNewContext(setup, {
    process: { env, platform: 'linux' }, path,
    app: {
      getPath: () => '/config',
      setPath: (key, value) => { result[key] = value },
      setName: value => { result.name = value },
    },
  })
  return result
}

test('named desktops have separate persistent browser profiles', () => {
  assert.equal(profile({}).userData, path.join('/config', 'libro'))
  const dev = profile({ LIBRO_INSTANCE: 'dev' })
  assert.equal(dev.userData, path.join('/config', 'libro', 'instances', 'dev'))
  assert.equal(dev.name, 'Libro (dev)')
  assert.notEqual(dev.userData, profile({ LIBRO_INSTANCE: 'feature-a' }).userData)
  assert.throws(() => profile({ LIBRO_INSTANCE: '../bad' }), /Invalid Libro instance/)
})
