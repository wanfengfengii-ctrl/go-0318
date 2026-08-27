import { build } from 'esbuild';
import { cpSync, mkdirSync } from 'node:fs';

mkdirSync('dist', { recursive: true });

await build({
  entryPoints: ['src/main.js'],
  bundle: true,
  minify: true,
  format: 'iife',
  outfile: 'dist/main.js',
});

cpSync('index.html', 'dist/index.html');
console.log('frontend built to dist/');
