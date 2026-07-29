"""XTXT — a plain-text document format with structure.

A pure-Python implementation of the format defined in SPEC.md. No dependencies.

    >>> import xtxt
    >>> doc = xtxt.parse("# Hello\\n\\nworld\\n").doc
    >>> [n.kind for n in doc.nodes]
    ['heading', 'paragraph']

This is a port, not a binding: it agrees with the Go reference implementation
because both are checked against the same conformance suite, not because one
calls the other.
"""

from __future__ import annotations

import html as _html
import json
import re
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any, Iterable

__version__ = "0.1.1"
__all__ = [
    "Arg", "Item", "Node", "Document", "Issue", "Result",
    "parse", "parse_file", "validate", "extract", "canonical",
    "render_html", "parse_table", "parse_fields", "inline_html", "inline_text",
]

# --------------------------------------------------------------------------
# Model
# --------------------------------------------------------------------------


@dataclass
class Arg:
    key: str
    value: str


@dataclass
class Item:
    text: str
    ordered: bool = False
    checked: bool | None = None


class Args(list):
    """A directive's arguments, in source order."""

    def get(self, key: str) -> str:
        for a in self:
            if a.key == key:
                return a.value
        return ""

    def has(self, key: str) -> bool:
        return any(a.key == key for a in self)

    def positional(self, i: int) -> str:
        n = 0
        for a in self:
            if a.key == "":
                if n == i:
                    return a.value
                n += 1
        return ""

    def resolve(self, key: str) -> str:
        """The named argument, falling back to the first positional one."""
        return self.get(key) or self.positional(0)


@dataclass
class Node:
    kind: str
    name: str = ""
    level: int = 0
    text: str = ""
    args: Args = field(default_factory=Args)
    items: list[Item] = field(default_factory=list)
    line: int = 0

    def fields(self) -> "Fields":
        """The payload read as an ordered `Key: value` record."""
        if self.kind != "block":
            return Fields()
        return parse_fields(self.text)


@dataclass
class Document:
    version: str = ""
    nodes: list[Node] = field(default_factory=list)

    def metadata(self) -> dict[str, str]:
        for n in self.nodes:
            if n.kind == "block" and n.name == "metadata":
                return _parse_metadata(n.text)
        return {}


@dataclass
class Issue:
    severity: str  # "error" or "warning"
    line: int
    message: str


@dataclass
class Result:
    doc: Document
    issues: list[Issue] = field(default_factory=list)

    def has_errors(self) -> bool:
        return any(i.severity == "error" for i in self.issues)


# --------------------------------------------------------------------------
# Parser
# --------------------------------------------------------------------------

_NAME_START = re.compile(r"[A-Za-z_]")
_NAME_BYTE = re.compile(r"[A-Za-z0-9_-]")

FENCED_BY_DEFAULT = {
    "code", "table", "math", "mermaid", "metadata", "comment", "raw",
}


def parse_file(path: str | Path) -> Result:
    return parse(Path(path).read_text(encoding="utf-8"))


def parse(src: str) -> Result:
    return _Parser(_read_lines(src)).run()


def _read_lines(src: str) -> list[str]:
    lines = src.replace("\r\n", "\n").split("\n")
    if lines and lines[-1] == "":
        lines.pop()
    if lines:
        lines[0] = lines[0].lstrip("﻿")
    return lines


def _is_blank(s: str) -> bool:
    return s.strip() == ""


def _heading_level(s: str) -> int:
    s = s.lstrip(" \t")
    n = 0
    while n < len(s) and s[n] == "#":
        n += 1
    if n == 0 or n > 6 or n >= len(s) or s[n] != " ":
        return 0
    return n


def _is_directive(s: str) -> bool:
    t = s.lstrip(" \t")
    return len(t) >= 2 and t[0] == "@" and bool(_NAME_START.match(t[1]))


def _directive_name(s: str) -> tuple[str, str]:
    t = s.lstrip(" \t")
    i = 1
    while i < len(t) and _NAME_BYTE.match(t[i]):
        i += 1
    return t[1:i], t[i:]


