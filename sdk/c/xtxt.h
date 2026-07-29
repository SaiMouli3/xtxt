/*
 * xtxt.h — a parser for the XTXT plain-text document format.
 *
 * C99, no dependencies. A port of the Go reference implementation, not a
 * binding: it agrees with it because both are checked against the same
 * conformance suite.
 *
 *     xtxt_result *r = xtxt_parse("# Hello\n\nworld\n", 0);
 *     for (size_t i = 0; i < r->doc.node_count; i++)
 *         puts(xtxt_kind_name(r->doc.nodes[i].kind));
 *     xtxt_result_free(r);
 *
 * Ownership is simple: every xtxt_* function that returns a pointer returns
 * memory you own, and each has exactly one matching free function. Strings
 * inside a document belong to the document and die with it.
 *
 * Spec: https://github.com/SaiMouli3/xtxt/blob/main/SPEC.md
 */

#ifndef XTXT_H
#define XTXT_H

#include <stddef.h>

#ifdef __cplusplus
extern "C" {
#endif

#define XTXT_VERSION "0.1.1"

/* The type of a node. */
typedef enum {
  XTXT_HEADING,
  XTXT_PARAGRAPH,
  XTXT_QUOTE,
  XTXT_LIST,
  XTXT_DIRECTIVE, /* inline form: @name(args)      */
  XTXT_BLOCK      /* fenced form: @name … @endname */
} xtxt_kind;

/* How serious a diagnostic is. */
typedef enum { XTXT_ERROR, XTXT_WARNING } xtxt_severity;

/* Tri-state for a checklist item. */
typedef enum {
  XTXT_UNCHECKED_NONE = -1, /* not a checklist item */
  XTXT_UNCHECKED = 0,
  XTXT_CHECKED = 1
} xtxt_check;

/* One argument of a directive. `key` is "" for a positional argument. */
typedef struct {
  char *key;
  char *value;
} xtxt_arg;

/* One entry in a list. */
typedef struct {
  char *text;
  int ordered;       /* 1 for `1.` style */
  xtxt_check checked; /* XTXT_UNCHECKED_NONE unless a checklist item */
} xtxt_item;

/* A single block in the document tree. */
typedef struct {
  xtxt_kind kind;
  char *name;  /* directive name, else "" */
  int level;   /* heading depth 1-6, else 0 */
  char *text;  /* heading/paragraph/quote text, or fenced payload */
  xtxt_arg *args;
  size_t arg_count;
  xtxt_item *items;
  size_t item_count;
  size_t line; /* 1-based line where the node starts */
} xtxt_node;

/* A parsed document. */
typedef struct {
  char *version;
  xtxt_node *nodes;
  size_t node_count;
} xtxt_document;

/* A diagnostic tied to a line of source. */
typedef struct {
  xtxt_severity severity;
  size_t line;
  char *message;
} xtxt_issue;

/* An owned list of diagnostics. Free with xtxt_issues_free. */
typedef struct {
  xtxt_issue *issues;
  size_t count;
} xtxt_issues;

/*
 * The outcome of parsing. A document is always returned, even when issues
 * contains errors — recovery is part of the format's compatibility guarantee.
 */
typedef struct {
  xtxt_document doc;
  xtxt_issue *issues;
  size_t issue_count;
} xtxt_result;

/*
 * Parse a document. `len` may be 0, in which case `src` is treated as a
 * NUL-terminated string. Returns NULL only if allocation fails.
 */
xtxt_result *xtxt_parse(const char *src, size_t len);

/* Release a parse result and every string inside it. Safe on NULL. */
void xtxt_result_free(xtxt_result *r);

/* Non-zero if any diagnostic is fatal. */
int xtxt_has_errors(const xtxt_result *r);

/* The human-readable name of a kind, as used in the conformance suite. */
const char *xtxt_kind_name(xtxt_kind kind);
const char *xtxt_severity_name(xtxt_severity severity);

/* ------------------------------------------------------------------------ */
/* Arguments                                                                 */
/* ------------------------------------------------------------------------ */

/* The value for `key`, or "" if absent. Borrowed; do not free. */
const char *xtxt_arg_get(const xtxt_node *n, const char *key);

/* The i-th argument that had no key, or "". Borrowed. */
const char *xtxt_arg_positional(const xtxt_node *n, size_t i);

/*
 * xtxt_arg_get falling back to the first positional argument. This is what
 * makes @video("x.mp4") and @video(src="x.mp4") equivalent. Borrowed.
 */
const char *xtxt_arg_resolve(const xtxt_node *n, const char *key);

/* ------------------------------------------------------------------------ */
/* Records                                                                   */
/* ------------------------------------------------------------------------ */

/* One `Key: value` entry in a block payload. */
typedef struct {
  char *key;
  char *value;
} xtxt_field;

/*
 * A block payload read as an ordered record. Order matters: a @chat block's
 * turns are fields, and their sequence is the conversation.
 */
typedef struct {
  xtxt_field *fields;
  size_t count;
} xtxt_fields;

/* Interpret a block payload as a record. Free with xtxt_fields_free. */
xtxt_fields xtxt_parse_fields(const char *payload);

/* The payload of a block node as a record; empty for other kinds. */
xtxt_fields xtxt_node_fields(const xtxt_node *n);

/* The first value whose key matches case-insensitively, or "". Borrowed. */
const char *xtxt_field_get(const xtxt_fields *f, const char *key);

void xtxt_fields_free(xtxt_fields f);

/* ------------------------------------------------------------------------ */
/* Tables                                                                    */
/* ------------------------------------------------------------------------ */

/* The interpreted payload of an @table block. */
typedef struct {
  char **header;
  size_t column_count;
  char ***rows; /* row_count arrays, each of column_count_per_row[i] cells */
  size_t *row_widths;
  size_t row_count;
  char **align; /* "left", "right" or "center", column_count entries */
} xtxt_table;

xtxt_table xtxt_parse_table(const xtxt_node *n);
void xtxt_table_free(xtxt_table t);

/* ------------------------------------------------------------------------ */
/* Inline formatting                                                         */
/* ------------------------------------------------------------------------ */

/* Convert inline markup to HTML, escaping everything else. Caller frees. */
char *xtxt_inline_html(const char *s);

/* Strip inline markup, for plain text and analysis. Caller frees. */
char *xtxt_inline_text(const char *s);

/* ------------------------------------------------------------------------ */
/* Validation                                                                */
/* ------------------------------------------------------------------------ */

/*
 * Semantic checks on top of the parser's syntactic ones.
 *
 * `declared` may name directives supplied by plugins (a plugin manifest is a
 * declaration that the directive exists); pass NULL and 0 for none.
 */
xtxt_issues xtxt_validate(const xtxt_document *doc, const char *const *declared,
                          size_t declared_count);

void xtxt_issues_free(xtxt_issues list);

/* Sort issues by line, stably. Sorts in place. */
void xtxt_sort_issues(xtxt_issue *issues, size_t count);

#ifdef __cplusplus
} /* extern "C" */
#endif

#endif /* XTXT_H */
