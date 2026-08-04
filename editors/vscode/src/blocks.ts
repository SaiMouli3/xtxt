/**
 * Pairs `@name … @endname` over raw lines.
 *
 * Deliberately free of any `vscode` import: folding and the outline both need
 * these extents, and keeping it pure means it can be tested by running node.
 *
 * A regex pair cannot do this job, which is why the folding markers this
 * replaces were wrong: `@image(src="x.png")` matches the start pattern exactly
 * and has no terminator, so it opened a region that swallowed the rest of the
 * file. Only a scan that requires a matching `@endname` gets it right.
 */

import { FENCED_BY_DEFAULT } from 'xtxt-js';

export interface BlockRange {
  name: string;
  /** 0-based line of the `@name` line. */
  start: number;
  /** 0-based line of the `@endname` line. */
  end: number;
}

const OPEN = /^@(?!end)([A-Za-z_][A-Za-z0-9_-]*)/;
const CLOSE = /^@end([A-Za-z_][A-Za-z0-9_-]*)\s*$/;

export function blockRanges(lines: readonly string[]): BlockRange[] {
  const open: Array<{ name: string; start: number }> = [];
  const out: BlockRange[] = [];

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    const top = open[open.length - 1];

    // A fenced payload is arbitrary text — a Python decorator or an XTXT
    // snippet inside `@code` is content, not structure. Only the block's own
    // terminator ends it.
    if (top && FENCED_BY_DEFAULT.has(top.name)) {
      if (line.trim() === `@end${top.name}`) {
        out.push({ name: top.name, start: top.start, end: i });
        open.pop();
      }
      continue;
    }

    const close = CLOSE.exec(line);
    if (close) {
      for (let j = open.length - 1; j >= 0; j--) {
        if (open[j].name === close[1]) {
          out.push({ name: close[1], start: open[j].start, end: i });
          open.splice(j); // whatever is still open inside was never closed
          break;
        }
      }
      continue;
    }

    const start = OPEN.exec(line);
    if (start) open.push({ name: start[1], start: i });
  }

  // Anything left in `open` never closed — a single-line directive, or the
  // unterminated block the parser is already reporting as an error.
  return out;
}

/**
 * The header cells of a table block, in order.
 *
 * Kept here with the other pure line work so the picker's column list can be
 * tested: the separator row is punctuation rather than a header, and skipping
 * it is the part that is easy to get wrong.
 */
export function headerCells(lines: readonly string[], block: BlockRange): string[] {
  for (let i = block.start + 1; i < block.end; i++) {
    const line = lines[i].trim();
    if (!line) continue;
    const cells = line.replace(/^\||\|$/g, '').split('|').map((c) => c.trim());
    if (cells.every((c) => c && !c.replace(/[-:]/g, ''))) continue;
    return cells.filter(Boolean);
  }
  return [];
}