def _item_prefix(s: str) -> str:
    t = s.lstrip(" \t")
    if len(t) >= 2 and t[0] in "-*" and t[1] == " ":
        return t[:2]
    i = 0
    while i < len(t) and t[i].isdigit():
        i += 1
    if i > 0 and i + 1 < len(t) and t[i] == "." and t[i + 1] == " ":
        return t[: i + 2]
    return ""


class _Parser:
    def __init__(self, lines: list[str]):
        self.lines = lines
        self.i = 0
        self.doc = Document()
        self.issues: list[Issue] = []

    def run(self) -> Result:
        while self.i < len(self.lines):
            line = self.lines[self.i]
            if _is_blank(line):
                self.i += 1
            elif _is_directive(line):
                self._directive()
            elif _heading_level(line) > 0:
                lvl = _heading_level(line)
                self._emit(Node("heading", level=lvl,
                                text=line.lstrip(" \t")[lvl:].strip(), line=self.i + 1))
                self.i += 1
            elif line.strip().startswith(">"):
                self._quote()
            elif _item_prefix(line):
                self._list()
            else:
                self._paragraph()
        return Result(self.doc, self.issues)

    def _emit(self, n: Node) -> None:
        self.doc.nodes.append(n)

    def _err(self, line: int, msg: str) -> None:
        self.issues.append(Issue("error", line, msg))

    # -- directives --------------------------------------------------------

    def _directive(self) -> None:
        start = self.i
        name, rest = _directive_name(self.lines[self.i])

        args, ok = self._args(rest)
        if not ok:
            self._err(start + 1, f"unclosed argument list for @{name}")
            self.i = start + 1
            return

        if name.startswith("end"):
            self._err(start + 1, f"@{name} has no matching opening fence")
            self.i += 1
            return

        if name == "xtxt" and not self.doc.nodes:
            self.doc.version = args.resolve("version")
            self.i += 1
            return

        end = self._find_fence(name, self.i + 1)
        if end >= 0:
            body = self.lines[self.i + 1: end]
            self.i = end + 1
            self._emit(Node("block", name=name, args=args,
                            text=_trim_fence_body(body), line=start + 1))
            return

        if name in FENCED_BY_DEFAULT:
            self._err(start + 1, f"unclosed @{name} block: no matching @end{name}")
        self.i += 1
        self._emit(Node("directive", name=name, args=args, line=start + 1))

    def _find_fence(self, name: str, frm: int) -> int:
        closer = "@end" + name
        for j in range(frm, len(self.lines)):
            if self.lines[j].rstrip(" \t") == closer:
                return j
        return -1

    def _args(self, rest: str) -> tuple[Args, bool]:
        trimmed = rest.strip()
        if not trimmed.startswith("("):
            return Args(), True
        buf = trimmed
        line = self.i
        while True:
            inner, ok = _balanced(buf)
            if ok:
                self.i = line
                return _split_args(inner), True
            line += 1
            if line >= len(self.lines):
                return Args(), False
            buf += "\n" + self.lines[line]

    # -- text blocks -------------------------------------------------------

    def _quote(self) -> None:
        start = self.i
        parts = []
        while self.i < len(self.lines):
            t = self.lines[self.i].strip()
            if not t.startswith(">"):
                break
            parts.append(t[1:].strip())
            self.i += 1
        self._emit(Node("quote", text=" ".join(parts), line=start + 1))

    def _list(self) -> None:
        start = self.i
        items: list[Item] = []
        while self.i < len(self.lines):
            pre = _item_prefix(self.lines[self.i])
            if not pre:
                break
            body = self.lines[self.i].lstrip(" \t")[len(pre):].strip()
            it = Item(text="", ordered=pre[0].isdigit())
            if len(body) >= 3 and body[0] == "[" and body[2] == "]" and body[1] in " xX":
                it.checked = body[1] != " "
                body = body[3:].strip()
            it.text = body
            items.append(it)
            self.i += 1
        self._emit(Node("list", items=items, line=start + 1))

    def _paragraph(self) -> None:
        start = self.i
        parts = []
        while self.i < len(self.lines):
            line = self.lines[self.i]
            if (_is_blank(line) or _heading_level(line) > 0 or _is_directive(line)
                    or _item_prefix(line) or line.strip().startswith(">")):
                break
            t = line.strip()
            if t.startswith("\\@"):
                t = t[1:]
            parts.append(t)
            self.i += 1
        self._emit(Node("paragraph", text=" ".join(parts), line=start + 1))


