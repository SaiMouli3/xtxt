/*
 * xtxt.c — implementation. See xtxt.h for the interface and SPEC.md for the
 * format. This mirrors the Go reference implementation structure for structure,
 * so the two can be diffed by eye when the spec changes.
 */

#include "xtxt.h"

#include <ctype.h>
#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

/* -------------------------------------------------------------------------- */
/* Small allocation helpers                                                    */
/*                                                                             */
/* Allocation failure is fatal rather than propagated. A document parser that   */
/* cannot allocate has nothing useful to say, and threading an error path       */
/* through every helper would triple the size of this file for a case that      */
/* only happens when the process is already doomed.                            */
/* -------------------------------------------------------------------------- */

static void *xmalloc(size_t n) {
  void *p = malloc(n ? n : 1);
  if (!p) {
    fputs("xtxt: out of memory\n", stderr);
    abort();
  }
  return p;
}

static void *xrealloc(void *p, size_t n) {
  void *q = realloc(p, n ? n : 1);
  if (!q) {
    fputs("xtxt: out of memory\n", stderr);
    abort();
  }
  return q;
}

static char *xstrndup(const char *s, size_t n) {
  char *out = xmalloc(n + 1);
  memcpy(out, s, n);
  out[n] = '\0';
  return out;
}

static char *xstrdup(const char *s) { return xstrndup(s, strlen(s)); }

/* A growable byte buffer used to build strings. */
typedef struct {
  char *data;
  size_t len, cap;
} buf;

static void buf_reserve(buf *b, size_t extra) {
  if (b->len + extra + 1 <= b->cap) return;
  size_t want = b->cap ? b->cap : 32;
  while (want < b->len + extra + 1) want *= 2;
  b->data = xrealloc(b->data, want);
  b->cap = want;
}

static void buf_add(buf *b, const char *s, size_t n) {
  if (n == 0) return;
  buf_reserve(b, n);
  memcpy(b->data + b->len, s, n);
  b->len += n;
  b->data[b->len] = '\0';
}

static void buf_puts(buf *b, const char *s) { buf_add(b, s, strlen(s)); }
static void buf_putc(buf *b, char c) { buf_add(b, &c, 1); }

/* Takes ownership of the buffer's storage. */
static char *buf_take(buf *b) {
  if (!b->data) return xstrdup("");
  char *out = b->data;
  b->data = NULL;
  b->len = b->cap = 0;
  return out;
}

/*
 * Formats into a fresh allocation. It measures into a stack buffer rather than
 * using the vsnprintf(NULL, 0, ...) idiom: that is valid C99, but glibc's
 * fortified vsnprintf makes GCC reject it under -Werror, and every diagnostic
 * this builds fits in the buffer anyway.
 */
static char *xasprintf(const char *fmt, ...) {
  char stack[256];
  va_list ap, ap2;
  va_start(ap, fmt);
  va_copy(ap2, ap);
  int n = vsnprintf(stack, sizeof stack, fmt, ap);
  va_end(ap);
  if (n < 0) {
    va_end(ap2);
    return xstrdup("");
  }
  if ((size_t)n < sizeof stack) {
    va_end(ap2);
    return xstrndup(stack, (size_t)n);
  }
  char *out = xmalloc((size_t)n + 1);
  vsnprintf(out, (size_t)n + 1, fmt, ap2);
  va_end(ap2);
  return out;
}

/* -------------------------------------------------------------------------- */
/* Lines and character classes                                                 */
/* -------------------------------------------------------------------------- */

typedef struct {
  char **items;
  size_t count, cap;
} strlist;

static void strlist_push(strlist *l, char *s) {
  if (l->count == l->cap) {
    l->cap = l->cap ? l->cap * 2 : 16;
    l->items = xrealloc(l->items, l->cap * sizeof *l->items);
  }
  l->items[l->count++] = s;
}

static void strlist_free(strlist *l) {
  for (size_t i = 0; i < l->count; i++) free(l->items[i]);
  free(l->items);
  l->items = NULL;
  l->count = l->cap = 0;
}

static int is_name_start(unsigned char c) {
  return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z');
}

static int is_name_char(unsigned char c) {
  return is_name_start(c) || c == '-' || (c >= '0' && c <= '9');
}

/* Space as the format defines it: never locale-dependent. */
static int is_space(unsigned char c) {
  return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\v' || c == '\f';
}

static int is_blank(const char *s) {
  for (; *s; s++) {
    if (!is_space((unsigned char)*s)) return 0;
  }
  return 1;
}

/* Offset of the first character that is not a space or tab. */
static size_t indent_of(const char *s) {
  size_t i = 0;
  while (s[i] == ' ' || s[i] == '\t') i++;
  return i;
}

/* A newly allocated copy of s with leading and trailing space removed. */
static char *trim_dup(const char *s, size_t n) {
  size_t start = 0;
  while (start < n && is_space((unsigned char)s[start])) start++;
  size_t end = n;
  while (end > start && is_space((unsigned char)s[end - 1])) end--;
  return xstrndup(s + start, end - start);
}

static char *trim_str(const char *s) { return trim_dup(s, strlen(s)); }

/* Splits source into lines, normalising CRLF and dropping a trailing blank. */
static strlist split_lines(const char *src, size_t len) {
  strlist out = {0};
  size_t start = 0;
  for (size_t i = 0; i <= len; i++) {
    if (i == len || src[i] == '\n') {
      size_t end = i;
      if (end > start && src[end - 1] == '\r') end--;
      if (i == len && end == start) break; /* no trailing empty line */
      strlist_push(&out, xstrndup(src + start, end - start));
      start = i + 1;
    }
  }
  /* Strip a UTF-8 BOM from the first line. */
  if (out.count > 0 && strncmp(out.items[0], "\xEF\xBB\xBF", 3) == 0) {
    char *fixed = xstrdup(out.items[0] + 3);
    free(out.items[0]);
    out.items[0] = fixed;
  }
  return out;
}

/* 1-6 for a heading line, else 0. */
static int heading_level(const char *s) {
  const char *t = s + indent_of(s);
  int n = 0;
  while (t[n] == '#') n++;
  if (n == 0 || n > 6 || t[n] != ' ') return 0;
  return n;
}

static int is_directive_line(const char *s) {
  const char *t = s + indent_of(s);
  return t[0] == '@' && is_name_start((unsigned char)t[1]);
}

/* The length of the list-item prefix at the start of s, or 0. */
static size_t item_prefix_len(const char *s) {
  const char *t = s + indent_of(s);
  if ((t[0] == '-' || t[0] == '*') && t[1] == ' ') return 2;
  size_t i = 0;
  while (t[i] >= '0' && t[i] <= '9') i++;
  if (i > 0 && t[i] == '.' && t[i + 1] == ' ') return i + 2;
  return 0;
}

