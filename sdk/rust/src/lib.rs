//! XTXT — a plain-text document format with structure.
//!
//! A dependency-free implementation of the format defined in [SPEC.md]. This is
//! a port of the Go reference implementation, not a binding: it agrees with it
//! because both are checked against the same conformance suite.
//!
//! ```
//! let res = xtxt::parse("# Hello\n\nworld\n");
//! assert_eq!(res.doc.nodes.len(), 2);
//! assert_eq!(res.doc.nodes[0].kind, xtxt::Kind::Heading);
//! ```
//!
//! [SPEC.md]: https://github.com/SaiMouli3/xtxt/blob/main/SPEC.md

#![forbid(unsafe_code)]

use std::collections::{HashMap, HashSet};

// ---------------------------------------------------------------------------
// Model
// ---------------------------------------------------------------------------

/// The type of a [`Node`].
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub enum Kind {
    Heading,
    Paragraph,
    Quote,
    List,
    /// Inline form: `@name(args)`.
    Directive,
    /// Fenced form: `@name … @endname`.
    Block,
}

impl Kind {
    pub fn as_str(self) -> &'static str {
        match self {
            Kind::Heading => "heading",
            Kind::Paragraph => "paragraph",
            Kind::Quote => "quote",
            Kind::List => "list",
            Kind::Directive => "directive",
            Kind::Block => "block",
        }
    }
}

/// One argument of a directive. `key` is empty for a positional argument.
#[derive(Debug, Clone, PartialEq, Eq, Default)]
pub struct Arg {
    pub key: String,
    pub value: String,
}

/// A directive's arguments, in source order.
#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct Args(pub Vec<Arg>);

impl Args {
    pub fn get(&self, key: &str) -> &str {
        self.0
            .iter()
            .find(|a| a.key == key)
            .map(|a| a.value.as_str())
            .unwrap_or("")
    }

    pub fn has(&self, key: &str) -> bool {
        self.0.iter().any(|a| a.key == key)
    }

    pub fn positional(&self, i: usize) -> &str {
        self.0
            .iter()
            .filter(|a| a.key.is_empty())
            .nth(i)
            .map(|a| a.value.as_str())
            .unwrap_or("")
    }

    /// The named argument, falling back to the first positional one. This is
    /// what makes `@video("x.mp4")` and `@video(src="x.mp4")` equivalent.
    pub fn resolve(&self, key: &str) -> &str {
        let named = self.get(key);
        if !named.is_empty() {
            named
        } else {
            self.positional(0)
        }
    }

    pub fn is_empty(&self) -> bool {
        self.0.is_empty()
    }

    pub fn len(&self) -> usize {
        self.0.len()
    }

    pub fn iter(&self) -> std::slice::Iter<'_, Arg> {
        self.0.iter()
    }
}

/// One entry in a list. `checked` is `None` unless it is a checklist item.
#[derive(Debug, Clone, PartialEq, Eq, Default)]
pub struct Item {
    pub text: String,
    pub ordered: bool,
    pub checked: Option<bool>,
}

/// A single block in the document tree.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Node {
    pub kind: Kind,
    pub name: String,
    pub level: usize,
    pub text: String,
    pub args: Args,
    pub items: Vec<Item>,
    /// 1-based line where the node starts.
    pub line: usize,
}

impl Node {
    fn new(kind: Kind, line: usize) -> Self {
        Node {
            kind,
            name: String::new(),
            level: 0,
            text: String::new(),
            args: Args::default(),
            items: Vec::new(),
            line,
        }
    }

    /// The payload read as an ordered `Key: value` record.
    pub fn fields(&self) -> Fields {
        if self.kind != Kind::Block {
            return Fields::default();
        }
        parse_fields(&self.text)
    }
}

/// A parsed document.
#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct Document {
    pub version: String,
    pub nodes: Vec<Node>,
}

impl Document {
    /// The `@metadata` block as key/value pairs, with keys lowercased.
    pub fn metadata(&self) -> HashMap<String, String> {
        for n in &self.nodes {
            if n.kind == Kind::Block && n.name == "metadata" {
                return parse_metadata(&n.text);
            }
        }
        HashMap::new()
    }
}

/// How serious a diagnostic is.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Severity {
    Error,
    Warning,
}

impl Severity {
    pub fn as_str(self) -> &'static str {
        match self {
            Severity::Error => "error",
            Severity::Warning => "warning",
        }
    }
}

/// A diagnostic tied to a line of source.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Issue {
    pub severity: Severity,
    pub line: usize,
    pub message: String,
}

/// The outcome of parsing. A document is always returned, even when `issues`
/// contains errors — recovery is part of the format's compatibility guarantee.
#[derive(Debug, Clone, Default)]
pub struct ParseResult {
    pub doc: Document,
    pub issues: Vec<Issue>,
}

impl ParseResult {
    pub fn has_errors(&self) -> bool {
        self.issues.iter().any(|i| i.severity == Severity::Error)
    }
}

// ---------------------------------------------------------------------------
// Parser
// ---------------------------------------------------------------------------

/// Directives that are blocks even when their closing fence is missing;
/// used only to report a helpful error.
const FENCED_BY_DEFAULT: [&str; 7] = [
    "code", "table", "math", "mermaid", "metadata", "comment", "raw",
];

fn is_name_start(b: u8) -> bool {
    b == b'_' || b.is_ascii_alphabetic()
}

fn is_name_byte(b: u8) -> bool {
    is_name_start(b) || b == b'-' || b.is_ascii_digit()
}

fn is_blank(s: &str) -> bool {
    s.trim().is_empty()
}

fn read_lines(src: &str) -> Vec<String> {
    let mut lines: Vec<String> = src
        .replace("\r\n", "\n")
        .split('\n')
        .map(str::to_string)
        .collect();
    if lines.last().map(String::is_empty).unwrap_or(false) {
        lines.pop();
    }
    if let Some(first) = lines.first_mut() {
        if let Some(stripped) = first.strip_prefix('\u{feff}') {
            *first = stripped.to_string();
        }
    }
    lines
}

/// 1-6 for a heading line, else 0.
fn heading_level(s: &str) -> usize {
    let t = s.trim_start_matches([' ', '\t']);
    let b = t.as_bytes();
    let n = b.iter().take_while(|&&c| c == b'#').count();
    if n == 0 || n > 6 || n >= b.len() || b[n] != b' ' {
        return 0;
    }
    n
}