def _balanced(s: str) -> tuple[str, bool]:
    depth = 0
    in_quote = esc = False
    for i, c in enumerate(s):
        if esc:
            esc = False
        elif c == "\\":
            esc = True
        elif in_quote:
            if c == '"':
                in_quote = False
        elif c == '"':
            in_quote = True
        elif c == "(":
            depth += 1
        elif c == ")":
            depth -= 1
            if depth == 0:
                return s[1:i], True
    return "", False


def _split_top(s: str, sep: str) -> list[str]:
    out = []
    depth = 0
    in_quote = esc = False
    start = 0
    for i, c in enumerate(s):
        if esc:
            esc = False
        elif c == "\\":
            esc = True
        elif in_quote:
            if c == '"':
                in_quote = False
        elif c == '"':
            in_quote = True
        elif c == "(":
            depth += 1
        elif c == ")":
            depth -= 1
        elif c == sep and depth == 0:
            out.append(s[start:i])
            start = i + 1
    out.append(s[start:])
    return out


def _is_name(s: str) -> bool:
    s = s.strip()
    return bool(s) and bool(_NAME_START.match(s[0])) and all(_NAME_BYTE.match(c) for c in s[1:])


def _split_args(s: str) -> Args:
    args = Args()
    for fieldstr in _split_top(s, ","):
        fieldstr = fieldstr.strip()
        if not fieldstr:
            continue
        parts = _split_top(fieldstr, "=")
        key, val = "", fieldstr
        if len(parts) >= 2 and _is_name(parts[0]):
            key = parts[0].strip()
            val = fieldstr[len(parts[0]) + 1:].strip()
        args.append(Arg(key, _unquote(val)))
    return args


def _unquote(s: str) -> str:
    s = s.strip()
    if len(s) >= 2 and s[0] == '"' and s[-1] == '"':
        return _unescape(s[1:-1])
    return s


def _unescape(s: str) -> str:
    if "\\" not in s:
        return s
    out = []
    i = 0
    while i < len(s):
        if s[i] == "\\" and i + 1 < len(s):
            i += 1
            out.append({"n": "\n", "t": "\t"}.get(s[i], s[i]))
        else:
            out.append(s[i])
        i += 1
    return "".join(out)


def _trim_fence_body(body: list[str]) -> str:
    body = list(body)
    if body and _is_blank(body[0]):
        body.pop(0)
    if body and _is_blank(body[-1]):
        body.pop()
    return "\n".join(
        l.replace("\\@end", "@end", 1) if l.lstrip(" \t").startswith("\\@end") else l
        for l in body
    )


def _parse_metadata(payload: str) -> dict[str, str]:
    out: dict[str, str] = {}
    for line in payload.split("\n"):
        if _is_blank(line) or "=" not in line:
            continue
        k, v = line.split("=", 1)
        out[k.strip().lower()] = v.strip()
    return out


# --------------------------------------------------------------------------
# Tables
# --------------------------------------------------------------------------


@dataclass
class Table:
    header: list[str] = field(default_factory=list)
    rows: list[list[str]] = field(default_factory=list)
    align: list[str] = field(default_factory=list)


def _split_cells(line: str) -> list[str]:
    return [c.strip() for c in line.strip().strip("|").split("|")]


def _is_separator_row(cells: list[str]) -> bool:
    return bool(cells) and all(c and not c.strip("-:") for c in cells)


def parse_table(n: Node) -> Table:
    t = Table()
    rows: list[list[str]] = []
    sep_at = -1
    for line in n.text.split("\n"):
        if _is_blank(line):
            continue
        cells = _split_cells(line)
        if sep_at < 0 and _is_separator_row(cells):
            sep_at = len(rows)
            t.align = [
                "center" if c.startswith(":") and c.endswith(":")
                else "right" if c.endswith(":") else "left"
                for c in cells
            ]
            continue
        rows.append(cells)
    if not rows:
        return t
    if sep_at <= 0:
        sep_at = 1
    t.header = rows[sep_at - 1] if sep_at > 1 else rows[0]
    t.rows = rows[sep_at:]
    return t