/* -------------------------------------------------------------------------- */
/* Argument parsing                                                            */
/* -------------------------------------------------------------------------- */

/*
 * The offset of the closing paren of the outermost group, or (size_t)-1.
 * Quotes and backslash escapes are respected, so a ')' inside a string does
 * not end the list.
 */
static size_t balanced_end(const char *s) {
  int depth = 0, in_quote = 0, esc = 0;
  for (size_t i = 0; s[i]; i++) {
    char c = s[i];
    if (esc) {
      esc = 0;
    } else if (c == '\\') {
      esc = 1;
    } else if (in_quote) {
      if (c == '"') in_quote = 0;
    } else if (c == '"') {
      in_quote = 1;
    } else if (c == '(') {
      depth++;
    } else if (c == ')') {
      if (--depth == 0) return i;
    }
  }
  return (size_t)-1;
}

/* Offsets of top-level separators, ignoring quotes and nested parens. */
typedef struct {
  size_t *at;
  size_t count, cap;
} offsets;

static void offsets_push(offsets *o, size_t v) {
  if (o->count == o->cap) {
    o->cap = o->cap ? o->cap * 2 : 8;
    o->at = xrealloc(o->at, o->cap * sizeof *o->at);
  }
  o->at[o->count++] = v;
}

static offsets split_top(const char *s, size_t len, char sep) {
  offsets out = {0};
  int depth = 0, in_quote = 0, esc = 0;
  for (size_t i = 0; i < len; i++) {
    char c = s[i];
    if (esc) {
      esc = 0;
    } else if (c == '\\') {
      esc = 1;
    } else if (in_quote) {
      if (c == '"') in_quote = 0;
    } else if (c == '"') {
      in_quote = 1;
    } else if (c == '(') {
      depth++;
    } else if (c == ')') {
      depth--;
    } else if (c == sep && depth == 0) {
      offsets_push(&out, i);
    }
  }
  return out;
}

static int is_name(const char *s, size_t len) {
  size_t start = 0, end = len;
  while (start < end && is_space((unsigned char)s[start])) start++;
  while (end > start && is_space((unsigned char)s[end - 1])) end--;
  if (start == end || !is_name_start((unsigned char)s[start])) return 0;
  for (size_t i = start + 1; i < end; i++) {
    if (!is_name_char((unsigned char)s[i])) return 0;
  }
  return 1;
}

static char *unescape_dup(const char *s, size_t n) {
  buf b = {0};
  for (size_t i = 0; i < n; i++) {
    if (s[i] == '\\' && i + 1 < n) {
      i++;
      if (s[i] == 'n') {
        buf_putc(&b, '\n');
      } else if (s[i] == 't') {
        buf_putc(&b, '\t');
      } else {
        buf_putc(&b, s[i]);
      }
    } else {
      buf_putc(&b, s[i]);
    }
  }
  return buf_take(&b);
}

/* Strips surrounding quotes and unescapes, or trims. */
static char *unquote_dup(const char *s, size_t n) {
  size_t start = 0, end = n;
  while (start < end && is_space((unsigned char)s[start])) start++;
  while (end > start && is_space((unsigned char)s[end - 1])) end--;
  size_t len = end - start;
  if (len >= 2 && s[start] == '"' && s[end - 1] == '"') {
    return unescape_dup(s + start + 1, len - 2);
  }
  return xstrndup(s + start, len);
}

typedef struct {
  xtxt_arg *items;
  size_t count, cap;
} arglist;

static void arglist_push(arglist *l, char *key, char *value) {
  if (l->count == l->cap) {
    l->cap = l->cap ? l->cap * 2 : 4;
    l->items = xrealloc(l->items, l->cap * sizeof *l->items);
  }
  l->items[l->count].key = key;
  l->items[l->count].value = value;
  l->count++;
}

/* Parses the inside of an argument list. */
static arglist split_args(const char *s, size_t len) {
  arglist out = {0};
  offsets commas = split_top(s, len, ',');
  size_t start = 0;
  for (size_t k = 0; k <= commas.count; k++) {
    size_t end = (k < commas.count) ? commas.at[k] : len;
    const char *field = s + start;
    size_t flen = end - start;
    start = end + 1;

    size_t fs = 0, fe = flen;
    while (fs < fe && is_space((unsigned char)field[fs])) fs++;
    while (fe > fs && is_space((unsigned char)field[fe - 1])) fe--;
    if (fs == fe) continue;

    const char *f = field + fs;
    size_t n = fe - fs;

    offsets eqs = split_top(f, n, '=');
    char *key = NULL, *value = NULL;
    if (eqs.count > 0 && is_name(f, eqs.at[0])) {
      key = trim_dup(f, eqs.at[0]);
      value = unquote_dup(f + eqs.at[0] + 1, n - eqs.at[0] - 1);
    } else {
      key = xstrdup("");
      value = unquote_dup(f, n);
    }
    free(eqs.at);
    arglist_push(&out, key, value);
  }
  free(commas.at);
  return out;
}

/* -------------------------------------------------------------------------- */
/* Parser                                                                      */
/* -------------------------------------------------------------------------- */

typedef struct {
  xtxt_node *items;
  size_t count, cap;
} nodelist;

static xtxt_node *nodelist_add(nodelist *l, xtxt_kind kind, size_t line) {
  if (l->count == l->cap) {
    l->cap = l->cap ? l->cap * 2 : 16;
    l->items = xrealloc(l->items, l->cap * sizeof *l->items);
  }
  xtxt_node *n = &l->items[l->count++];
  memset(n, 0, sizeof *n);
  n->kind = kind;
  n->name = xstrdup("");
  n->text = xstrdup("");
  n->line = line;
  return n;
}

typedef struct {
  xtxt_issue *items;
  size_t count, cap;
} issuelist;

static void issuelist_push(issuelist *l, xtxt_severity sev, size_t line, char *msg) {
  if (l->count == l->cap) {
    l->cap = l->cap ? l->cap * 2 : 8;
    l->items = xrealloc(l->items, l->cap * sizeof *l->items);
  }
  l->items[l->count].severity = sev;
  l->items[l->count].line = line;
  l->items[l->count].message = msg;
  l->count++;
}

typedef struct {
  strlist lines;
  size_t i;
  nodelist nodes;
  issuelist issues;
  char *version;
} parser;

/*
 * Directives that are blocks even when the closing fence is missing; used only
 * to report a helpful error.
 */
