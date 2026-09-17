const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const { test } = require('node:test')

const source = fs.readFileSync(path.join(__dirname, '../internal/components.go'), 'utf8')
const render = source.slice(source.indexOf('\tfunction renderDirectoryItems('), source.indexOf('\n\tfunction render(){', source.indexOf('\tfunction renderDirectoryItems(')))
const selection = source.slice(source.indexOf('\tfunction selectedDirectory('), source.indexOf('\tfunction completeSelectedDirectory('))

function setup(query) {
  const calls = []
  const rows = [0, 1].map(index => ({
    listeners: {},
    getAttribute() { return String(index) },
    addEventListener(type, handler) { this.listeners[type] = handler },
  }))
  const context = vm.createContext({
    query: () => query,
    isPathQuery: () => true,
    dirMatches: [
      { name: 'other', path: '/home/capo/code/private/other' },
      { name: 'nirio', path: '/home/capo/code/private/nirio' },
    ],
    selectedIdx: 0,
    lookupLoading: false,
    hoverEnabled: false,
    escapeHtml: value => value,
    closePopup() {},
    __ws: { call: (method, payload) => calls.push({ method, payload }) },
    res: { querySelectorAll: () => rows },
  })
  vm.runInContext(render + selection, context)
  return { context, calls, rows }
}

test('clicking a directory opens that row even when the query is its parent', () => {
  for (const query of ['~/code/private/', '/home/capo/code/private/']) {
    const { context, calls, rows } = setup(query)
    vm.runInContext('renderDirectoryItems(res, false)', context)
    rows[1].listeners.mousedown({ preventDefault() {} })
    assert.equal(calls.length, 1)
    assert.equal(calls[0].method, 'project.create')
    assert.equal(calls[0].payload['project-path'], '/home/capo/code/private/nirio')
  }
})

test('Enter still opens the typed directory with a trailing slash', () => {
  const { context, calls } = setup('~/code/private/')
  vm.runInContext('openSelectedDirectory()', context)
  assert.equal(calls[0].payload['project-path'], '~/code/private')
})