fn is_directive(s: &str) -> bool {
    let t = s.trim_start_matches([' ', '\t']);
    let b = t.as_bytes();
    b.len() >= 2 && b[0] == b'@' && is_name_start(b[1])
}

/// The directive name and the rest of the line after it.
fn directive_name(s: &str) -> (&str, &str) {
    let t = s.trim_start_matches([' ', '\t']);
    let b = t.as_bytes();
    let mut i = 1;
    while i < b.len() && is_name_byte(b[i]) {
        i += 1;
    }
    (&t[1..i], &t[i..])
}

/// The bullet or number prefix of a list item, or "".
fn item_prefix(s: &str) -> &str {
    let t = s.trim_start_matches([' ', '\t']);
    let b = t.as_bytes();
    if b.len() >= 2 && (b[0] == b'-' || b[0] == b'*') && b[1] == b' ' {
        return &t[..2];
    }
    let mut i = 0;
    while i < b.len() && b[i].is_ascii_digit() {
        i += 1;
    }
    if i > 0 && i + 1 < b.len() && b[i] == b'.' && b[i + 1] == b' ' {
        return &t[..i + 2];
    }
    ""
}

struct Parser {
    lines: Vec<String>,
    i: usize,
    doc: Document,
    issues: Vec<Issue>,
}

/// Parse an XTXT document. Parsing never fails: unrecognised content becomes a
/// diagnostic, not a lost node.
pub fn parse(src: &str) -> ParseResult {
    let mut p = Parser {
        lines: read_lines(src),
        i: 0,
        doc: Document::default(),
        issues: Vec::new(),
    };
    p.run();
    ParseResult {
        doc: p.doc,
        issues: p.issues,
    }
}

impl Parser {
    fn err(&mut self, line: usize, message: String) {
        self.issues.push(Issue {
            severity: Severity::Error,
            line,
            message,
        });
    }

    fn warn(&mut self, line: usize, message: String) {
        self.issues.push(Issue {
            severity: Severity::Warning,
            line,
            message,
        });
    }

    fn run(&mut self) {
        while self.i < self.lines.len() {
            let line = self.lines[self.i].clone();
            if is_blank(&line) {
                self.i += 1;
            } else if is_directive(&line) {
                self.directive();
            } else if heading_level(&line) > 0 {
                let lvl = heading_level(&line);
                let mut n = Node::new(Kind::Heading, self.i + 1);
                n.level = lvl;
                n.text = line.trim_start_matches([' ', '\t'])[lvl..]
                    .trim()
                    .to_string();
                self.doc.nodes.push(n);
                self.i += 1;
            } else if line.trim_start().starts_with('>') {
                self.quote();
            } else if !item_prefix(&line).is_empty() {
                self.list();
            } else {
                self.paragraph();
            }
        }
    }

    fn directive(&mut self) {
        let start = self.i;
        let line = self.lines[self.i].clone();
        let (name, rest) = directive_name(&line);
        let (name, rest) = (name.to_string(), rest.to_string());

        let args = match self.parse_args(&rest) {
            Some(a) => a,
            None => {
                self.err(start + 1, format!("unclosed argument list for @{name}"));
                self.i = start + 1;
                return;
            }
        };

        if name.starts_with("end") {
            self.err(start + 1, format!("@{name} has no matching opening fence"));
            self.i += 1;
            return;
        }

        if name == "xtxt" && self.doc.nodes.is_empty() {
            self.doc.version = args.resolve("version").to_string();
            self.i += 1;
            return;
        }

        // A directive is fenced if a matching @end<name> line follows, which
        // keeps the rule local: no registry of block names to keep in sync.
        if let Some(end) = self.find_fence(&name, self.i + 1) {
            let body: Vec<String> = self.lines[self.i + 1..end].to_vec();
            self.i = end + 1;
            let mut n = Node::new(Kind::Block, start + 1);
            n.name = name;
            n.args = args;
            n.text = trim_fence_body(&body);
            self.doc.nodes.push(n);
            return;
        }

        if FENCED_BY_DEFAULT.contains(&name.as_str()) {
            self.err(
                start + 1,
                format!("unclosed @{name} block: no matching @end{name}"),
            );
        }
        self.i += 1;
        let mut n = Node::new(Kind::Directive, start + 1);
        n.name = name;
        n.args = args;
        self.doc.nodes.push(n);
    }

    fn find_fence(&self, name: &str, from: usize) -> Option<usize> {
        let closer = format!("@end{name}");
        (from..self.lines.len()).find(|&j| self.lines[j].trim_end_matches([' ', '\t']) == closer)
    }

    /// Reads an argument list, consuming further lines if it spans them.
    /// Advances `self.i` to the last consumed line only on success.
    fn parse_args(&mut self, rest: &str) -> Option<Args> {
        let trimmed = rest.trim();
        if !trimmed.starts_with('(') {
            return Some(Args::default());
        }
        let mut buf = trimmed.to_string();
        let mut line = self.i;
        loop {
            if let Some(inner) = balanced(&buf) {
                self.i = line;
                return Some(split_args(&inner));
            }
            line += 1;
            if line >= self.lines.len() {
                return None;
            }
            buf.push('\n');
            buf.push_str(&self.lines[line]);
        }
    }

    fn quote(&mut self) {
        let start = self.i;
        let mut parts = Vec::new();
        while self.i < self.lines.len() {
            let t = self.lines[self.i].trim().to_string();
            if !t.starts_with('>') {
                break;
            }
            parts.push(t[1..].trim().to_string());
            self.i += 1;
        }
        let mut n = Node::new(Kind::Quote, start + 1);
        n.text = parts.join(" ");
        self.doc.nodes.push(n);
    }

    fn list(&mut self) {
        let start = self.i;
        let mut items = Vec::new();
        let mut base_indent: Option<usize> = None;
        let mut flagged = false;
        while self.i < self.lines.len() {
            let line = self.lines[self.i].clone();
            let pre = item_prefix(&line).to_string();
            if pre.is_empty() {
                break;
            }
            // Lists do not nest (SPEC 3.4), so an item indented deeper than the
            // first is structure about to be lost. Flattening it silently turns
            // a formatting mistake into data loss; uniform indent is style.
            let indent = line.len() - line.trim_start_matches([' ', '\t']).len();
            match base_indent {
                None => base_indent = Some(indent),
                Some(base) if indent > base && !flagged => {
                    self.warn(
                        self.i + 1,
                        "list item is indented deeper than the first: XTXT lists do not nest, \
                         so it is flattened"
                            .to_string(),
                    );
                    flagged = true;
                }
                _ => {}
            }
            let mut body = line.trim_start_matches([' ', '\t'])[pre.len()..]
                .trim()
                .to_string();
            let mut item = Item {
                ordered: pre.as_bytes()[0].is_ascii_digit(),
                ..Default::default()
            };
            let b = body.as_bytes();
            if b.len() >= 3 && b[0] == b'[' && b[2] == b']' && matches!(b[1], b' ' | b'x' | b'X') {
                item.checked = Some(b[1] != b' ');
                body = body[3..].trim().to_string();
            }
            item.text = body;
            items.push(item);
            self.i += 1;
        }
        let mut n = Node::new(Kind::List, start + 1);
        n.items = items;
        self.doc.nodes.push(n);
    }