static int fenced_by_default(const char *name) {
  static const char *const names[] = {"code",     "table",   "math", "mermaid",
                                      "metadata", "comment", "raw"};
  for (size_t i = 0; i < sizeof names / sizeof *names; i++) {
    if (strcmp(name, names[i]) == 0) return 1;
  }
  return 0;
}

/* The index of the @end<name> line, or (size_t)-1. */
static size_t find_fence(parser *p, const char *name, size_t from) {
  char *closer = xasprintf("@end%s", name);
  size_t found = (size_t)-1;
  for (size_t j = from; j < p->lines.count; j++) {
    const char *l = p->lines.items[j];
    size_t end = strlen(l);
    while (end > 0 && (l[end - 1] == ' ' || l[end - 1] == '\t')) end--;
    if (strlen(closer) == end && strncmp(l, closer, end) == 0) {
      found = j;
      break;
    }
  }
  free(closer);
  return found;
}

/*
 * Joins a fence body: drops one blank line at each end and unescapes any
 * \@end… line, so a payload can contain what would otherwise close it.
 */
static char *join_fence_body(strlist *lines, size_t from, size_t to) {
  while (from < to && is_blank(lines->items[from])) {
    from++;
    break;
  }
  while (to > from && is_blank(lines->items[to - 1])) {
    to--;
    break;
  }
  buf b = {0};
  for (size_t i = from; i < to; i++) {
    if (i > from) buf_putc(&b, '\n');
    const char *l = lines->items[i];
    size_t ind = indent_of(l);
    if (strncmp(l + ind, "\\@end", 5) == 0) {
      buf_add(&b, l, ind);
      buf_puts(&b, l + ind + 1);
    } else {
      buf_puts(&b, l);
    }
  }
  return buf_take(&b);
}

/*
 * Reads an argument list that may span lines. On success returns 1, fills
 * `out`, and leaves p->i on the last consumed line. On an unclosed list
 * returns 0 and leaves p->i untouched.
 */
static int parse_args(parser *p, const char *rest, arglist *out) {
  size_t off = 0;
  while (is_space((unsigned char)rest[off])) off++;
  if (rest[off] != '(') {
    memset(out, 0, sizeof *out);
    return 1;
  }

  buf b = {0};
  buf_puts(&b, rest + off);
  size_t line = p->i;
  for (;;) {
    size_t end = balanced_end(b.data);
    if (end != (size_t)-1) {
      *out = split_args(b.data + 1, end - 1);
      free(b.data);
      p->i = line;
      return 1;
    }
    line++;
    if (line >= p->lines.count) {
      free(b.data);
      return 0;
    }
    buf_putc(&b, '\n');
    buf_puts(&b, p->lines.items[line]);
  }
}

static void parse_directive(parser *p) {
  size_t start = p->i;
  const char *line = p->lines.items[p->i];
  const char *t = line + indent_of(line);
  size_t n = 1;
  while (is_name_char((unsigned char)t[n])) n++;
  char *name = xstrndup(t + 1, n - 1);
  const char *rest = t + n;

  arglist args = {0};
  if (!parse_args(p, rest, &args)) {
    issuelist_push(&p->issues, XTXT_ERROR, start + 1,
                   xasprintf("unclosed argument list for @%s", name));
    p->i = start + 1;
    free(name);
    return;
  }

  if (strncmp(name, "end", 3) == 0) {
    issuelist_push(&p->issues, XTXT_ERROR, start + 1,
                   xasprintf("@%s has no matching opening fence", name));
    p->i++;
    for (size_t k = 0; k < args.count; k++) {
      free(args.items[k].key);
      free(args.items[k].value);
    }
    free(args.items);
    free(name);
    return;
  }

  if (strcmp(name, "xtxt") == 0 && p->nodes.count == 0) {
    const char *v = "";
    for (size_t k = 0; k < args.count; k++) {
      if (strcmp(args.items[k].key, "version") == 0 && args.items[k].value[0]) {
        v = args.items[k].value;
        break;
      }
    }
    if (!v[0]) {
      for (size_t k = 0; k < args.count; k++) {
        if (!args.items[k].key[0]) {
          v = args.items[k].value;
          break;
        }
      }
    }
    free(p->version);
    p->version = xstrdup(v);
    p->i++;
    for (size_t k = 0; k < args.count; k++) {
      free(args.items[k].key);
      free(args.items[k].value);
    }
    free(args.items);
    free(name);
    return;
  }

  /*
   * A directive is fenced if a matching @end<name> line follows. Keeping the
   * rule local to the document means no registry of block names has to be kept
   * in sync across implementations.
   */
  size_t end = find_fence(p, name, p->i + 1);
  if (end != (size_t)-1) {
    char *body = join_fence_body(&p->lines, p->i + 1, end);
    p->i = end + 1;
    xtxt_node *node = nodelist_add(&p->nodes, XTXT_BLOCK, start + 1);
    free(node->name);
    node->name = name;
    free(node->text);
    node->text = body;
    node->args = args.items;
    node->arg_count = args.count;
    return;
  }

  if (fenced_by_default(name)) {
    issuelist_push(&p->issues, XTXT_ERROR, start + 1,
                   xasprintf("unclosed @%s block: no matching @end%s", name, name));
  }
  p->i++;
  xtxt_node *node = nodelist_add(&p->nodes, XTXT_DIRECTIVE, start + 1);
  free(node->name);
  node->name = name;
  node->args = args.items;
  node->arg_count = args.count;
}

static void parse_quote(parser *p) {
  size_t start = p->i;
  buf b = {0};
  int first = 1;
  while (p->i < p->lines.count) {
    char *t = trim_str(p->lines.items[p->i]);
    if (t[0] != '>') {
      free(t);
      break;
    }
    char *inner = trim_str(t + 1);
    if (!first) buf_putc(&b, ' ');
    buf_puts(&b, inner);
    free(inner);
    free(t);
    first = 0;
    p->i++;
  }
  xtxt_node *n = nodelist_add(&p->nodes, XTXT_QUOTE, start + 1);
  free(n->text);
  n->text = buf_take(&b);
}

