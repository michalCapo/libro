const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const { test } = require('node:test')

const source = fs.readFileSync(path.join(__dirname, '../internal/components.go'), 'utf8')
const themeCode = source.slice(source.indexOf('function applyTerminalTheme('), source.indexOf('\n\t\t\tfunction scan(root)', source.indexOf('function applyTerminalTheme(')))

for (const initialDark of [false, true]) {
  test(`theme switches preserve cached CLI colors and restore the original surface (${initialDark ? 'dark' : 'light'} start)`, () => {
    let dark = initialDark
    const palette = { background: initialDark ? '#1e1e1e' : '#fdfdfd' }
    const controllers = ['Codex', 'Pi', 'Claude'].map(() => ({ initialDark, term: { element: { style: {} }, options: { theme: palette } } }))
    const context = vm.createContext({
      document: { documentElement: { classList: { contains: () => dark } } },
      window: {}, terminals: new Map(controllers.map((c, i) => [i, c])),
    })
    vm.runInContext(themeCode, context)
    const refresh = context.window.__libroRefreshTerminalThemes
    refresh()
    controllers.forEach(c => assert.equal(c.term.element.style.filter, ''))
    dark = !initialDark
    refresh()
    refresh() // Duplicate OS/class notifications must not toggle back.
    controllers.forEach(c => {
      assert.equal(c.term.element.style.filter, 'invert(1) hue-rotate(180deg)')
      assert.equal(c.term.options.theme, palette)
    })
    // Terminals opened after the switch already have the current palette.
    const fresh = { initialDark: dark, term: { element: { style: {} } } }
    context.terminals.set(3, fresh)
    refresh()
    assert.equal(fresh.term.element.style.filter, '')
    dark = initialDark
    refresh()
    controllers.forEach(c => assert.equal(c.term.element.style.filter, ''))
    assert.equal(fresh.term.element.style.filter, 'invert(1) hue-rotate(180deg)')
  })
}