    fn paragraph(&mut self) {
        let start = self.i;
        let mut parts = Vec::new();
        while self.i < self.lines.len() {
            let line = self.lines[self.i].clone();
            if is_blank(&line)
                || heading_level(&line) > 0
                || is_directive(&line)
                || !item_prefix(&line).is_empty()
                || line.trim_start().starts_with('>')
            {
                break;
            }
            let t = line.trim();
            let t = t
                .strip_prefix('\\')
                .filter(|r| r.starts_with('@'))
                .unwrap_or(t);
            parts.push(t.to_string());
            self.i += 1;
        }
        let mut n = Node::new(Kind::Paragraph, start + 1);
        n.text = parts.join(" ");
        self.doc.nodes.push(n);
    }
}

/// The text inside the outermost parens, if `s` closes them.
fn balanced(s: &str) -> Option<String> {
    let b = s.as_bytes();
    let (mut depth, mut in_quote, mut esc) = (0i32, false, false);
    for (i, &c) in b.iter().enumerate() {
        if esc {
            esc = false;
        } else if c == b'\\' {
            esc = true;
        } else if in_quote {
            if c == b'"' {
                in_quote = false;
            }
        } else if c == b'"' {
            in_quote = true;
        } else if c == b'(' {
            depth += 1;
        } else if c == b')' {
            depth -= 1;
            if depth == 0 {
                return Some(s[1..i].to_string());
            }
        }
    }
    None
}

/// Splits on `sep`, ignoring separators inside quotes or nested parens.
fn split_top(s: &str, sep: u8) -> Vec<&str> {
    let b = s.as_bytes();
    let mut out = Vec::new();
    let (mut depth, mut in_quote, mut esc, mut start) = (0i32, false, false, 0usize);
    for (i, &c) in b.iter().enumerate() {
        if esc {
            esc = false;
        } else if c == b'\\' {
            esc = true;
        } else if in_quote {
            if c == b'"' {
                in_quote = false;
            }
        } else if c == b'"' {
            in_quote = true;
        } else if c == b'(' {
            depth += 1;
        } else if c == b')' {
            depth -= 1;
        } else if c == sep && depth == 0 {
            out.push(&s[start..i]);
            start = i + 1;
        }
    }
    out.push(&s[start..]);
    out
}

fn is_name(s: &str) -> bool {
    let s = s.trim();
    let b = s.as_bytes();
    !b.is_empty() && is_name_start(b[0]) && b[1..].iter().all(|&c| is_name_byte(c))
}

fn split_args(s: &str) -> Args {
    let mut args = Vec::new();
    for field in split_top(s, b',') {
        let field = field.trim();
        if field.is_empty() {
            continue;
        }
        let parts = split_top(field, b'=');
        let (key, val) = if parts.len() >= 2 && is_name(parts[0]) {
            (parts[0].trim(), field[parts[0].len() + 1..].trim())
        } else {
            ("", field)
        };
        args.push(Arg {
            key: key.to_string(),
            value: unquote(val),
        });
    }
    Args(args)
}

fn unquote(s: &str) -> String {
    let s = s.trim();
    if s.len() >= 2 && s.starts_with('"') && s.ends_with('"') {
        return unescape(&s[1..s.len() - 1]);
    }
    s.to_string()
}

fn unescape(s: &str) -> String {
    if !s.contains('\\') {
        return s.to_string();
    }
    let mut out = String::with_capacity(s.len());
    let mut chars = s.chars();
    while let Some(c) = chars.next() {
        if c == '\\' {
            match chars.next() {
                Some('n') => out.push('\n'),
                Some('t') => out.push('\t'),
                Some(other) => out.push(other),
                None => out.push('\\'),
            }
        } else {
            out.push(c);
        }
    }
    out
}

fn trim_fence_body(body: &[String]) -> String {
    let mut body = body;
    if body.first().map(|l| is_blank(l)).unwrap_or(false) {
        body = &body[1..];
    }
    if body.last().map(|l| is_blank(l)).unwrap_or(false) {
        body = &body[..body.len() - 1];
    }
    body.iter()
        .map(|l| {
            if l.trim_start_matches([' ', '\t']).starts_with("\\@end") {
                l.replacen("\\@end", "@end", 1)
            } else {
                l.clone()
            }
        })
        .collect::<Vec<_>>()
        .join("\n")
}

fn parse_metadata(payload: &str) -> HashMap<String, String> {
    let mut out = HashMap::new();
    for line in payload.split('\n') {
        if is_blank(line) {
            continue;
        }
        if let Some((k, v)) = line.split_once('=') {
            out.entry(k.trim().to_lowercase())
                .or_insert_with(|| v.trim().to_string());
        }
    }
    out
}

// ---------------------------------------------------------------------------
// Tables
// ---------------------------------------------------------------------------

/// The interpreted payload of an `@table` block.
#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct Table {
    pub header: Vec<String>,
    pub rows: Vec<Vec<String>>,
    /// "left", "right" or "center" per column.
    pub align: Vec<String>,
}

fn split_cells(line: &str) -> Vec<String> {
    line.trim()
        .trim_matches('|')
        .split('|')
        .map(|c| c.trim().to_string())
        .collect()
}

fn is_separator_row(cells: &[String]) -> bool {
    !cells.is_empty()
        && cells
            .iter()
            .all(|c| !c.is_empty() && c.chars().all(|ch| ch == '-' || ch == ':'))
}

