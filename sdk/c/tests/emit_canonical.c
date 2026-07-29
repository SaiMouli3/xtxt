/*
 * Emits the conformance-suite view of one document as JSON on stdout.
 *
 * The comparison itself is done by tests/run_conformance.py: writing a JSON
 * *parser* in C purely to read the expectation files would be more test code
 * than library code, and the library stays dependency-free either way.
 */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "../xtxt.h"

static void json_string(const char *s) {
  putchar('"');
  for (const unsigned char *p = (const unsigned char *)s; *p; p++) {
    switch (*p) {
      case '"': fputs("\\\"", stdout); break;
      case '\\': fputs("\\\\", stdout); break;
      case '\n': fputs("\\n", stdout); break;
      case '\r': fputs("\\r", stdout); break;
      case '\t': fputs("\\t", stdout); break;
      default:
        if (*p < 0x20) {
          printf("\\u%04x", *p);
        } else {
          putchar((int)*p);
        }
    }
  }
  putchar('"');
}

static char *read_all(const char *path, size_t *len) {
  FILE *f = fopen(path, "rb");
  if (!f) return NULL;
  if (fseek(f, 0, SEEK_END) != 0) { fclose(f); return NULL; }
  long size = ftell(f);
  if (size < 0) { fclose(f); return NULL; }
  rewind(f);
  char *buf = malloc((size_t)size + 1);
  if (!buf) { fclose(f); return NULL; }
  size_t got = fread(buf, 1, (size_t)size, f);
  fclose(f);
  buf[got] = '\0';
  *len = got;
  return buf;
}

int main(int argc, char **argv) {
  if (argc != 2) {
    fprintf(stderr, "usage: emit_canonical <file.xtxt>\n");
    return 2;
  }
  size_t len = 0;
  char *src = read_all(argv[1], &len);
  if (!src) {
    perror(argv[1]);
    return 2;
  }

  xtxt_result *r = xtxt_parse(src, len);
  free(src);
  if (!r) return 2;

  printf("{\"ast\":{\"version\":");
  json_string(r->doc.version);
  printf(",\"nodes\":[");
  for (size_t i = 0; i < r->doc.node_count; i++) {
    const xtxt_node *n = &r->doc.nodes[i];
    if (i) putchar(',');
    printf("{\"kind\":");
    json_string(xtxt_kind_name(n->kind));
    printf(",\"name\":");
    json_string(n->name);
    printf(",\"level\":%d,\"text\":", n->level);
    json_string(n->text);
    printf(",\"args\":[");
    for (size_t k = 0; k < n->arg_count; k++) {
      if (k) putchar(',');
      printf("{\"key\":");
      json_string(n->args[k].key);
      printf(",\"value\":");
      json_string(n->args[k].value);
      putchar('}');
    }
    printf("],\"items\":[");
    for (size_t k = 0; k < n->item_count; k++) {
      if (k) putchar(',');
      printf("{\"text\":");
      json_string(n->items[k].text);
      printf(",\"ordered\":%s,\"checked\":%s}", n->items[k].ordered ? "true" : "false",
             n->items[k].checked == XTXT_UNCHECKED_NONE
                 ? "null"
                 : (n->items[k].checked == XTXT_CHECKED ? "true" : "false"));
    }
    printf("],\"line\":%zu}", n->line);
  }
  printf("]},\"issues\":[");

  xtxt_issues extra = xtxt_validate(&r->doc, NULL, 0);
  size_t total = r->issue_count + extra.count;
  xtxt_issue *all = malloc((total ? total : 1) * sizeof *all);
  if (!all) { xtxt_issues_free(extra); xtxt_result_free(r); return 2; }
  memcpy(all, r->issues, r->issue_count * sizeof *all);
  memcpy(all + r->issue_count, extra.issues, extra.count * sizeof *all);
  xtxt_sort_issues(all, total);
  for (size_t i = 0; i < total; i++) {
    if (i) putchar(',');
    printf("{\"severity\":");
    json_string(xtxt_severity_name(all[i].severity));
    printf(",\"line\":%zu}", all[i].line);
  }
  printf("]}\n");

  free(all);
  xtxt_issues_free(extra);
  xtxt_result_free(r);
  return 0;
}