# --------------------------------------------------------------------------
# Records (Key: value payloads)
# --------------------------------------------------------------------------


class Fields(list):
    """An ordered `Key: value` record. Order matters: a @chat block's turns
    are fields, and their sequence is the conversation."""

    def get(self, key: str) -> str:
        for f in self:
            if f.key.lower() == key.lower():
                return f.value
        return ""

    def map(self) -> dict[str, str]:
        out: dict[str, str] = {}
        for f in self:
            out.setdefault(f.key.lower(), f.value)
        return out


@dataclass
class Field:
    key: str
    value: str


# A field key is a label, not a sentence: these caps are what keep ordinary
# prose containing a colon from being read as a record field.
MAX_FIELD_KEY_LEN = 32
MAX_FIELD_KEY_WORDS = 3


def _is_field_line(line: str) -> tuple[str, str, bool]:
    for i, c in enumerate(line):
        if i > MAX_FIELD_KEY_LEN:
            break
        if c in ":=":
            k = line[:i].strip()
            if not k or not _NAME_START.match(k[0]) or len(k.split()) > MAX_FIELD_KEY_WORDS:
                return "", "", False
            return k, line[i + 1:].strip(), True
        if not (c in " _-." or _NAME_BYTE.match(c)):
            return "", "", False
    return "", "", False


def parse_fields(payload: str) -> Fields:
    out = Fields()
    cur: Field | None = None
    preamble: list[str] = []
    for line in payload.split("\n"):
        k, v, ok = _is_field_line(line)
        if ok:
            cur = Field(k, v)
            out.append(cur)
            continue
        if cur is None:
            preamble.append(line)
            continue
        cur.value = line.strip() if cur.value == "" else cur.value + "\n" + line
    for f in out:
        f.value = f.value.strip()
    text = "\n".join(preamble).strip()
    if text:
        out.insert(0, Field("", text))
    return out


# --------------------------------------------------------------------------
# Inline formatting
# --------------------------------------------------------------------------


def _footnote_ref(s: str, i: int) -> tuple[str, int, bool]:
    if i + 2 >= len(s) or s[i + 1] != "^":
        return "", 0, False
    close = s.find("]", i + 2)
    if close <= i + 2 - 1 or close < 0:
        return "", 0, False
    ident = s[i + 2:close]
    if not ident or " " in ident or "\t" in ident:
        return "", 0, False
    return ident, close, True


def _find_close(s: str, frm: int, mark: str) -> int:
    i = frm
    while i + len(mark) <= len(s):
        if s[i] == "\\":
            i += 2
            continue
        if s.startswith(mark, i):
            return -1 if i == frm else i
        i += 1
    return -1


def _link(s: str, i: int) -> tuple[str, str, int, bool]:
    close = _find_close(s, i + 1, "]")
    if close < 0 or close + 1 >= len(s) or s[close + 1] != "(":
        return "", "", 0, False
    inner, ok = _balanced(s[close + 1:])
    if not ok:
        return "", "", 0, False
    return s[i + 1:close], inner.strip(), close + 1 + len(inner) + 1, True


def inline_html(s: str) -> str:
    """Convert inline markup to HTML, escaping everything else."""
    out: list[str] = []
    i = 0
    while i < len(s):
        c = s[i]
        if c == "\\" and i + 1 < len(s):
            i += 1
            out.append(_html.escape(s[i], quote=False))
        elif c == "`":
            end = s.find("`", i + 1)
            if end >= 0:
                out.append("<code>" + _html.escape(s[i + 1:end], quote=False) + "</code>")
                i = end
            else:
                out.append("&#96;")
        elif c == "*":
            mark = "**" if s.startswith("**", i) else "*"
            tag = "strong" if mark == "**" else "em"
            end = _find_close(s, i + len(mark), mark)
            if end >= 0:
                out.append(f"<{tag}>{inline_html(s[i + len(mark):end])}</{tag}>")
                i = end + len(mark) - 1
            else:
                out.append("*")
        elif c == "[":
            ident, end, ok = _footnote_ref(s, i)
            if ok:
                e = _html.escape(ident, quote=True)
                out.append(f'<sup class="fnref" id="fnref-{e}"><a href="#fn-{e}">{e}</a></sup>')
                i = end
            else:
                label, target, end, ok = _link(s, i)
                if ok:
                    out.append(f'<a href="{_html.escape(target, quote=True)}">{inline_html(label)}</a>')
                    i = end
                else:
                    out.append("[")
        else:
            out.append(_html.escape(c, quote=False))
        i += 1
    return "".join(out)