static void parse_list(parser *p) {
  size_t start = p->i;
  xtxt_item *items = NULL;
  size_t count = 0, cap = 0;

  size_t base_indent = 0;
  int have_base = 0, flagged = 0;

  while (p->i < p->lines.count) {
    const char *line = p->lines.items[p->i];
    size_t pre = item_prefix_len(line);
    if (pre == 0) break;
    /* Lists do not nest (SPEC 3.4), so an item indented deeper than the first
       is structure about to be lost. Flattening it silently turns a formatting
       mistake into data loss; uniform indentation is style, not intent. */
    {
      size_t ind = indent_of(line);
      if (!have_base) { base_indent = ind; have_base = 1; }
      else if (ind > base_indent && !flagged) {
        issuelist_push(&p->issues, XTXT_WARNING, p->i + 1,
                       xstrdup("list item is indented deeper than the first: "
                               "XTXT lists do not nest, so it is flattened"));
        flagged = 1;
      }
    }
    const char *t = line + indent_of(line);
    char *body = trim_str(t + pre);

    if (count == cap) {
      cap = cap ? cap * 2 : 8;
      items = xrealloc(items, cap * sizeof *items);
    }
    xtxt_item *it = &items[count++];
    it->ordered = (t[0] >= '0' && t[0] <= '9');
    it->checked = XTXT_UNCHECKED_NONE;

    if (strlen(body) >= 3 && body[0] == '[' && body[2] == ']' &&
        (body[1] == ' ' || body[1] == 'x' || body[1] == 'X')) {
      it->checked = (body[1] == ' ') ? XTXT_UNCHECKED : XTXT_CHECKED;
      char *rest = trim_str(body + 3);
      free(body);
      body = rest;
    }
    it->text = body;
    p->i++;
  }

  xtxt_node *n = nodelist_add(&p->nodes, XTXT_LIST, start + 1);
  n->items = items;
  n->item_count = count;
}

static void parse_paragraph(parser *p) {
  size_t start = p->i;
  buf b = {0};
  int first = 1;
  while (p->i < p->lines.count) {
    const char *line = p->lines.items[p->i];
    if (is_blank(line) || heading_level(line) > 0 || is_directive_line(line) ||
        item_prefix_len(line) > 0) {
      break;
    }
    char *t = trim_str(line);
    if (t[0] == '>') {
      free(t);
      break;
    }
    if (!first) buf_putc(&b, ' ');
    /* `\@` at the start of a line is a literal `@`. */
    buf_puts(&b, (t[0] == '\\' && t[1] == '@') ? t + 1 : t);
    free(t);
    first = 0;
    p->i++;
  }
  xtxt_node *n = nodelist_add(&p->nodes, XTXT_PARAGRAPH, start + 1);
  free(n->text);
  n->text = buf_take(&b);
}

xtxt_result *xtxt_parse(const char *src, size_t len) {
  if (!src) return NULL;
  if (len == 0) len = strlen(src);

  parser p = {0};
  p.lines = split_lines(src, len);
  p.version = xstrdup("");

  while (p.i < p.lines.count) {
    const char *line = p.lines.items[p.i];
    if (is_blank(line)) {
      p.i++;
    } else if (is_directive_line(line)) {
      parse_directive(&p);
    } else if (heading_level(line) > 0) {
      int lvl = heading_level(line);
      const char *t = line + indent_of(line);
      xtxt_node *n = nodelist_add(&p.nodes, XTXT_HEADING, p.i + 1);
      n->level = lvl;
      free(n->text);
      n->text = trim_str(t + lvl);
      p.i++;
    } else {
      char *t = trim_str(line);
      int quote = t[0] == '>';
      free(t);
      if (quote) {
        parse_quote(&p);
      } else if (item_prefix_len(line) > 0) {
        parse_list(&p);
      } else {
        parse_paragraph(&p);
      }
    }
  }

  strlist_free(&p.lines);

  xtxt_result *r = xmalloc(sizeof *r);
  r->doc.version = p.version;
  r->doc.nodes = p.nodes.items;
  r->doc.node_count = p.nodes.count;
  r->issues = p.issues.items;
  r->issue_count = p.issues.count;
  return r;
}

void xtxt_result_free(xtxt_result *r) {
  if (!r) return;
  for (size_t i = 0; i < r->doc.node_count; i++) {
    xtxt_node *n = &r->doc.nodes[i];
    free(n->name);
    free(n->text);
    for (size_t k = 0; k < n->arg_count; k++) {
      free(n->args[k].key);
      free(n->args[k].value);
    }
    free(n->args);
    for (size_t k = 0; k < n->item_count; k++) free(n->items[k].text);
    free(n->items);
  }
  free(r->doc.nodes);
  free(r->doc.version);
  for (size_t i = 0; i < r->issue_count; i++) free(r->issues[i].message);
  free(r->issues);
  free(r);
}

int xtxt_has_errors(const xtxt_result *r) {
  if (!r) return 0;
  for (size_t i = 0; i < r->issue_count; i++) {
    if (r->issues[i].severity == XTXT_ERROR) return 1;
  }
  return 0;
}

const char *xtxt_kind_name(xtxt_kind kind) {
  switch (kind) {
    case XTXT_HEADING: return "heading";
    case XTXT_PARAGRAPH: return "paragraph";
    case XTXT_QUOTE: return "quote";
    case XTXT_LIST: return "list";
    case XTXT_DIRECTIVE: return "directive";
    case XTXT_BLOCK: return "block";
  }
  return "unknown";
}

const char *xtxt_severity_name(xtxt_severity severity) {
  return severity == XTXT_ERROR ? "error" : "warning";
}

/* -------------------------------------------------------------------------- */
/* Arguments                                                                   */
/* -------------------------------------------------------------------------- */

const char *xtxt_arg_get(const xtxt_node *n, const char *key) {
  if (!n || !key) return "";
  for (size_t i = 0; i < n->arg_count; i++) {
    if (strcmp(n->args[i].key, key) == 0) return n->args[i].value;
  }
  return "";
}

const char *xtxt_arg_positional(const xtxt_node *n, size_t want) {
  if (!n) return "";
  size_t seen = 0;
  for (size_t i = 0; i < n->arg_count; i++) {
    if (!n->args[i].key[0]) {
      if (seen == want) return n->args[i].value;
      seen++;
    }
  }
  return "";
}

const char *xtxt_arg_resolve(const xtxt_node *n, const char *key) {
  const char *named = xtxt_arg_get(n, key);
  return named[0] ? named : xtxt_arg_positional(n, 0);
}

/* -------------------------------------------------------------------------- */
/* Records                                                                     */
/* -------------------------------------------------------------------------- */

/*
 * A field key is a label, not a sentence: these caps are what keep ordinary
 * prose containing a colon from being read as a record field.
 */
#define XTXT_MAX_FIELD_KEY_LEN 32
#define XTXT_MAX_FIELD_KEY_WORDS 3

static size_t word_count(const char *s, size_t n) {
  size_t words = 0;
  int in_word = 0;
  for (size_t i = 0; i < n; i++) {
    if (is_space((unsigned char)s[i])) {
      in_word = 0;
    } else if (!in_word) {
      in_word = 1;
      words++;
    }
  }
  return words;
}

