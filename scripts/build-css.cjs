// Compiles internal/tailwind.css with the classes used by Libro and g-sui,
// and copies the Material Icons Round font. --check fails when either is stale.
const { compile, optimize } = require('@tailwindcss/node');
const { Scanner } = require('@tailwindcss/oxide');
const { execFileSync } = require('node:child_process');
const { readFileSync, writeFileSync } = require('node:fs');
const path = require('node:path');

const check = process.argv.includes('--check');
const outputs = { css: 'internal/tailwind.min.css', font: 'internal/material-icons-round.woff2' };

(async () => {
  const gsui = execFileSync('go', ['list', '-m', '-f', '{{.Dir}}', 'github.com/michalCapo/g-sui'], { encoding: 'utf8' }).trim();
  const input = readFileSync('internal/tailwind.css', 'utf8') + `@source ${JSON.stringify(path.join(gsui, 'ui'))};\n`;
  const compiler = await compile(input, { base: path.resolve('internal'), onDependency() {} });
  const candidates = new Scanner({ sources: compiler.sources }).scan();
  const files = {
    css: Buffer.from(optimize(compiler.build(candidates), { minify: true }).code),
    font: readFileSync('node_modules/material-icons/iconfont/material-icons-round.woff2'),
  };
  for (const [key, data] of Object.entries(files)) {
    if (!check) writeFileSync(outputs[key], data);
    else if (!data.equals(readFileSync(outputs[key]))) throw new Error(`${outputs[key]} is stale. Run npm run build:css.`);
  }
  console.log(check ? 'Styles are current.' : `Wrote ${outputs.css} and ${outputs.font}.`);
})().catch(error => { console.error(error.message); process.exitCode = 1; });