def inline_text(s: str) -> str:
    """Strip inline markup, for plain text and analysis."""
    out: list[str] = []
    i = 0
    while i < len(s):
        c = s[i]
        if c == "\\" and i + 1 < len(s):
            i += 1
            out.append(s[i])
        elif c == "`":
            end = s.find("`", i + 1)
            if end >= 0:
                out.append(s[i + 1:end])
                i = end
            else:
                out.append(c)
        elif c == "*":
            mark = "**" if s.startswith("**", i) else "*"
            end = _find_close(s, i + len(mark), mark)
            if end >= 0:
                out.append(inline_text(s[i + len(mark):end]))
                i = end + len(mark) - 1
            else:
                out.append(c)
        elif c == "[":
            ident, end, ok = _footnote_ref(s, i)
            if ok:
                out.append(f"[{ident}]")
                i = end
            else:
                label, _t, end, ok = _link(s, i)
                if ok:
                    out.append(inline_text(label))
                    i = end
                else:
                    out.append(c)
        else:
            out.append(c)
        i += 1
    return "".join(out)


# --------------------------------------------------------------------------
# Validation
# --------------------------------------------------------------------------

KNOWN: dict[str, bool] = {
    "xtxt": False, "image": False, "video": False, "audio": False,
    "attachment": False, "include": False, "embed": False, "hr": False,
    "code": True, "table": True, "math": True, "mermaid": True,
    "metadata": True, "comment": True, "raw": True, "chart": True,
    "footnote": True,
    "task": True, "decision": True, "knowledge": True,
    "ai": True, "prompt": True, "chat": True, "note": True,
}

_REQUIRED_SRC = {"image", "video", "audio", "attachment", "include"}


def validate(doc: Document) -> list[Issue]:
    """Semantic checks on top of the parser's syntactic ones."""
    issues: list[Issue] = []

    def warn(line: int, msg: str) -> None:
        issues.append(Issue("warning", line, msg))

    metadata_seen = False
    notes: dict[str, int] = {}

    for n in doc.nodes:
        if n.kind not in ("directive", "block"):
            continue
        if n.name not in KNOWN:
            warn(n.line, f"unknown directive @{n.name} (preserved, but this reader cannot render it)")
            continue
        fenced = KNOWN[n.name]
        if fenced and n.kind != "block":
            warn(n.line, f"@{n.name} is a block directive and should be closed with @end{n.name}")
        if not fenced and n.kind == "block":
            warn(n.line, f"@{n.name} is not a block directive but was closed with @end{n.name}")
        if n.name in _REQUIRED_SRC and not n.args.resolve("src"):
            warn(n.line, f"@{n.name} has no src")
        if n.name == "metadata":
            if metadata_seen:
                warn(n.line, "duplicate @metadata block")
            metadata_seen = True
        elif n.name == "table":
            t = parse_table(n)
            if not t.header:
                warn(n.line, "@table is empty")
            else:
                for i, row in enumerate(t.rows):
                    if len(row) != len(t.header):
                        warn(n.line + 1 + i,
                             f"table row has {len(row)} cells, header has {len(t.header)}")
        elif n.name == "code":
            if not n.args.resolve("language"):
                warn(n.line, "@code has no language; syntax highlighting will be skipped")
        elif n.name == "chart":
            if not _chart_rows(n):
                warn(n.line, "@chart has no readable data rows")
        elif n.name == "task":
            if not n.fields().get("title"):
                warn(n.line, "@task has no Title field")
        elif n.name == "footnote":
            ident = n.args.resolve("id")
            if not ident:
                warn(n.line, "@footnote has no id; references cannot point at it")
            notes[ident] = n.line

    issues.extend(_check_footnote_refs(doc, notes))
    return issues


