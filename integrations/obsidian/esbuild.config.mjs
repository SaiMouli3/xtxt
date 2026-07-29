import esbuild from 'esbuild';
import process from 'node:process';

const production = process.argv.includes('production');

const context = await esbuild.context({
  entryPoints: ['src/main.ts'],
  bundle: true,
  // Obsidian provides these at runtime; bundling them would break the plugin.
  external: ['obsidian', 'electron', '@codemirror/*', '@lezer/*'],
  format: 'cjs',
  target: 'es2020',
  logLevel: 'info',
  sourcemap: production ? false : 'inline',
  treeShaking: true,
  minify: production,
  outfile: 'main.js',
});

if (production) {
  await context.rebuild();
  await context.dispose();
} else {
  await context.watch();
}