pub fn parse_table(n: &Node) -> Table {
    let mut t = Table::default();
    let mut rows: Vec<Vec<String>> = Vec::new();
    let mut sep_at: Option<usize> = None;

    for line in n.text.split('\n') {
        if is_blank(line) {
            continue;
        }
        let cells = split_cells(line);
        if sep_at.is_none() && is_separator_row(&cells) {
            sep_at = Some(rows.len());
            t.align = cells
                .iter()
                .map(|c| {
                    match (c.starts_with(':'), c.ends_with(':')) {
                        (true, true) => "center",
                        (false, true) => "right",
                        _ => "left",
                    }
                    .to_string()
                })
                .collect();
            continue;
        }
        rows.push(cells);
    }
    if rows.is_empty() {
        return t;
    }
    let sep = match sep_at {
        Some(n) if n > 0 => n,
        _ => 1,
    };
    t.header = if sep > 1 {
        rows[sep - 1].clone()
    } else {
        rows[0].clone()
    };
    t.rows = rows[sep.min(rows.len())..].to_vec();
    t
}

// ---------------------------------------------------------------------------
// Records
// ---------------------------------------------------------------------------

/// One `Key: value` entry in a block payload.
#[derive(Debug, Clone, PartialEq, Eq, Default)]
pub struct Field {
    pub key: String,
    pub value: String,
}

/// A block payload read as an ordered record. Order matters: a `@chat` block's
/// turns are fields, and their sequence is the conversation.
#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct Fields(pub Vec<Field>);

impl Fields {
    pub fn get(&self, key: &str) -> &str {
        self.0
            .iter()
            .find(|f| f.key.eq_ignore_ascii_case(key))
            .map(|f| f.value.as_str())
            .unwrap_or("")
    }

    /// Flatten to a lowercase-keyed map, keeping the first of any duplicate.
    pub fn map(&self) -> HashMap<String, String> {
        let mut out = HashMap::new();
        for f in &self.0 {
            out.entry(f.key.to_lowercase())
                .or_insert_with(|| f.value.clone());
        }
        out
    }

    pub fn is_empty(&self) -> bool {
        self.0.is_empty()
    }

    pub fn len(&self) -> usize {
        self.0.len()
    }

    pub fn iter(&self) -> std::slice::Iter<'_, Field> {
        self.0.iter()
    }
}

/// A field key is a label, not a sentence: these caps are what keep ordinary
/// prose containing a colon from being read as a record field.
const MAX_FIELD_KEY_LEN: usize = 32;
const MAX_FIELD_KEY_WORDS: usize = 3;

fn is_field_line(line: &str) -> Option<(String, String)> {
    let b = line.as_bytes();
    for (i, &c) in b.iter().enumerate() {
        if i > MAX_FIELD_KEY_LEN {
            break;
        }
        if c == b':' || c == b'=' {
            let k = line[..i].trim();
            if k.is_empty()
                || !is_name_start(k.as_bytes()[0])
                || k.split_whitespace().count() > MAX_FIELD_KEY_WORDS
            {
                return None;
            }
            return Some((k.to_string(), line[i + 1..].trim().to_string()));
        }
        if !(c == b' ' || c == b'_' || c == b'-' || c == b'.' || is_name_byte(c)) {
            return None;
        }
    }
    None
}

/// Interpret a block payload as an ordered record. Lines before the first field
/// become the empty key, so nothing is lost.
pub fn parse_fields(payload: &str) -> Fields {
    let mut out: Vec<Field> = Vec::new();
    let mut preamble: Vec<&str> = Vec::new();

    for line in payload.split('\n') {
        if let Some((key, value)) = is_field_line(line) {
            out.push(Field { key, value });
            continue;
        }
        match out.last_mut() {
            None => preamble.push(line),
            Some(cur) => {
                if cur.value.is_empty() {
                    cur.value = line.trim().to_string();
                } else {
                    cur.value.push('\n');
                    cur.value.push_str(line);
                }
            }
        }
    }
    for f in &mut out {
        f.value = f.value.trim().to_string();
    }
    let text = preamble.join("\n").trim().to_string();
    if !text.is_empty() {
        out.insert(
            0,
            Field {
                key: String::new(),
                value: text,
            },
        );
    }
    Fields(out)
}

// ---------------------------------------------------------------------------
// Inline formatting
// ---------------------------------------------------------------------------

/// Parses a marker `[^id]` starting at `s[i] == '['`.
fn footnote_ref(s: &str, i: usize) -> Option<(String, usize)> {
    let b = s.as_bytes();
    if i + 2 >= b.len() || b[i + 1] != b'^' {
        return None;
    }
    let close = s[i + 2..].find(']')? + i + 2;
    if close == i + 2 {
        return None;
    }
    let id = &s[i + 2..close];
    if id.contains(' ') || id.contains('\t') {
        return None;
    }
    Some((id.to_string(), close))
}

/// The index of the next unescaped `mark` at or after `from`.
fn find_close(s: &str, from: usize, mark: &str) -> Option<usize> {
    let b = s.as_bytes();
    let m = mark.as_bytes();
    let mut i = from;
    while i + m.len() <= b.len() {
        if b[i] == b'\\' {
            i += 2;
            continue;
        }
        if b[i..].starts_with(m) {
            return if i == from { None } else { Some(i) };
        }
        i += 1;
    }
    None
}

/// Parses `[label](target)` starting at `s[i] == '['`.
fn parse_link(s: &str, i: usize) -> Option<(String, String, usize)> {
    let close = find_close(s, i + 1, "]")?;
    let b = s.as_bytes();
    if close + 1 >= b.len() || b[close + 1] != b'(' {
        return None;
    }
    let inner = balanced(&s[close + 1..])?;
    Some((
        s[i + 1..close].to_string(),
        inner.trim().to_string(),
        close + 1 + inner.len() + 1,
    ))
}

/// Writes one byte, escaping the five characters that matter in HTML.
///
/// It works a byte at a time rather than converting to a char, because the scan
/// is byte-oriented: turning a UTF-8 continuation byte into a char would mangle
/// every non-ASCII document. Bytes above ASCII pass through untouched.
fn escape_byte(out: &mut Vec<u8>, c: u8) {
    match c {
        b'&' => out.extend_from_slice(b"&amp;"),
        b'<' => out.extend_from_slice(b"&lt;"),
        b'>' => out.extend_from_slice(b"&gt;"),
        b'"' => out.extend_from_slice(b"&#34;"),
        b'\'' => out.extend_from_slice(b"&#39;"),
        _ => out.push(c),
    }
}

fn escape_str(s: &str) -> String {
    let mut out = Vec::with_capacity(s.len());
    for &c in s.as_bytes() {
        escape_byte(&mut out, c);
    }
    String::from_utf8(out).expect("escaping preserves UTF-8")
}