def _check_footnote_refs(doc: Document, notes: dict[str, int]) -> list[Issue]:
    issues: list[Issue] = []
    cited: set[str] = set()

    def visit(text: str, line: int) -> None:
        i = 0
        while i < len(text):
            if text[i] == "\\":
                i += 2
                continue
            if text[i] == "[":
                ident, end, ok = _footnote_ref(text, i)
                if ok:
                    cited.add(ident)
                    if ident not in notes:
                        issues.append(Issue("warning", line,
                                            f'footnote reference [^{ident}] has no matching '
                                            f'@footnote(id="{ident}")'))
                    i = end
            i += 1

    for n in doc.nodes:
        if n.kind in ("heading", "paragraph", "quote"):
            visit(n.text, n.line)
        elif n.kind == "list":
            for it in n.items:
                visit(it.text, n.line)
    for ident, line in notes.items():
        if ident and ident not in cited:
            issues.append(Issue("warning", line, f'@footnote(id="{ident}") is never referenced'))
    return issues


def _chart_rows(n: Node) -> list[list[str]]:
    rows = []
    for line in n.text.split("\n"):
        if _is_blank(line):
            continue
        cells = _split_cells(line) if "|" in line else line.rsplit(None, 1)
        if len(cells) >= 2 and not _is_separator_row(cells):
            rows.append([c.strip() for c in cells])
    return rows


def sort_issues(issues: Iterable[Issue]) -> list[Issue]:
    return sorted(issues, key=lambda i: i.line)


# --------------------------------------------------------------------------
# Extraction — the machine-facing view
# --------------------------------------------------------------------------

_PRESENTATIONAL = {
    "code", "table", "math", "mermaid", "metadata", "comment", "raw",
    "image", "video", "audio", "attachment", "hr", "xtxt", "include",
    "embed", "footnote",
}


def _links_in(s: str, line: int) -> list[dict[str, Any]]:
    out = []
    i = 0
    while i < len(s):
        if s[i] == "\\":
            i += 2
            continue
        if s[i] == "[":
            label, target, end, ok = _link(s, i)
            if ok:
                out.append({"text": inline_text(label), "href": target, "line": line})
                i = end
        i += 1
    return out


def extract(doc: Document) -> dict[str, Any]:
    """Everything an agent needs without inferring structure from prose."""
    out: dict[str, Any] = {
        "version": doc.version, "metadata": doc.metadata(),
        "outline": [], "tasks": [], "blocks": [], "links": [],
        "media": [], "code": [], "text": "", "words": 0,
    }
    prose: list[str] = []

    for n in doc.nodes:
        if n.kind == "heading":
            text = inline_text(n.text)
            out["outline"].append({"level": n.level, "text": text, "line": n.line})
            prose.append(text)
            out["links"].extend(_links_in(n.text, n.line))
        elif n.kind in ("paragraph", "quote"):
            prose.append(inline_text(n.text))
            out["links"].extend(_links_in(n.text, n.line))
        elif n.kind == "list":
            for it in n.items:
                prose.append(inline_text(it.text))
                out["links"].extend(_links_in(it.text, n.line))
                if it.checked is not None:
                    out["tasks"].append({
                        "title": inline_text(it.text), "done": it.checked, "line": n.line,
                    })
        elif n.kind in ("directive", "block"):
            _absorb(out, n, prose)

    out["text"] = "\n\n".join(prose)
    out["words"] = len(out["text"].split())
    return out


def _absorb(out: dict[str, Any], n: Node, prose: list[str]) -> None:
    if n.name in ("comment", "metadata"):
        return
    if n.name in ("image", "video", "audio", "attachment"):
        out["media"].append({
            "kind": n.name, "src": n.args.resolve("src"),
            "caption": inline_text(n.args.get("caption")), "line": n.line,
        })
        if n.args.get("caption"):
            prose.append(inline_text(n.args.get("caption")))
        return
    if n.name == "code":
        out["code"].append({
            "language": n.args.resolve("language"),
            "lines": n.text.count("\n") + 1, "line": n.line, "source": n.text,
        })
        return
    if n.name == "table":
        t = parse_table(n)
        for row in [t.header] + t.rows:
            prose.append(" | ".join(row))
        return
    if n.name in _PRESENTATIONAL:
        if n.kind == "block" and n.text:
            prose.append(n.text)
        return

    f = n.fields()
    block: dict[str, Any] = {"type": n.name, "line": n.line, "text": n.text}
    if len(n.args):
        block["args"] = {(a.key or str(i)): a.value for i, a in enumerate(n.args)}
    if f:
        block["fields"] = f.map()
        block["order"] = [x.key for x in f]
    out["blocks"].append(block)

    if n.name == "task":
        m = f.map()
        status = m.get("status", "")
        out["tasks"].append({
            "title": m.get("title") or m.get("", ""),
            "status": status, "owner": m.get("owner", ""), "due": m.get("due", ""),
            "done": status.lower() in ("done", "complete"), "line": n.line,
        })
    if n.kind == "block" and n.text:
        prose.append(n.text)


