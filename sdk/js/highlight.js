/**
 * Syntax highlighting for `@code` blocks: no dependency, no script in the page.
 *
 * One generic tokeniser parameterised per language rather than a lexer per
 * language. It recognises comments, strings, numbers and keywords, which is
 * what carries almost all of the readability gain. Kept byte-identical to the
 * Go implementation in highlight.go — the same document must highlight the same
 * way whichever renderer produced the page.
 *
 * @module xtxt/highlight
 */

const kw = (...w) => new Set(w);

const LANGUAGES = {
  go: { line: ['//'], block: ['/*', '*/'], quotes: '"\'`',
    keywords: kw('break','case','chan','const','continue','default','defer','else',
      'fallthrough','for','func','go','goto','if','import','interface','map','package',
      'range','return','select','struct','switch','type','var','nil','true','false') },
  javascript: { line: ['//'], block: ['/*', '*/'], quotes: '"\'`',
    keywords: kw('async','await','break','case','catch','class','const','continue',
      'default','delete','do','else','export','extends','finally','for','from','function',
      'if','import','in','instanceof','let','new','of','return','super','switch','this',
      'throw','try','typeof','var','void','while','yield','null','true','false') },
  python: { line: ['#'], block: ['', ''], quotes: '"\'',
    keywords: kw('and','as','assert','async','await','break','class','continue','def',
      'del','elif','else','except','finally','for','from','global','if','import','in',
      'is','lambda','None','nonlocal','not','or','pass','raise','return','try','while',
      'with','yield','True','False') },
  rust: { line: ['//'], block: ['/*', '*/'], quotes: '"\'',
    keywords: kw('as','async','await','break','const','continue','crate','dyn','else',
      'enum','extern','fn','for','if','impl','in','let','loop','match','mod','move','mut',
      'pub','ref','return','self','static','struct','trait','type','unsafe','use','where',
      'while','true','false') },
  c: { line: ['//'], block: ['/*', '*/'], quotes: '"\'',
    keywords: kw('auto','break','case','char','const','continue','default','do','double',
      'else','enum','extern','float','for','goto','if','int','long','return','short',
      'signed','sizeof','static','struct','switch','typedef','union','unsigned','void',
      'volatile','while') },
  java: { line: ['//'], block: ['/*', '*/'], quotes: '"\'',
    keywords: kw('abstract','boolean','break','case','catch','class','const','continue',
      'default','do','double','else','enum','extends','final','finally','float','for','if',
      'implements','import','instanceof','int','interface','long','new','package','private',
      'protected','public','return','static','super','switch','this','throw','throws','try',
      'void','while','null','true','false') },
  shell: { line: ['#'], block: ['', ''], quotes: '"\'',
    keywords: kw('case','do','done','elif','else','esac','export','fi','for','function',
      'if','in','local','return','then','while') },
  sql: { line: ['--'], block: ['/*', '*/'], quotes: '\'"',
    keywords: kw('AND','AS','BY','CREATE','DELETE','DROP','FROM','GROUP','HAVING','INSERT',
      'INTO','JOIN','LEFT','LIMIT','NOT','NULL','ON','OR','ORDER','SELECT','SET','TABLE',
      'UPDATE','VALUES','WHERE') },
  json: { line: [], block: ['', ''], quotes: '"', keywords: kw('true','false','null') },
  yaml: { line: ['#'], block: ['', ''], quotes: '"\'', keywords: kw('true','false','null') },
};

const ALIASES = {
  js: 'javascript', jsx: 'javascript', ts: 'javascript', typescript: 'javascript',
  tsx: 'javascript', mjs: 'javascript', py: 'python', rs: 'rust', sh: 'shell',
  bash: 'shell', zsh: 'shell', cpp: 'c', 'c++': 'c', h: 'c', hpp: 'c', cc: 'c',
  yml: 'yaml', golang: 'go',
};

// Matches Go's html.EscapeString, which also escapes ' as &#39;.
const esc = (s) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
  .replace(/"/g, '&#34;').replace(/'/g, '&#39;');

const isDigit = (c) => c >= '0' && c <= '9';
const isHex = (c) => (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F');
const isWord = (c) => c === '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || isDigit(c);

/** Closing quote, honouring escapes and stopping at a newline. */
function stringEnd(s, start) {
  const quote = s[start];
  for (let i = start + 1; i < s.length; i++) {
    if (s[i] === '\\') i++;
    else if (s[i] === '\n' && quote !== '`') return i;
    else if (s[i] === quote) return i + 1;
  }
  return s.length;
}

/**
 * Escaped HTML with span classes around comments, strings, numbers and
 * keywords. An unknown language is escaped and returned unchanged.
 */
export function highlightHTML(source, language) {
  let name = String(language ?? '').trim().toLowerCase();
  if (ALIASES[name]) name = ALIASES[name];
  const spec = LANGUAGES[name];
  if (!spec) return esc(source);

  let out = '';
  let plain = 0;
  const flush = (upto) => { if (upto > plain) out += esc(source.slice(plain, upto)); };
  const span = (cls, text) => { out += `<span class="tok-${cls}">${esc(text)}</span>`; };

  let i = 0;
  while (i < source.length) {
    const lineMarker = spec.line.find((m) => m && source.startsWith(m, i));
    if (lineMarker) {
      let end = source.indexOf('\n', i);
      if (end < 0) end = source.length;
      flush(i); span('com', source.slice(i, end)); i = plain = end;
      continue;
    }
    if (spec.block[0] && source.startsWith(spec.block[0], i)) {
      const at = source.indexOf(spec.block[1], i + spec.block[0].length);
      const end = at < 0 ? source.length : at + spec.block[1].length;
      flush(i); span('com', source.slice(i, end)); i = plain = end;
      continue;
    }
    if (spec.quotes.includes(source[i])) {
      const end = stringEnd(source, i);
      flush(i); span('str', source.slice(i, end)); i = plain = end;
      continue;
    }
    if (isDigit(source[i]) && (i === 0 || !isWord(source[i - 1]))) {
      let end = i;
      while (end < source.length &&
             (isDigit(source[end]) || source[end] === '.' || source[end] === 'x' ||
              isHex(source[end]))) end++;
      flush(i); span('num', source.slice(i, end)); i = plain = end;
      continue;
    }
    if (isWord(source[i]) && (i === 0 || !isWord(source[i - 1]))) {
      let end = i;
      while (end < source.length && isWord(source[end])) end++;
      if (spec.keywords.has(source.slice(i, end))) {
        flush(i); span('kw', source.slice(i, end)); plain = end;
      }
      i = end;
      continue;
    }
    i++;
  }
  flush(source.length);
  return out;
}
