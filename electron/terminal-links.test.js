const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const { test } = require('node:test')

const source = fs.readFileSync(path.join(__dirname, '../internal/components.go'), 'utf8')
const code = source.slice(source.indexOf('function findTerminalLinks('), source.indexOf('function openTerminalLink('))
const context = vm.createContext({})
vm.runInContext(code, context)

// Returns [clickable text, opened target] pairs.
const find = (text) => JSON.parse(JSON.stringify(context.findTerminalLinks(text)))
  .sort((a, b) => a.start - b.start)
  .map((l) => [text.slice(l.start, l.start + l.shown.length), l.text])
const same = (...links) => links.map((l) => [l, l])

test('finds web addresses and absolute file paths', () => {
  assert.deepEqual(find('- Windows: /tmp/claude-1000/-home-x/scratchpad/fuvo-windows.png'), same('/tmp/claude-1000/-home-x/scratchpad/fuvo-windows.png'))
  assert.deepEqual(find('see https://example.com/a?b=1. and (~/notes.md)'), same('https://example.com/a?b=1', '~/notes.md'))
  assert.deepEqual(find('edit /src/main.go:42:7, then C:\\Users\\me\\a.txt or C:/Users/me/b.txt'), [['/src/main.go:42:7', '/src/main.go'], ...same('C:\\Users\\me\\a.txt', 'C:/Users/me/b.txt')])
})

test('keeps brackets that belong to the link', () => {
  assert.deepEqual(find('read https://en.wikipedia.org/wiki/Function_(mathematics).'), same('https://en.wikipedia.org/wiki/Function_(mathematics)'))
  assert.deepEqual(find('(/tmp/a.txt]) and [/app/[id]/page.tsx]'), same('/tmp/a.txt', '/app/[id]/page.tsx'))
})

test('quoted paths may contain spaces', () => {
  assert.deepEqual(find('open "C:\\Program Files\\app\\notes.txt" or \'/Users/me/My Report.pdf\''), same('C:\\Program Files\\app\\notes.txt', '/Users/me/My Report.pdf'))
})

test('skips words with slashes and paths inside web addresses', () => {
  assert.deepEqual(find('read and/or write; http://localhost:3000/api/x'), same('http://localhost:3000/api/x'))
  assert.deepEqual(find('just / here, ~/ and "/"'), [])
})