/* If the line opens a field, returns 1 and sets key/value offsets. */
static int is_field_line(const char *line, size_t len, size_t *key_end, size_t *value_start) {
  for (size_t i = 0; i < len && i <= XTXT_MAX_FIELD_KEY_LEN; i++) {
    char c = line[i];
    if (c == ':' || c == '=') {
      size_t ks = 0, ke = i;
      while (ks < ke && is_space((unsigned char)line[ks])) ks++;
      while (ke > ks && is_space((unsigned char)line[ke - 1])) ke--;
      if (ks == ke || !is_name_start((unsigned char)line[ks])) return 0;
      if (word_count(line + ks, ke - ks) > XTXT_MAX_FIELD_KEY_WORDS) return 0;
      *key_end = i;
      *value_start = i + 1;
      return 1;
    }
    if (!(c == ' ' || c == '_' || c == '-' || c == '.' || is_name_char((unsigned char)c))) {
      return 0;
    }
  }
  return 0;
}

xtxt_fields xtxt_parse_fields(const char *payload) {
  xtxt_fields out = {0};
  if (!payload) return out;

  size_t cap = 0;
  buf preamble = {0};
  int have_preamble = 0;
  buf current = {0};
  int have_current = 0;

  const char *p = payload;
  for (;;) {
    const char *nl = strchr(p, '\n');
    size_t len = nl ? (size_t)(nl - p) : strlen(p);

    size_t key_end, value_start;
    if (is_field_line(p, len, &key_end, &value_start)) {
      if (have_current) {
        out.fields[out.count - 1].value = buf_take(&current);
        memset(&current, 0, sizeof current);
      }
      if (out.count == cap) {
        cap = cap ? cap * 2 : 8;
        out.fields = xrealloc(out.fields, cap * sizeof *out.fields);
      }
      out.fields[out.count].key = trim_dup(p, key_end);
      out.fields[out.count].value = NULL;
      out.count++;
      buf_add(&current, p + value_start, len - value_start);
      have_current = 1;
    } else if (!have_current) {
      if (have_preamble) buf_putc(&preamble, '\n');
      buf_add(&preamble, p, len);
      have_preamble = 1;
    } else {
      /*
       * A continuation line. An empty accumulated value takes the trimmed
       * line, so `Summary:` followed by text reads naturally; otherwise the
       * line is appended verbatim and the whole value is trimmed at the end.
       */
      if (current.len == 0) {
        char *t = trim_dup(p, len);
        buf_puts(&current, t);
        free(t);
      } else {
        buf_putc(&current, '\n');
        buf_add(&current, p, len);
      }
    }

    if (!nl) break;
    p = nl + 1;
  }

  if (have_current) {
    out.fields[out.count - 1].value = buf_take(&current);
  }
  free(current.data);

  /* Trim every value, matching the reference implementation. */
  for (size_t i = 0; i < out.count; i++) {
    char *raw = out.fields[i].value ? out.fields[i].value : xstrdup("");
    out.fields[i].value = trim_str(raw);
    free(raw);
  }

  /* Prose before the first field is kept under the empty key. */
  if (have_preamble) {
    char *text = trim_str(preamble.data ? preamble.data : "");
    if (text[0]) {
      if (out.count == cap) {
        cap = cap ? cap * 2 : 8;
        out.fields = xrealloc(out.fields, cap * sizeof *out.fields);
      }
      memmove(out.fields + 1, out.fields, out.count * sizeof *out.fields);
      out.fields[0].key = xstrdup("");
      out.fields[0].value = text;
      out.count++;
    } else {
      free(text);
    }
  }
  free(preamble.data);
  return out;
}

xtxt_fields xtxt_node_fields(const xtxt_node *n) {
  xtxt_fields empty = {0};
  if (!n || n->kind != XTXT_BLOCK) return empty;
  return xtxt_parse_fields(n->text);
}

static int ascii_ieq(const char *a, const char *b) {
  for (; *a && *b; a++, b++) {
    if (tolower((unsigned char)*a) != tolower((unsigned char)*b)) return 0;
  }
  return *a == *b;
}

const char *xtxt_field_get(const xtxt_fields *f, const char *key) {
  if (!f || !key) return "";
  for (size_t i = 0; i < f->count; i++) {
    if (ascii_ieq(f->fields[i].key, key)) return f->fields[i].value;
  }
  return "";
}

void xtxt_fields_free(xtxt_fields f) {
  for (size_t i = 0; i < f.count; i++) {
    free(f.fields[i].key);
    free(f.fields[i].value);
  }
  free(f.fields);
}

/* -------------------------------------------------------------------------- */
/* Tables                                                                      */
/* -------------------------------------------------------------------------- */

static char **split_cells(const char *line, size_t len, size_t *out_count) {
  size_t start = 0, end = len;
  while (start < end && is_space((unsigned char)line[start])) start++;
  while (end > start && is_space((unsigned char)line[end - 1])) end--;
  if (start < end && line[start] == '|') start++;
  if (end > start && line[end - 1] == '|') end--;

  char **cells = NULL;
  size_t count = 0, cap = 0;
  size_t cell_start = start;
  for (size_t i = start; i <= end; i++) {
    if (i == end || line[i] == '|') {
      if (count == cap) {
        cap = cap ? cap * 2 : 4;
        cells = xrealloc(cells, cap * sizeof *cells);
      }
      cells[count++] = trim_dup(line + cell_start, i - cell_start);
      cell_start = i + 1;
    }
  }
  *out_count = count;
  return cells;
}

static int is_separator_cells(char **cells, size_t count) {
  if (count == 0) return 0;
  for (size_t i = 0; i < count; i++) {
    const char *c = cells[i];
    if (!c[0]) return 0;
    for (; *c; c++) {
      if (*c != '-' && *c != ':') return 0;
    }
  }
  return 1;
}

static void free_cells(char **cells, size_t count) {
  for (size_t i = 0; i < count; i++) free(cells[i]);
  free(cells);
}

