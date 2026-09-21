const { build } = require('esbuild');
const { readFileSync } = require('node:fs');
(async () => {
  const result = await build({ entryPoints: ['internal/note-editor.js'], bundle: true, minify: true, format: 'iife', globalName: 'libroNoteEditor', outfile: 'internal/note-editor.bundle.js', write: false });
  if (!Buffer.from(result.outputFiles[0].contents).equals(readFileSync('internal/note-editor.bundle.js'))) {
    throw new Error('Notes editor bundle is stale. Run npm run build:notes.');
  }
  console.log('Notes editor bundle is current.');
})().catch(error => { console.error(error.message); process.exitCode = 1; });