/// Convert inline markup to HTML, escaping everything else.
pub fn inline_html(s: &str) -> String {
    let b = s.as_bytes();
    let mut out: Vec<u8> = Vec::with_capacity(s.len());
    let mut i = 0;
    while i < b.len() {
        let c = b[i];
        if c == b'\\' && i + 1 < b.len() {
            i += 1;
            escape_byte(&mut out, b[i]);
        } else if c == b'`' {
            match s[i + 1..].find('`') {
                Some(rel) => {
                    let end = i + 1 + rel;
                    out.extend_from_slice(b"<code>");
                    out.extend_from_slice(escape_str(&s[i + 1..end]).as_bytes());
                    out.extend_from_slice(b"</code>");
                    i = end;
                }
                None => out.extend_from_slice(b"&#96;"),
            }
        } else if c == b'*' {
            let mark = if s[i..].starts_with("**") { "**" } else { "*" };
            let tag = if mark == "**" { "strong" } else { "em" };
            match find_close(s, i + mark.len(), mark) {
                Some(end) => {
                    out.extend_from_slice(format!("<{tag}>").as_bytes());
                    out.extend_from_slice(inline_html(&s[i + mark.len()..end]).as_bytes());
                    out.extend_from_slice(format!("</{tag}>").as_bytes());
                    i = end + mark.len() - 1;
                }
                None => out.push(b'*'),
            }
        } else if c == b'[' {
            if let Some((id, end)) = footnote_ref(s, i) {
                let e = escape_str(&id);
                out.extend_from_slice(
                    format!(
                        "<sup class=\"fnref\" id=\"fnref-{e}\"><a href=\"#fn-{e}\">{e}</a></sup>"
                    )
                    .as_bytes(),
                );
                i = end;
            } else if let Some((label, target, end)) = parse_link(s, i) {
                out.extend_from_slice(
                    format!(
                        "<a href=\"{}\">{}</a>",
                        escape_str(&target),
                        inline_html(&label)
                    )
                    .as_bytes(),
                );
                i = end;
            } else {
                out.push(b'[');
            }
        } else {
            escape_byte(&mut out, c);
        }
        i += 1;
    }
    String::from_utf8(out).expect("escaping preserves UTF-8")
}

/// Strip inline markup, for plain text and analysis.
pub fn inline_text(s: &str) -> String {
    let b = s.as_bytes();
    let mut out: Vec<u8> = Vec::with_capacity(s.len());
    let mut i = 0;
    while i < b.len() {
        let c = b[i];
        if c == b'\\' && i + 1 < b.len() {
            i += 1;
            out.push(b[i]);
        } else if c == b'`' {
            match s[i + 1..].find('`') {
                Some(rel) => {
                    let end = i + 1 + rel;
                    out.extend_from_slice(&s.as_bytes()[i + 1..end]);
                    i = end;
                }
                None => out.push(c),
            }
        } else if c == b'*' {
            let mark = if s[i..].starts_with("**") { "**" } else { "*" };
            match find_close(s, i + mark.len(), mark) {
                Some(end) => {
                    out.extend_from_slice(inline_text(&s[i + mark.len()..end]).as_bytes());
                    i = end + mark.len() - 1;
                }
                None => out.push(c),
            }
        } else if c == b'[' {
            if let Some((id, end)) = footnote_ref(s, i) {
                out.extend_from_slice(format!("[{id}]").as_bytes());
                i = end;
            } else if let Some((label, _, end)) = parse_link(s, i) {
                out.extend_from_slice(inline_text(&label).as_bytes());
                i = end;
            } else {
                out.push(c);
            }
        } else {
            out.push(c);
        }
        i += 1;
    }
    String::from_utf8(out).expect("byte copy preserves UTF-8")
}

// ---------------------------------------------------------------------------
// Validation
// ---------------------------------------------------------------------------

/// The standard directives and whether each is a fenced block. Anything absent
/// is unknown: a warning, never an error, so a 1.0 reader stays usable on a
/// newer document.
fn known(name: &str) -> Option<bool> {
    Some(match name {
        "xtxt" | "image" | "video" | "audio" | "attachment" | "include" | "embed" | "hr" => false,
        "code" | "table" | "math" | "mermaid" | "metadata" | "comment" | "raw" | "chart"
        | "footnote" | "task" | "decision" | "knowledge" | "ai" | "prompt" | "chat" | "note" => {
            true
        }
        _ => return None,
    })
}

fn requires_src(name: &str) -> bool {
    matches!(name, "image" | "video" | "audio" | "attachment" | "include")
}

/// Semantic checks on top of the parser's syntactic ones.
///
/// Names of directives supplied by plugins may be passed in `declared`: a
/// plugin manifest is a declaration that the directive exists.
pub fn validate_with(doc: &Document, declared: &[&str]) -> Vec<Issue> {
    let mut issues = Vec::new();
    let mut metadata_seen = false;
    let mut notes: Vec<(String, usize)> = Vec::new();
    let extra: HashSet<&str> = declared.iter().copied().collect();

    macro_rules! warn {
        ($line:expr, $msg:expr) => {
            issues.push(Issue {
                severity: Severity::Warning,
                line: $line,
                message: $msg,
            })
        };
    }

    for n in &doc.nodes {
        if n.kind != Kind::Directive && n.kind != Kind::Block {
            continue;
        }
        let fenced = match known(&n.name) {
            Some(f) => f,
            None => {
                if !extra.contains(n.name.as_str()) {
                    warn!(
                        n.line,
                        format!(
                            "unknown directive @{} (preserved, but this reader cannot render it)",
                            n.name
                        )
                    );
                }
                continue;
            }
        };
        if fenced && n.kind != Kind::Block {
            warn!(
                n.line,
                format!(
                    "@{} is a block directive and should be closed with @end{}",
                    n.name, n.name
                )
            );
        }
        if !fenced && n.kind == Kind::Block {
            warn!(
                n.line,
                format!(
                    "@{} is not a block directive but was closed with @end{}",
                    n.name, n.name
                )
            );
        }
        if requires_src(&n.name) && n.args.resolve("src").is_empty() {
            warn!(n.line, format!("@{} has no src", n.name));
        }

        match n.name.as_str() {
            "metadata" => {
                if metadata_seen {
                    warn!(n.line, "duplicate @metadata block".to_string());
                }
                metadata_seen = true;
            }
            "table" => {
                let t = parse_table(n);
                if t.header.is_empty() {
                    warn!(n.line, "@table is empty".to_string());
                } else {
                    for (i, row) in t.rows.iter().enumerate() {
                        if row.len() != t.header.len() {
                            warn!(
                                n.line + 1 + i,
                                format!(
                                    "table row has {} cells, header has {}",
                                    row.len(),
                                    t.header.len()
                                )
                            );
                        }
                    }
                }
            }
            "code" => {
                if n.args.resolve("language").is_empty() {
                    warn!(
                        n.line,
                        "@code has no language; syntax highlighting will be skipped".to_string()
                    );
                }
            }
            "chart" => {
                if chart_rows(n) == 0 {
                    warn!(n.line, "@chart has no readable data rows".to_string());
                }
            }
            "task" => {
                if n.fields().get("title").is_empty() {
                    warn!(n.line, "@task has no Title field".to_string());
                }
            }
            "footnote" => {
                let id = n.args.resolve("id").to_string();
                if id.is_empty() {
                    warn!(
                        n.line,
                        "@footnote has no id; references cannot point at it".to_string()
                    );
                }
                notes.push((id, n.line));
            }
            _ => {}
        }
    }

    issues.extend(check_footnote_refs(doc, &notes));
    issues
}

