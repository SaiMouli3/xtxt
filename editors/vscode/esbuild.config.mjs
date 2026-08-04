import esbuild from 'esbuild';
import { readFileSync } from 'node:fs';
import process from 'node:process';
import { fileURLToPath } from 'node:url';

// The same file the Go renderer embeds, inlined here at build time so the
// preview and `xtxt export --interactive` cannot ship different behaviour.
const runtime = readFileSync(
  fileURLToPath(new URL('../../assets/chart-runtime.js', import.meta.url)), 'utf8');

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
  define: { __CHART_RUNTIME__: JSON.stringify(runtime) },
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
