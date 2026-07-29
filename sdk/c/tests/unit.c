/* Unit checks that do not need the shared fixtures. */

#include <assert.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "../xtxt.h"

static void check(int cond, const char *what) {
  if (!cond) {
    fprintf(stderr, "FAIL: %s\n", what);
    exit(1);
  }
}

static void test_text_blocks(void) {
  xtxt_result *r = xtxt_parse("# Title\n\nA para\nspanning lines.\n\n- one\n- [x] done\n", 0);
  check(!xtxt_has_errors(r), "no errors");
  check(r->doc.node_count == 3, "three nodes");
  check(r->doc.nodes[0].kind == XTXT_HEADING && r->doc.nodes[0].level == 1, "h1");
  check(strcmp(r->doc.nodes[1].text, "A para spanning lines.") == 0, "soft wrap");
  check(r->doc.nodes[2].items[1].checked == XTXT_CHECKED, "checklist");
  xtxt_result_free(r);
}

static void test_multiline_directive(void) {
  xtxt_result *r = xtxt_parse("@image(\n  src=\"a.png\",\n  caption=\"A, B\"\n)\n\nafter\n", 0);
  check(strcmp(xtxt_arg_resolve(&r->doc.nodes[0], "src"), "a.png") == 0, "src");
  check(strcmp(xtxt_arg_get(&r->doc.nodes[0], "caption"), "A, B") == 0, "comma in quotes");
  check(strcmp(r->doc.nodes[1].text, "after") == 0, "parser kept its place");
  xtxt_result_free(r);
}

static void test_positional(void) {
  xtxt_result *r = xtxt_parse("@video(\"demo.mp4\")", 0);
  check(strcmp(xtxt_arg_resolve(&r->doc.nodes[0], "src"), "demo.mp4") == 0, "positional resolve");
  xtxt_result_free(r);
}

static void test_unknown_is_not_an_error(void) {
  xtxt_result *r = xtxt_parse("@futurething(a=1)\n\n@newblock\nbody\n@endnewblock\n", 0);
  check(!xtxt_has_errors(r), "unknown directives are not errors");
  check(r->doc.node_count == 2, "both survive");
  check(strcmp(r->doc.nodes[1].text, "body") == 0, "payload survives");
  xtxt_issues v = xtxt_validate(&r->doc, NULL, 0);
  check(v.count == 2, "two warnings");
  xtxt_issues_free(v);
  xtxt_result_free(r);
}

static void test_plugin_declared_directive(void) {
  xtxt_result *r = xtxt_parse("@youtube(id=\"abc\")\n", 0);
  const char *declared[] = {"youtube"};
  xtxt_issues v = xtxt_validate(&r->doc, declared, 1);
  check(v.count == 0, "a declared directive is not unknown");
  xtxt_issues_free(v);
  xtxt_result_free(r);
}

static void test_inline(void) {
  char *h = xtxt_inline_html("**b** and `a<b`");
  check(strcmp(h, "<strong>b</strong> and <code>a&lt;b</code>") == 0, "inline html");
  free(h);

  char *t = xtxt_inline_text("**b** and [x](y)");
  check(strcmp(t, "b and x") == 0, "inline text");
  free(t);

  /* Byte-oriented escaping must not corrupt multi-byte characters. */
  char *u = xtxt_inline_html("an em dash — and 日本語 & more");
  check(strstr(u, "—") != NULL && strstr(u, "日本語") != NULL, "utf-8 survives");
  check(strstr(u, "&amp;") != NULL, "ampersand escaped");
  free(u);
}

static void test_fields(void) {
  xtxt_fields f = xtxt_parse_fields("Title: Ship it\nStatus: Done\nOwner: Subbu");
  check(f.count == 3, "three fields");
  check(strcmp(xtxt_field_get(&f, "title"), "Ship it") == 0, "case-insensitive get");
  check(strcmp(f.fields[1].key, "Status") == 0, "order preserved");
  xtxt_fields_free(f);

  xtxt_fields multi = xtxt_parse_fields("Summary:\nLine one\nLine two\n\nTags: a, b");
  check(strcmp(xtxt_field_get(&multi, "Summary"), "Line one\nLine two") == 0, "multiline value");
  check(strcmp(xtxt_field_get(&multi, "Tags"), "a, b") == 0, "field after multiline");
  xtxt_fields_free(multi);

  /* A colon deep in a sentence is prose, not a field. */
  xtxt_fields prose = xtxt_parse_fields("There is one rule that matters here: keep it readable.");
  check(prose.count == 1 && prose.fields[0].key[0] == '\0', "prose is not a field");
  xtxt_fields_free(prose);
}

static void test_table(void) {
  xtxt_result *r = xtxt_parse("@table\nName | Age\n-----|----:\nJohn | 20\n\nAlice | 22\n@endtable\n", 0);
  xtxt_table t = xtxt_parse_table(&r->doc.nodes[0]);
  check(t.column_count == 2, "two columns");
  check(strcmp(t.header[0], "Name") == 0, "header");
  check(t.row_count == 2, "blank lines between rows ignored");
  check(strcmp(t.rows[1][1], "22") == 0, "second row");
  check(strcmp(t.align[1], "right") == 0, "alignment");
  xtxt_table_free(t);
  xtxt_result_free(r);
}

static void test_escaped_at(void) {
  xtxt_result *r = xtxt_parse("\\@image is written like this\n", 0);
  check(r->doc.nodes[0].kind == XTXT_PARAGRAPH, "paragraph");
  check(strcmp(r->doc.nodes[0].text, "@image is written like this") == 0, "escape");
  xtxt_result_free(r);
}

static void test_unclosed_fence(void) {
  xtxt_result *r = xtxt_parse("@code\nprint()\n", 0);
  check(xtxt_has_errors(r), "unclosed fence is an error");
  xtxt_result_free(r);
}

static void test_empty_input(void) {
  xtxt_result *r = xtxt_parse("", 0);
  check(r != NULL && r->doc.node_count == 0, "empty input parses");
  xtxt_result_free(r);
}

int main(void) {
  test_text_blocks();
  test_multiline_directive();
  test_positional();
  test_unknown_is_not_an_error();
  test_plugin_declared_directive();
  test_inline();
  test_fields();
  test_table();
  test_escaped_at();
  test_unclosed_fence();
  test_empty_input();
  puts("all C unit tests passed");
  return 0;
}