xtxt_table xtxt_parse_table(const xtxt_node *n) {
  xtxt_table t = {0};
  if (!n) return t;

  char ***rows = NULL;
  size_t *widths = NULL;
  size_t count = 0, cap = 0;
  size_t sep_at = (size_t)-1;

  const char *p = n->text;
  for (;;) {
    const char *nl = strchr(p, '\n');
    size_t len = nl ? (size_t)(nl - p) : strlen(p);

    char *line = xstrndup(p, len);
    if (!is_blank(line)) {
      size_t cells_n = 0;
      char **cells = split_cells(p, len, &cells_n);
      if (sep_at == (size_t)-1 && is_separator_cells(cells, cells_n)) {
        sep_at = count;
        t.align = xmalloc(cells_n * sizeof *t.align);
        for (size_t i = 0; i < cells_n; i++) {
          const char *c = cells[i];
          size_t clen = strlen(c);
          int left = c[0] == ':';
          int right = clen > 0 && c[clen - 1] == ':';
          t.align[i] = xstrdup((left && right) ? "center" : right ? "right" : "left");
        }
        free_cells(cells, cells_n);
      } else {
        if (count == cap) {
          cap = cap ? cap * 2 : 8;
          rows = xrealloc(rows, cap * sizeof *rows);
          widths = xrealloc(widths, cap * sizeof *widths);
        }
        rows[count] = cells;
        widths[count] = cells_n;
        count++;
      }
    }
    free(line);

    if (!nl) break;
    p = nl + 1;
  }

  if (count == 0) {
    free(rows);
    free(widths);
    return t;
  }

  size_t sep = (sep_at != (size_t)-1 && sep_at > 0) ? sep_at : 1;
  size_t header_index = sep > 1 ? sep - 1 : 0;
  t.header = rows[header_index];
  t.column_count = widths[header_index];

  /* Rows before the header (there are none in valid input) are discarded. */
  for (size_t i = 0; i < sep && i < count; i++) {
    if (i != header_index) free_cells(rows[i], widths[i]);
  }

  size_t first = sep < count ? sep : count;
  t.row_count = count - first;
  if (t.row_count > 0) {
    t.rows = xmalloc(t.row_count * sizeof *t.rows);
    t.row_widths = xmalloc(t.row_count * sizeof *t.row_widths);
    for (size_t i = 0; i < t.row_count; i++) {
      t.rows[i] = rows[first + i];
      t.row_widths[i] = widths[first + i];
    }
  }
  free(rows);
  free(widths);
  return t;
}

void xtxt_table_free(xtxt_table t) {
  free_cells(t.header, t.column_count);
  for (size_t i = 0; i < t.row_count; i++) free_cells(t.rows[i], t.row_widths[i]);
  free(t.rows);
  free(t.row_widths);
  if (t.align) {
    for (size_t i = 0; i < t.column_count; i++) free(t.align[i]);
    free(t.align);
  }
}

/* -------------------------------------------------------------------------- */
/* Inline formatting                                                           */
/* -------------------------------------------------------------------------- */

/*
 * Escapes one byte. It works a byte at a time rather than decoding a code
 * point, because the scan is byte-oriented and every byte above ASCII passes
 * through untouched — so valid UTF-8 in gives valid UTF-8 out.
 */
static void escape_byte(buf *b, unsigned char c) {
  switch (c) {
    case '&': buf_puts(b, "&amp;"); break;
    case '<': buf_puts(b, "&lt;"); break;
    case '>': buf_puts(b, "&gt;"); break;
    case '"': buf_puts(b, "&#34;"); break;
    case '\'': buf_puts(b, "&#39;"); break;
    default: buf_putc(b, (char)c);
  }
}

static void escape_into(buf *b, const char *s, size_t n) {
  for (size_t i = 0; i < n; i++) escape_byte(b, (unsigned char)s[i]);
}

/* The offset of the next unescaped `mark` at or after `from`, or -1. */
static size_t find_close(const char *s, size_t len, size_t from, const char *mark) {
  size_t m = strlen(mark);
  for (size_t i = from; i + m <= len; i++) {
    if (s[i] == '\\') {
      i++;
      continue;
    }
    if (strncmp(s + i, mark, m) == 0) return i == from ? (size_t)-1 : i;
  }
  return (size_t)-1;
}

/* Parses `[^id]` at s[i]; returns 1 and sets id bounds and end offset. */
static int footnote_ref(const char *s, size_t len, size_t i, size_t *id_start, size_t *id_end,
                        size_t *end) {
  if (i + 2 >= len || s[i + 1] != '^') return 0;
  const char *close = memchr(s + i + 2, ']', len - i - 2);
  if (!close) return 0;
  size_t c = (size_t)(close - s);
  if (c == i + 2) return 0;
  for (size_t k = i + 2; k < c; k++) {
    if (s[k] == ' ' || s[k] == '\t') return 0;
  }
  *id_start = i + 2;
  *id_end = c;
  *end = c;
  return 1;
}

/* Parses `[label](target)` at s[i]. */
static int parse_link(const char *s, size_t len, size_t i, size_t *label_start, size_t *label_end,
                      size_t *target_start, size_t *target_end, size_t *end) {
  size_t close = find_close(s, len, i + 1, "]");
  if (close == (size_t)-1 || close + 1 >= len || s[close + 1] != '(') return 0;
  size_t rel = balanced_end(s + close + 1);
  if (rel == (size_t)-1) return 0;

  size_t inner_start = close + 2;
  size_t inner_end = close + 1 + rel;
  while (inner_start < inner_end && is_space((unsigned char)s[inner_start])) inner_start++;
  while (inner_end > inner_start && is_space((unsigned char)s[inner_end - 1])) inner_end--;

  *label_start = i + 1;
  *label_end = close;
  *target_start = inner_start;
  *target_end = inner_end;
  *end = close + 1 + rel;
  return 1;
}

static void inline_html_into(buf *b, const char *s, size_t len);

static void inline_text_into(buf *b, const char *s, size_t len);

static void inline_html_into(buf *b, const char *s, size_t len) {
  for (size_t i = 0; i < len; i++) {
    char c = s[i];
    if (c == '\\' && i + 1 < len) {
      escape_byte(b, (unsigned char)s[++i]);
    } else if (c == '`') {
      const char *close = memchr(s + i + 1, '`', len - i - 1);
      if (close) {
        size_t end = (size_t)(close - s);
        buf_puts(b, "<code>");
        escape_into(b, s + i + 1, end - i - 1);
        buf_puts(b, "</code>");
        i = end;
      } else {
        buf_puts(b, "&#96;");
      }
    } else if (c == '*') {
      const char *mark = (i + 1 < len && s[i + 1] == '*') ? "**" : "*";
      const char *tag = mark[1] ? "strong" : "em";
      size_t mlen = strlen(mark);
      size_t end = find_close(s, len, i + mlen, mark);
      if (end != (size_t)-1) {
        buf_putc(b, '<');
        buf_puts(b, tag);
        buf_putc(b, '>');
        inline_html_into(b, s + i + mlen, end - i - mlen);
        buf_puts(b, "</");
        buf_puts(b, tag);
        buf_putc(b, '>');
        i = end + mlen - 1;
      } else {
        buf_putc(b, '*');
      }
    } else if (c == '[') {
      size_t ids, ide, end;
      if (footnote_ref(s, len, i, &ids, &ide, &end)) {
        buf esc = {0};
        escape_into(&esc, s + ids, ide - ids);
        char *e = buf_take(&esc);
        buf_puts(b, "<sup class=\"fnref\" id=\"fnref-");
        buf_puts(b, e);
        buf_puts(b, "\"><a href=\"#fn-");
        buf_puts(b, e);
        buf_puts(b, "\">");
        buf_puts(b, e);
        buf_puts(b, "</a></sup>");
        free(e);
        i = end;
        continue;
      }
      size_t ls, le, ts, te;
      if (parse_link(s, len, i, &ls, &le, &ts, &te, &end)) {
        buf_puts(b, "<a href=\"");
        escape_into(b, s + ts, te - ts);
        buf_puts(b, "\">");
        inline_html_into(b, s + ls, le - ls);
        buf_puts(b, "</a>");
        i = end;
      } else {
        buf_putc(b, '[');
      }
    } else {
      escape_byte(b, (unsigned char)c);
    }
  }
}

