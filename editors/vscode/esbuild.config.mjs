import esbuild from 'esbuild';
import process from 'node:process';

const production = process.argv.includes('production');

// xtxt-js is bundled in; `vscode` is provided by the host at runtime.
const context = await esbuild.context({
  entryPoints: ['src/extension.ts'],
  bundle: true,
  external: ['vscode'],
  format: 'cjs',
  platform: 'node',
  target: 'node18',
  outfile: 'out/extension.js',
  sourcemap: !production,
  minify: production,
  logLevel: 'info',
});

if (production) {
  await context.rebuild();
  await context.dispose();
} else {
  await context.watch();
}