/// Semantic checks with no plugin-declared directives.
pub fn validate(doc: &Document) -> Vec<Issue> {
    validate_with(doc, &[])
}

fn chart_rows(n: &Node) -> usize {
    n.text
        .split('\n')
        .filter(|line| !is_blank(line))
        .filter(|line| {
            let cells: Vec<String> = if line.contains('|') {
                split_cells(line)
            } else {
                let f: Vec<&str> = line.split_whitespace().collect();
                if f.len() >= 2 {
                    vec![f[..f.len() - 1].join(" "), f[f.len() - 1].to_string()]
                } else {
                    f.iter().map(|s| s.to_string()).collect()
                }
            };
            cells.len() >= 2 && !is_separator_row(&cells)
        })
        .count()
}

fn check_footnote_refs(doc: &Document, notes: &[(String, usize)]) -> Vec<Issue> {
    let mut issues = Vec::new();
    let mut cited: HashSet<String> = HashSet::new();
    let ids: HashSet<&str> = notes.iter().map(|(id, _)| id.as_str()).collect();

    let mut visit = |text: &str, line: usize| {
        let b = text.as_bytes();
        let mut i = 0;
        while i < b.len() {
            if b[i] == b'\\' {
                i += 2;
                continue;
            }
            if b[i] == b'[' {
                if let Some((id, end)) = footnote_ref(text, i) {
                    cited.insert(id.clone());
                    if !ids.contains(id.as_str()) {
                        issues.push(Issue {
                            severity: Severity::Warning,
                            line,
                            message: format!(
                                "footnote reference [^{id}] has no matching @footnote(id=\"{id}\")"
                            ),
                        });
                    }
                    i = end;
                }
            }
            i += 1;
        }
    };

    for n in &doc.nodes {
        match n.kind {
            Kind::Heading | Kind::Paragraph | Kind::Quote => visit(&n.text, n.line),
            Kind::List => {
                for it in &n.items {
                    visit(&it.text, n.line)
                }
            }
            _ => {}
        }
    }
    for (id, line) in notes {
        if !id.is_empty() && !cited.contains(id) {
            issues.push(Issue {
                severity: Severity::Warning,
                line: *line,
                message: format!("@footnote(id=\"{id}\") is never referenced"),
            });
        }
    }
    issues
}

/// Order issues by line for stable reporting.
pub fn sort_issues(issues: &[Issue]) -> Vec<Issue> {
    let mut out = issues.to_vec();
    out.sort_by_key(|i| i.line);
    out
}

// ---------------------------------------------------------------------------
// Extraction
// ---------------------------------------------------------------------------

/// One heading in the table of contents.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Outline {
    pub level: usize,
    pub text: String,
    pub line: usize,
}

