/**
 * Type declarations for the XTXT JavaScript SDK.
 *
 * Hand-written rather than generated: the runtime is plain JavaScript with no
 * build step, and these types are part of the published package's contract.
 */

export type Kind = 'heading' | 'paragraph' | 'quote' | 'list' | 'directive' | 'block';
export type Severity = 'error' | 'warning';

/** One argument of a directive. `key` is empty for a positional argument. */
export interface Arg {
  key: string;
  value: string;
}

/** A directive's arguments, in source order. */
export declare class Args extends Array<Arg> {
  get(key: string): string;
  has(key: string): boolean;
  positional(i: number): string;
  /** The named argument, falling back to the first positional one. */
  resolve(key: string): string;
}

/** One entry in a list. `checked` is null unless it is a checklist item. */
export interface Item {
  text: string;
  ordered: boolean;
  checked: boolean | null;
}

/** One `Key: value` entry in a block payload. */
export interface Field {
  key: string;
  value: string;
}

/**
 * A block payload read as an ordered record. Note `toObject` rather than `map`:
 * this extends Array, so a method called `map` would shadow Array.prototype.map.
 */
export declare class Fields extends Array<Field> {
  get(key: string): string;
  toObject(): Record<string, string>;
}

/** A single block in the document tree. */
export declare class Node {
  kind: Kind;
  name: string;
  level: number;
  text: string;
  args: Args;
  items: Item[];
  /** 1-based line where the node starts. */
  line: number;
  fields(): Fields;
}

/** A parsed document. */
export declare class Document {
  version: string;
  nodes: Node[];
  metadata(): Record<string, string>;
}

/** A diagnostic tied to a line of source. */
export interface Issue {
  severity: Severity;
  line: number;
  message: string;
}

/**
 * The outcome of parsing. A document is always returned, even when `issues`
 * contains errors — recovery is part of the format's compatibility guarantee.
 */
export interface ParseResult {
  doc: Document;
  issues: Issue[];
  hasErrors(): boolean;
}

/** Parse an XTXT document. Never throws on malformed input. */
export declare function parse(src: string): ParseResult;

/** The interpreted payload of an `@table` block. */
export interface Table {
  header: string[];
  rows: string[][];
  /** "left", "right" or "center" per column. */
  align: string[];
}

export declare function parseTable(n: Node): Table;

/** Interpret a block payload as an ordered record. */
export declare function parseFields(payload: string): Fields;

/** Convert inline markup to HTML, escaping everything else. */
export declare function inlineHTML(s: string): string;

/** Strip inline markup, for plain text and analysis. */
export declare function inlineText(s: string): string;

/**
 * Semantic checks on top of the parser's syntactic ones. Unknown directives are
 * warnings, never errors.
 */
export declare function validate(doc: Document): Issue[];

export declare function sortIssues(issues: Issue[]): Issue[];

/** The standard directives, mapped to whether each is a fenced block. */
export declare const KNOWN: Map<string, boolean>;

export declare const FENCED_BY_DEFAULT: Set<string>;

// ---------------------------------------------------------------------------
// Extraction — the machine-facing view
// ---------------------------------------------------------------------------

export interface Outline {
  level: number;
  text: string;
  line: number;
}

/** A unit of work, from either a `@task` block or a checklist item. */
export interface Task {
  title: string;
  done: boolean;
  status?: string;
  owner?: string;
  due?: string;
  line: number;
}

/** A structured directive as an agent sees it. */
export interface Block {
  type: string;
  line: number;
  args?: Record<string, string>;
  fields?: Record<string, string>;
  /** Field keys in source order. */
  order?: string[];
  text: string;
}

export interface Link {
  text: string;
  href: string;
  line: number;
}

export interface Media {
  kind: string;
  src: string;
  caption: string;
  line: number;
}

export interface Code {
  language: string;
  lines: number;
  line: number;
  source: string;
}

/** Everything an agent needs without inferring structure from prose. */
export interface Extraction {
  version: string;
  metadata: Record<string, string>;
  outline: Outline[];
  tasks: Task[];
  blocks: Block[];
  links: Link[];
  media: Media[];
  code: Code[];
  text: string;
  words: number;
}

export declare function extract(doc: Document): Extraction;

// ---------------------------------------------------------------------------
// Conformance
// ---------------------------------------------------------------------------

/** The normalised shape used by the conformance suite. */
export declare function canonical(doc: Document): unknown;

export declare function canonicalIssues(issues: Issue[]): Array<{ severity: Severity; line: number }>;

// ---------------------------------------------------------------------------
// Rendering
// ---------------------------------------------------------------------------

export interface RenderOptions {
  /** Wrap the result in a standalone HTML document. */
  full?: boolean;
  title?: string;
}

export declare function renderHTML(doc: Document, options?: RenderOptions): string;

// ---------------------------------------------------------------------------
// Charts
// ---------------------------------------------------------------------------

export interface Series {
  name: string;
  values: number[];
}

export interface Chart {
  type: string;
  title: string;
  unit: string;
  labels: string[];
  series: Series[];
  /** Reported by validation; not rendered. */
  warnings: string[];
}

export declare function parseChart(n: Node): Chart;

/**
 * Reads a `@table` carrying a `chart=` argument as a chart over its own rows.
 * Takes the parsed table rather than parsing it, so the chart module does not
 * have to import the parser. Null when the table asks for no chart.
 */
export declare function tableChart(n: Node, table: Table): Chart | null;

/**
 * The chart's numbers as JSON, for the interactive runtime to read back out of
 * the DOM. Matches the Go renderer byte for byte.
 */
export declare function chartData(c: Chart): string;

/** The chart a `@table` asked for, above the table itself. */
export declare function renderTableChart(n: Node, table: Table): string;

/** Draw a chart as script-free inline SVG. */
export declare function renderChartSVG(c: Chart): string;

/** The mandatory text alternative for a chart. */
export declare function chartTableHTML(c: Chart): string;

/** Full `@chart` figure: SVG, optional caption, and the table view. */
export declare function renderChart(n: Node): string;