# --------------------------------------------------------------------------
# Conformance
# --------------------------------------------------------------------------


def canonical(doc: Document) -> dict[str, Any]:
    """The normalised shape used by the conformance suite."""
    return {
        "version": doc.version,
        "nodes": [
            {
                "kind": n.kind,
                "name": n.name,
                "level": n.level,
                "text": n.text,
                "args": [{"key": a.key, "value": a.value} for a in n.args],
                "items": [
                    {"text": it.text, "ordered": it.ordered, "checked": it.checked}
                    for it in n.items
                ],
                "line": n.line,
            }
            for n in doc.nodes
        ],
    }


def canonical_issues(issues: Iterable[Issue]) -> list[dict[str, Any]]:
    return [{"severity": i.severity, "line": i.line} for i in sort_issues(issues)]


# --------------------------------------------------------------------------
# HTML rendering
# --------------------------------------------------------------------------


def render_html(doc: Document, full: bool = False, title: str = "") -> str:
    """Render to HTML. `full` wraps the result in a standalone document."""
    body: list[str] = []
    notes: list[Node] = []
    for n in doc.nodes:
        if n.kind == "heading":
            body.append(f"<h{n.level}>{inline_html(n.text)}</h{n.level}>")
        elif n.kind == "paragraph":
            body.append(f"<p>{inline_html(n.text)}</p>")
        elif n.kind == "quote":
            body.append(f"<blockquote><p>{inline_html(n.text)}</p></blockquote>")
        elif n.kind == "list":
            body.append(_list_html(n))
        elif n.kind in ("directive", "block"):
            if n.name == "footnote":
                notes.append(n)
            else:
                body.append(_directive_html(n))
    if notes:
        items = []
        for i, n in enumerate(notes):
            ident = _html.escape(n.args.resolve("id") or str(i + 1), quote=True)
            items.append(f'<li id="fn-{ident}">{inline_html(n.text)} '
                         f'<a class="fnback" href="#fnref-{ident}">&#8617;</a></li>')
        body.append('<section class="footnotes"><ol>' + "".join(items) + "</ol></section>")

    out = "\n".join(x for x in body if x)
    if not full:
        return out
    if not title:
        title = doc.metadata().get("title") or next(
            (inline_text(n.text) for n in doc.nodes if n.kind == "heading"), "Untitled")
    return (
        '<!doctype html>\n<html lang="en">\n<head>\n<meta charset="utf-8">\n'
        '<meta name="viewport" content="width=device-width, initial-scale=1">\n'
        f"<title>{_html.escape(title, quote=False)}</title>\n"
        "</head>\n<body>\n<main class=\"xtxt\">\n" + out + "\n</main>\n</body>\n</html>\n"
    )


def _list_html(n: Node) -> str:
    if not n.items:
        return ""
    tag = "ol" if n.items[0].ordered else "ul"
    cls = ' class="checklist"' if n.items[0].checked is not None else ""
    parts = [f"<{tag}{cls}>"]
    for it in n.items:
        box = ""
        if it.checked is not None:
            box = f'<input type="checkbox" disabled{" checked" if it.checked else ""}> '
        parts.append(f"<li>{box}{inline_html(it.text)}</li>")
    parts.append(f"</{tag}>")
    return "".join(parts)