/// A structured directive as an agent sees it.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Block {
    pub block_type: String,
    pub line: usize,
    pub args: HashMap<String, String>,
    pub fields: HashMap<String, String>,
    /// Field keys in source order.
    pub order: Vec<String>,
    pub text: String,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Link {
    pub text: String,
    pub href: String,
    pub line: usize,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Media {
    pub kind: String,
    pub src: String,
    pub caption: String,
    pub line: usize,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Code {
    pub language: String,
    pub lines: usize,
    pub line: usize,
    pub source: String,
}

/// A unit of work, from either a `@task` block or a checklist item.
#[derive(Debug, Clone, PartialEq, Eq, Default)]
pub struct Task {
    pub title: String,
    pub done: bool,
    pub status: String,
    pub owner: String,
    pub due: String,
    pub line: usize,
}

/// The machine-facing view of a document: everything an agent needs without
/// having to infer structure from prose.
#[derive(Debug, Clone, Default)]
pub struct Extraction {
    pub version: String,
    pub metadata: HashMap<String, String>,
    pub outline: Vec<Outline>,
    pub tasks: Vec<Task>,
    pub blocks: Vec<Block>,
    pub links: Vec<Link>,
    pub media: Vec<Media>,
    pub code: Vec<Code>,
    pub text: String,
    pub words: usize,
}

fn is_presentational(name: &str) -> bool {
    matches!(
        name,
        "code"
            | "table"
            | "math"
            | "mermaid"
            | "metadata"
            | "comment"
            | "raw"
            | "image"
            | "video"
            | "audio"
            | "attachment"
            | "hr"
            | "xtxt"
            | "include"
            | "embed"
            | "footnote"
    )
}

fn links_in(s: &str, line: usize) -> Vec<Link> {
    let b = s.as_bytes();
    let mut out = Vec::new();
    let mut i = 0;
    while i < b.len() {
        if b[i] == b'\\' {
            i += 2;
            continue;
        }
        if b[i] == b'[' {
            if let Some((label, target, end)) = parse_link(s, i) {
                out.push(Link {
                    text: inline_text(&label),
                    href: target,
                    line,
                });
                i = end;
            }
        }
        i += 1;
    }
    out
}

/// Build the machine-facing view of a document.
pub fn extract(doc: &Document) -> Extraction {
    let mut e = Extraction {
        version: doc.version.clone(),
        metadata: doc.metadata(),
        ..Default::default()
    };
    let mut prose: Vec<String> = Vec::new();

    for n in &doc.nodes {
        match n.kind {
            Kind::Heading => {
                let text = inline_text(&n.text);
                e.outline.push(Outline {
                    level: n.level,
                    text: text.clone(),
                    line: n.line,
                });
                prose.push(text);
                e.links.extend(links_in(&n.text, n.line));
            }
            Kind::Paragraph | Kind::Quote => {
                prose.push(inline_text(&n.text));
                e.links.extend(links_in(&n.text, n.line));
            }
            Kind::List => {
                for it in &n.items {
                    prose.push(inline_text(&it.text));
                    e.links.extend(links_in(&it.text, n.line));
                    if let Some(done) = it.checked {
                        e.tasks.push(Task {
                            title: inline_text(&it.text),
                            done,
                            line: n.line,
                            ..Default::default()
                        });
                    }
                }
            }
            Kind::Directive | Kind::Block => absorb(&mut e, n, &mut prose),
        }
    }

    e.text = prose.join("\n\n");
    e.words = e.text.split_whitespace().count();
    e
}

fn absorb(e: &mut Extraction, n: &Node, prose: &mut Vec<String>) {
    match n.name.as_str() {
        "comment" | "metadata" => return,
        "image" | "video" | "audio" | "attachment" => {
            e.media.push(Media {
                kind: n.name.clone(),
                src: n.args.resolve("src").to_string(),
                caption: inline_text(n.args.get("caption")),
                line: n.line,
            });
            if !n.args.get("caption").is_empty() {
                prose.push(inline_text(n.args.get("caption")));
            }
            return;
        }
        "code" => {
            e.code.push(Code {
                language: n.args.resolve("language").to_string(),
                lines: n.text.matches('\n').count() + 1,
                line: n.line,
                source: n.text.clone(),
            });
            return;
        }
        "table" => {
            let t = parse_table(n);
            prose.push(t.header.join(" | "));
            for row in &t.rows {
                prose.push(row.join(" | "));
            }
            return;
        }
        _ => {}
    }

    if is_presentational(&n.name) {
        if n.kind == Kind::Block && !n.text.is_empty() {
            prose.push(n.text.clone());
        }
        return;
    }

    let f = n.fields();
    let mut block = Block {
        block_type: n.name.clone(),
        line: n.line,
        args: HashMap::new(),
        fields: HashMap::new(),
        order: Vec::new(),
        text: n.text.clone(),
    };
    for (i, a) in n.args.iter().enumerate() {
        let key = if a.key.is_empty() {
            i.to_string()
        } else {
            a.key.clone()
        };
        block.args.insert(key, a.value.clone());
    }
    if !f.is_empty() {
        block.fields = f.map();
        block.order = f.iter().map(|x| x.key.clone()).collect();
    }
    e.blocks.push(block);

    if n.name == "task" {
        let m = f.map();
        let empty = String::new();
        let title = m
            .get("title")
            .filter(|s| !s.is_empty())
            .or_else(|| m.get(""))
            .unwrap_or(&empty)
            .clone();
        let status = m.get("status").unwrap_or(&empty).clone();
        e.tasks.push(Task {
            done: status.eq_ignore_ascii_case("done") || status.eq_ignore_ascii_case("complete"),
            title,
            status,
            owner: m.get("owner").unwrap_or(&empty).clone(),
            due: m.get("due").unwrap_or(&empty).clone(),
            line: n.line,
        });
    }
    if n.kind == Kind::Block && !n.text.is_empty() {
        prose.push(n.text.clone());
    }
}

// ---------------------------------------------------------------------------
// HTML
// ---------------------------------------------------------------------------

/// Render a document to an HTML fragment.
pub fn render_html(doc: &Document) -> String {
    let mut body: Vec<String> = Vec::new();
    let mut notes: Vec<&Node> = Vec::new();

    for n in &doc.nodes {
        match n.kind {
            Kind::Heading => body.push(format!("<h{0}>{1}</h{0}>", n.level, inline_html(&n.text))),
            Kind::Paragraph => body.push(format!("<p>{}</p>", inline_html(&n.text))),
            Kind::Quote => body.push(format!(
                "<blockquote><p>{}</p></blockquote>",
                inline_html(&n.text)
            )),
            Kind::List => body.push(list_html(n)),
            Kind::Directive | Kind::Block => {
                if n.name == "footnote" {
                    notes.push(n);
                } else {
                    body.push(directive_html(n));
                }
            }
        }
    }

    if !notes.is_empty() {
        let items: String = notes
            .iter()
            .enumerate()
            .map(|(i, n)| {
                let id = n.args.resolve("id");
                let id = if id.is_empty() {
                    (i + 1).to_string()
                } else {
                    id.to_string()
                };
                let e = escape_str(&id);
                format!(
                    "<li id=\"fn-{e}\">{} <a class=\"fnback\" href=\"#fnref-{e}\">&#8617;</a></li>",
                    inline_html(&n.text)
                )
            })
            .collect();
        body.push(format!(
            "<section class=\"footnotes\"><ol>{items}</ol></section>"
        ));
    }

    body.retain(|s| !s.is_empty());
    body.join("\n")
}

fn list_html(n: &Node) -> String {
    if n.items.is_empty() {
        return String::new();
    }
    let tag = if n.items[0].ordered { "ol" } else { "ul" };
    let class = if n.items[0].checked.is_some() {
        " class=\"checklist\""
    } else {
        ""
    };
    let items: String = n
        .items
        .iter()
        .map(|it| {
            let box_ = match it.checked {
                Some(true) => "<input type=\"checkbox\" disabled checked> ",
                Some(false) => "<input type=\"checkbox\" disabled> ",
                None => "",
            };
            format!("<li>{box_}{}</li>", inline_html(&it.text))
        })
        .collect();
    format!("<{tag}{class}>{items}</{tag}>")
}

fn directive_html(n: &Node) -> String {
    let caption = n.args.get("caption");
    let figcap = if caption.is_empty() {
        String::new()
    } else {
        format!("<figcaption>{}</figcaption>", inline_html(caption))
    };

    match n.name.as_str() {
        "comment" | "metadata" => String::new(),
        "hr" => "<hr>".to_string(),
        "image" => {
            let alt = if n.args.get("alt").is_empty() {
                inline_text(caption)
            } else {
                n.args.get("alt").to_string()
            };
            let attrs: String = ["width", "height"]
                .iter()
                .filter(|k| !n.args.get(k).is_empty())
                .map(|k| format!(" {k}=\"{}\"", escape_str(n.args.get(k))))
                .collect();
            format!(
                "<figure><img src=\"{}\" alt=\"{}\"{attrs}>{figcap}</figure>",
                escape_str(n.args.resolve("src")),
                escape_str(&alt)
            )
        }
        "video" | "audio" => {
            let extra = if n.name == "video" {
                " controls playsinline"
            } else {
                " controls"
            };
            format!(
                "<figure><{0} src=\"{1}\"{extra}></{0}>{figcap}</figure>",
                n.name,
                escape_str(n.args.resolve("src"))
            )
        }
        "attachment" => {
            let src = n.args.resolve("src");
            let name = if n.args.get("name").is_empty() {
                src
            } else {
                n.args.get("name")
            };
            format!(
                "<p class=\"attachment\"><a href=\"{}\" download>{}</a></p>",
                escape_str(src),
                escape_str(name)
            )
        }
        "code" => {
            let lang = n.args.resolve("language");
            let class = if lang.is_empty() {
                String::new()
            } else {
                format!(" class=\"language-{}\"", escape_str(lang))
            };
            format!("<pre><code{class}>{}</code></pre>", escape_str(&n.text))
        }
        "math" => format!("<div class=\"math\">{}</div>", escape_str(&n.text)),
        "mermaid" => format!("<pre class=\"mermaid\">{}</pre>", escape_str(&n.text)),
        "raw" => {
            if n.args.resolve("format") == "html" {
                n.text.clone()
            } else {
                format!("<pre>{}</pre>", escape_str(&n.text))
            }
        }
        "table" => table_html(n),
        _ => {
            if n.kind == Kind::Block {
                let f = n.fields();
                if !f.is_empty() {
                    let rows: String = f
                        .iter()
                        .map(|x| {
                            let key = if x.key.is_empty() { "—" } else { &x.key };
                            format!(
                                "<dt>{}</dt><dd>{}</dd>",
                                escape_str(key),
                                inline_html(&x.value)
                            )
                        })
                        .collect();
                    return format!(
                        "<section class=\"record\" data-type=\"{0}\"><h4 class=\"record-type\">{0}</h4><dl>{rows}</dl></section>",
                        escape_str(&n.name)
                    );
                }
            }
            format!(
                "<div class=\"unknown\" data-directive=\"{}\">{}</div>",
                escape_str(&n.name),
                escape_str(&source_of(n))
            )
        }
    }
}

fn table_html(n: &Node) -> String {
    let t = parse_table(n);
    if t.header.is_empty() {
        return String::new();
    }
    let style = |i: usize| -> String {
        match t.align.get(i).map(String::as_str) {
            Some(a) if a != "left" => format!(" style=\"text-align:{a}\""),
            _ => String::new(),
        }
    };
    let head: String = t
        .header
        .iter()
        .enumerate()
        .map(|(i, h)| format!("<th{}>{}</th>", style(i), inline_html(h)))
        .collect();
    let rows: String = t
        .rows
        .iter()
        .map(|r| {
            let cells: String = r
                .iter()
                .enumerate()
                .map(|(i, c)| format!("<td{}>{}</td>", style(i), inline_html(c)))
                .collect();
            format!("<tr>{cells}</tr>")
        })
        .collect();
    format!("<table>\n<thead><tr>{head}</tr></thead>\n<tbody>{rows}</tbody>\n</table>")
}

/// Reconstruct a directive's source, for round-tripping and for showing
/// unknown directives.
pub fn source_of(n: &Node) -> String {
    let mut out = format!("@{}", n.name);
    if !n.args.is_empty() {
        let parts: Vec<String> = n
            .args
            .iter()
            .map(|a| {
                let v = if a.value.is_empty() || a.value.contains([' ', ',', ')', '"']) {
                    format!("\"{}\"", a.value.replace('\\', "\\\\").replace('"', "\\\""))
                } else {
                    a.value.clone()
                };
                if a.key.is_empty() {
                    v
                } else {
                    format!("{}={v}", a.key)
                }
            })
            .collect();
        out.push('(');
        out.push_str(&parts.join(", "));
        out.push(')');
    }
    if n.kind == Kind::Block {
        out.push('\n');
        out.push_str(&n.text);
        out.push_str(&format!("\n@end{}", n.name));
    }
    out
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn text_blocks() {
        let res = parse("# Title\n\nA para\nspanning lines.\n\n- one\n- [x] done\n");
        assert!(!res.has_errors());
        let kinds: Vec<Kind> = res.doc.nodes.iter().map(|n| n.kind).collect();
        assert_eq!(kinds, vec![Kind::Heading, Kind::Paragraph, Kind::List]);
        assert_eq!(res.doc.nodes[1].text, "A para spanning lines.");
        assert_eq!(res.doc.nodes[2].items[1].checked, Some(true));
    }

    #[test]
    fn multiline_directive() {
        let res = parse("@image(\n  src=\"a.png\",\n  caption=\"A, B\"\n)\n\nafter\n");
        assert_eq!(res.doc.nodes[0].args.resolve("src"), "a.png");
        assert_eq!(res.doc.nodes[0].args.get("caption"), "A, B");
        assert_eq!(res.doc.nodes[1].text, "after");
    }

    #[test]
    fn unknown_directives_are_not_errors() {
        let res = parse("@futurething(a=1)\n\n@newblock\nbody\n@endnewblock\n");
        assert!(!res.has_errors());
        assert_eq!(res.doc.nodes.len(), 2);
        assert_eq!(res.doc.nodes[1].text, "body");
        assert_eq!(validate(&res.doc).len(), 2);
    }

    #[test]
    fn inline_preserves_utf8() {
        for s in ["an em dash — here", "日本語", "café", "emoji 🎉"] {
            let got = inline_html(s);
            for ch in s.chars().filter(|c| !c.is_ascii()) {
                assert!(got.contains(ch), "{s:?} lost {ch:?}: {got:?}");
            }
        }
        assert_eq!(inline_html("a — b & c"), "a — b &amp; c");
    }

    #[test]
    fn prose_is_not_a_field() {
        let f = parse_fields("There is one rule that matters here: keep it readable.");
        assert_eq!(f.len(), 1);
        assert_eq!(f.0[0].key, "");
    }

    #[test]
    fn records_and_tasks() {
        let res = parse("@task\nTitle: Ship it\nStatus: Done\n@endtask\n");
        let e = extract(&res.doc);
        assert_eq!(e.tasks.len(), 1);
        assert_eq!(e.tasks[0].title, "Ship it");
        assert!(e.tasks[0].done);
        assert_eq!(e.blocks[0].order, vec!["Title", "Status"]);
    }
}
