const {build} = require('esbuild');
const {readFileSync} = require('node:fs');
(async()=>{
  const result=await build({entryPoints:['internal/file-editor.js'],bundle:true,minify:true,format:'iife',globalName:'libroFileEditor',outfile:'internal/file-editor.bundle.js',write:false});
  if(!Buffer.from(result.outputFiles[0].contents).equals(readFileSync('internal/file-editor.bundle.js')))throw new Error('Files editor bundle is stale. Run npm run build:files.');
  console.log('Files editor bundle is current.');
})().catch(error=>{console.error(error.message);process.exitCode=1;});