static void inline_text_into(buf *b, const char *s, size_t len) {
  for (size_t i = 0; i < len; i++) {
    char c = s[i];
    if (c == '\\' && i + 1 < len) {
      buf_putc(b, s[++i]);
    } else if (c == '`') {
      const char *close = memchr(s + i + 1, '`', len - i - 1);
      if (close) {
        size_t end = (size_t)(close - s);
        buf_add(b, s + i + 1, end - i - 1);
        i = end;
      } else {
        buf_putc(b, c);
      }
    } else if (c == '*') {
      const char *mark = (i + 1 < len && s[i + 1] == '*') ? "**" : "*";
      size_t mlen = strlen(mark);
      size_t end = find_close(s, len, i + mlen, mark);
      if (end != (size_t)-1) {
        inline_text_into(b, s + i + mlen, end - i - mlen);
        i = end + mlen - 1;
      } else {
        buf_putc(b, c);
      }
    } else if (c == '[') {
      size_t ids, ide, end;
      if (footnote_ref(s, len, i, &ids, &ide, &end)) {
        buf_putc(b, '[');
        buf_add(b, s + ids, ide - ids);
        buf_putc(b, ']');
        i = end;
        continue;
      }
      size_t ls, le, ts, te;
      if (parse_link(s, len, i, &ls, &le, &ts, &te, &end)) {
        inline_text_into(b, s + ls, le - ls);
        i = end;
      } else {
        buf_putc(b, c);
      }
    } else {
      buf_putc(b, c);
    }
  }
}

char *xtxt_inline_html(const char *s) {
  buf b = {0};
  if (s) inline_html_into(&b, s, strlen(s));
  return buf_take(&b);
}

char *xtxt_inline_text(const char *s) {
  buf b = {0};
  if (s) inline_text_into(&b, s, strlen(s));
  return buf_take(&b);
}

/* -------------------------------------------------------------------------- */
/* Validation                                                                  */
/* -------------------------------------------------------------------------- */

/*
 * The standard directives and whether each is a fenced block. Anything absent
 * is unknown: a warning, never an error, so a 1.0 reader stays usable on a
 * newer document.
 */
static int known_directive(const char *name, int *fenced) {
  static const char *const inline_names[] = {"xtxt",    "image",  "video", "audio",
                                             "attachment", "include", "embed", "hr"};
  static const char *const block_names[] = {
      "code",  "table",  "math",     "mermaid", "metadata", "comment", "raw",  "chart",
      "footnote", "task", "decision", "knowledge", "ai",    "prompt",  "chat", "note"};
  for (size_t i = 0; i < sizeof inline_names / sizeof *inline_names; i++) {
    if (strcmp(name, inline_names[i]) == 0) {
      *fenced = 0;
      return 1;
    }
  }
  for (size_t i = 0; i < sizeof block_names / sizeof *block_names; i++) {
    if (strcmp(name, block_names[i]) == 0) {
      *fenced = 1;
      return 1;
    }
  }
  return 0;
}

static int requires_src(const char *name) {
  static const char *const names[] = {"image", "video", "audio", "attachment", "include"};
  for (size_t i = 0; i < sizeof names / sizeof *names; i++) {
    if (strcmp(name, names[i]) == 0) return 1;
  }
  return 0;
}

static size_t chart_row_count(const xtxt_node *n) {
  size_t count = 0;
  const char *p = n->text;
  for (;;) {
    const char *nl = strchr(p, '\n');
    size_t len = nl ? (size_t)(nl - p) : strlen(p);
    char *line = xstrndup(p, len);
    if (!is_blank(line)) {
      size_t cells_n = 0;
      char **cells;
      if (memchr(p, '|', len)) {
        cells = split_cells(p, len, &cells_n);
      } else {
        /* `Label value`: the value is whatever follows the last space run. */
        size_t end = len;
        while (end > 0 && is_space((unsigned char)p[end - 1])) end--;
        size_t split = end;
        while (split > 0 && !is_space((unsigned char)p[split - 1])) split--;
        if (split > 0) {
          cells_n = 2;
          cells = xmalloc(2 * sizeof *cells);
          cells[0] = trim_dup(p, split);
          cells[1] = trim_dup(p + split, end - split);
        } else {
          cells_n = 1;
          cells = xmalloc(sizeof *cells);
          cells[0] = trim_dup(p, end);
        }
      }
      if (cells_n >= 2 && !is_separator_cells(cells, cells_n)) count++;
      free_cells(cells, cells_n);
    }
    free(line);
    if (!nl) break;
    p = nl + 1;
  }
  return count;
}

typedef struct {
  char **ids;
  size_t *lines;
  size_t count, cap;
} notelist;

static void notelist_push(notelist *l, const char *id, size_t line) {
  if (l->count == l->cap) {
    l->cap = l->cap ? l->cap * 2 : 8;
    l->ids = xrealloc(l->ids, l->cap * sizeof *l->ids);
    l->lines = xrealloc(l->lines, l->cap * sizeof *l->lines);
  }
  l->ids[l->count] = xstrdup(id);
  l->lines[l->count] = line;
  l->count++;
}

static int notelist_has(const notelist *l, const char *id, size_t n) {
  for (size_t i = 0; i < l->count; i++) {
    if (strlen(l->ids[i]) == n && strncmp(l->ids[i], id, n) == 0) return 1;
  }
  return 0;
}

/*
 * Pairs [^id] markers with @footnote blocks in both directions: a marker with
 * no note renders as a dead link, and a note nobody cites is usually a
 * leftover.
 */