def _directive_html(n: Node) -> str:
    esc = lambda s: _html.escape(s, quote=True)
    if n.name in ("comment", "metadata"):
        return ""
    if n.name == "hr":
        return "<hr>"
    if n.name == "image":
        alt = n.args.get("alt") or inline_text(n.args.get("caption"))
        attrs = "".join(f' {k}="{esc(n.args.get(k))}"' for k in ("width", "height") if n.args.get(k))
        cap = n.args.get("caption")
        figcap = f"<figcaption>{inline_html(cap)}</figcaption>" if cap else ""
        return f'<figure><img src="{esc(n.args.resolve("src"))}" alt="{esc(alt)}"{attrs}>{figcap}</figure>'
    if n.name in ("video", "audio"):
        extra = " controls playsinline" if n.name == "video" else " controls"
        cap = n.args.get("caption")
        figcap = f"<figcaption>{inline_html(cap)}</figcaption>" if cap else ""
        return f'<figure><{n.name} src="{esc(n.args.resolve("src"))}"{extra}></{n.name}>{figcap}</figure>'
    if n.name == "attachment":
        src = n.args.resolve("src")
        return f'<p class="attachment"><a href="{esc(src)}" download>{esc(n.args.get("name") or src)}</a></p>'
    if n.name == "code":
        lang = n.args.resolve("language")
        cls = f' class="language-{esc(lang)}"' if lang else ""
        return f"<pre><code{cls}>{_html.escape(n.text, quote=False)}</code></pre>"
    if n.name == "math":
        return f'<div class="math">{_html.escape(n.text, quote=False)}</div>'
    if n.name == "mermaid":
        return f'<pre class="mermaid">{_html.escape(n.text, quote=False)}</pre>'
    if n.name == "raw":
        return n.text if n.args.resolve("format") == "html" else f"<pre>{_html.escape(n.text, quote=False)}</pre>"
    if n.name == "table":
        return _table_html(n)
    if n.kind == "block":
        f = n.fields()
        if f:
            rows = "".join(f"<dt>{esc(x.key or '—')}</dt><dd>{inline_html(x.value)}</dd>" for x in f)
            return (f'<section class="record" data-type="{esc(n.name)}">'
                    f'<h4 class="record-type">{esc(n.name)}</h4><dl>{rows}</dl></section>')
    return f'<div class="unknown" data-directive="{esc(n.name)}">{esc(_source_of(n))}</div>'


def _table_html(n: Node) -> str:
    t = parse_table(n)
    if not t.header:
        return ""

    def style(i: int) -> str:
        a = t.align[i] if i < len(t.align) else "left"
        return f' style="text-align:{a}"' if a != "left" else ""

    head = "".join(f"<th{style(i)}>{inline_html(h)}</th>" for i, h in enumerate(t.header))
    rows = "".join(
        "<tr>" + "".join(f"<td{style(i)}>{inline_html(c)}</td>" for i, c in enumerate(r)) + "</tr>"
        for r in t.rows
    )
    return f"<table>\n<thead><tr>{head}</tr></thead>\n<tbody>{rows}</tbody>\n</table>"


def _source_of(n: Node) -> str:
    parts = []
    for a in n.args:
        v = a.value
        if not v or any(c in v for c in ' ,)"'):
            v = '"' + v.replace("\\", "\\\\").replace('"', '\\"') + '"'
        parts.append(f"{a.key}={v}" if a.key else v)
    out = "@" + n.name + (f"({', '.join(parts)})" if parts else "")
    if n.kind == "block":
        out += f"\n{n.text}\n@end{n.name}"
    return out


def _main(argv: list[str]) -> int:
    import argparse
    ap = argparse.ArgumentParser(prog="python -m xtxt", description="XTXT tools")
    ap.add_argument("command", choices=["ast", "extract", "html", "validate"])
    ap.add_argument("file")
    ns = ap.parse_args(argv)

    res = parse_file(ns.file)
    if ns.command == "validate":
        issues = sort_issues(list(res.issues) + validate(res.doc))
        for i in issues:
            print(f"{ns.file}:{i.line}: {i.severity}: {i.message}")
        if not issues:
            print(f"{ns.file}: ok ({len(res.doc.nodes)} blocks)")
        return 1 if any(i.severity == "error" for i in issues) else 0
    if ns.command == "ast":
        print(json.dumps(canonical(res.doc), indent=2))
    elif ns.command == "extract":
        print(json.dumps(extract(res.doc), indent=2))
    else:
        print(render_html(res.doc, full=True))
    return 0
