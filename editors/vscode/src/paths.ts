/**
 * Path containment, kept free of any `vscode` import so it can be tested by
 * running node. This is a security control — it decides whether a path a
 * repository chose is somewhere this extension is willing to write — so a
 * plain `startsWith` will not do: `/work/notes-evil` starts with `/work/notes`
 * without being inside it.
 */

/** Whether `target` is `root` itself or sits underneath it. */
export function isUnder(root: string, target: string): boolean {
  const base = root.endsWith('/') ? root : `${root}/`;
  return target === root || target.startsWith(base);
}