static void check_footnote_refs(const xtxt_document *doc, const notelist *notes,
                                issuelist *issues) {
  strlist cited = {0};

  for (size_t i = 0; i < doc->node_count; i++) {
    const xtxt_node *n = &doc->nodes[i];
    size_t texts_n = 0;
    const char **texts = NULL;
    if (n->kind == XTXT_HEADING || n->kind == XTXT_PARAGRAPH || n->kind == XTXT_QUOTE) {
      texts = xmalloc(sizeof *texts);
      texts[0] = n->text;
      texts_n = 1;
    } else if (n->kind == XTXT_LIST && n->item_count > 0) {
      texts = xmalloc(n->item_count * sizeof *texts);
      for (size_t k = 0; k < n->item_count; k++) texts[k] = n->items[k].text;
      texts_n = n->item_count;
    } else {
      continue;
    }

    for (size_t k = 0; k < texts_n; k++) {
      const char *s = texts[k];
      size_t len = strlen(s);
      for (size_t j = 0; j < len; j++) {
        if (s[j] == '\\') {
          j++;
          continue;
        }
        if (s[j] != '[') continue;
        size_t ids, ide, end;
        if (!footnote_ref(s, len, j, &ids, &ide, &end)) continue;
        strlist_push(&cited, xstrndup(s + ids, ide - ids));
        if (!notelist_has(notes, s + ids, ide - ids)) {
          char *id = xstrndup(s + ids, ide - ids);
          issuelist_push(issues, XTXT_WARNING, n->line,
                         xasprintf("footnote reference [^%s] has no matching "
                                   "@footnote(id=\"%s\")",
                                   id, id));
          free(id);
        }
        j = end;
      }
    }
    free(texts);
  }

  for (size_t i = 0; i < notes->count; i++) {
    if (!notes->ids[i][0]) continue;
    int found = 0;
    for (size_t k = 0; k < cited.count; k++) {
      if (strcmp(cited.items[k], notes->ids[i]) == 0) {
        found = 1;
        break;
      }
    }
    if (!found) {
      issuelist_push(issues, XTXT_WARNING, notes->lines[i],
                     xasprintf("@footnote(id=\"%s\") is never referenced", notes->ids[i]));
    }
  }
  strlist_free(&cited);
}

xtxt_issues xtxt_validate(const xtxt_document *doc, const char *const *declared,
                          size_t declared_count) {
  issuelist issues = {0};
  xtxt_issues out = {0};
  if (!doc) return out;

  int metadata_seen = 0;
  notelist notes = {0};

  for (size_t i = 0; i < doc->node_count; i++) {
    const xtxt_node *n = &doc->nodes[i];
    if (n->kind != XTXT_DIRECTIVE && n->kind != XTXT_BLOCK) continue;

    int fenced = 0;
    if (!known_directive(n->name, &fenced)) {
      int declared_here = 0;
      for (size_t k = 0; k < declared_count; k++) {
        if (declared[k] && strcmp(declared[k], n->name) == 0) {
          declared_here = 1;
          break;
        }
      }
      if (!declared_here) {
        issuelist_push(&issues, XTXT_WARNING, n->line,
                       xasprintf("unknown directive @%s (preserved, but this "
                                 "reader cannot render it)",
                                 n->name));
      }
      continue;
    }

    if (fenced && n->kind != XTXT_BLOCK) {
      issuelist_push(&issues, XTXT_WARNING, n->line,
                     xasprintf("@%s is a block directive and should be closed with @end%s",
                               n->name, n->name));
    }
    if (!fenced && n->kind == XTXT_BLOCK) {
      issuelist_push(&issues, XTXT_WARNING, n->line,
                     xasprintf("@%s is not a block directive but was closed with @end%s",
                               n->name, n->name));
    }
    if (requires_src(n->name) && !xtxt_arg_resolve(n, "src")[0]) {
      issuelist_push(&issues, XTXT_WARNING, n->line, xasprintf("@%s has no src", n->name));
    }

    if (strcmp(n->name, "metadata") == 0) {
      if (metadata_seen) {
        issuelist_push(&issues, XTXT_WARNING, n->line, xstrdup("duplicate @metadata block"));
      }
      metadata_seen = 1;
    } else if (strcmp(n->name, "table") == 0) {
      xtxt_table t = xtxt_parse_table(n);
      if (t.column_count == 0) {
        issuelist_push(&issues, XTXT_WARNING, n->line, xstrdup("@table is empty"));
      } else {
        for (size_t r = 0; r < t.row_count; r++) {
          if (t.row_widths[r] != t.column_count) {
            issuelist_push(&issues, XTXT_WARNING, n->line + 1 + r,
                           xasprintf("table row has %zu cells, header has %zu", t.row_widths[r],
                                     t.column_count));
          }
        }
      }
      xtxt_table_free(t);
    } else if (strcmp(n->name, "code") == 0) {
      if (!xtxt_arg_resolve(n, "language")[0]) {
        issuelist_push(&issues, XTXT_WARNING, n->line,
                       xstrdup("@code has no language; syntax highlighting will be skipped"));
      }
    } else if (strcmp(n->name, "chart") == 0) {
      if (chart_row_count(n) == 0) {
        issuelist_push(&issues, XTXT_WARNING, n->line, xstrdup("@chart has no readable data rows"));
      }
    } else if (strcmp(n->name, "task") == 0) {
      xtxt_fields f = xtxt_node_fields(n);
      if (!xtxt_field_get(&f, "title")[0]) {
        issuelist_push(&issues, XTXT_WARNING, n->line, xstrdup("@task has no Title field"));
      }
      xtxt_fields_free(f);
    } else if (strcmp(n->name, "footnote") == 0) {
      const char *id = xtxt_arg_resolve(n, "id");
      if (!id[0]) {
        issuelist_push(&issues, XTXT_WARNING, n->line,
                       xstrdup("@footnote has no id; references cannot point at it"));
      }
      notelist_push(&notes, id, n->line);
    }
  }

  check_footnote_refs(doc, &notes, &issues);

  for (size_t i = 0; i < notes.count; i++) free(notes.ids[i]);
  free(notes.ids);
  free(notes.lines);

  out.issues = issues.items;
  out.count = issues.count;
  return out;
}

void xtxt_issues_free(xtxt_issues list) {
  for (size_t i = 0; i < list.count; i++) free(list.issues[i].message);
  free(list.issues);
}

void xtxt_sort_issues(xtxt_issue *issues, size_t count) {
  /* Insertion sort: stable, and these lists are short. */
  for (size_t i = 1; i < count; i++) {
    xtxt_issue key = issues[i];
    size_t j = i;
    while (j > 0 && issues[j - 1].line > key.line) {
      issues[j] = issues[j - 1];
      j--;
    }
    issues[j] = key;
  }
}
