// Unit checks plus a smoke pass over the shared conformance fixtures.
//
// The byte-exact AST comparison lives in the C SDK, which owns the parser; this
// suite proves the C++ layer surfaces the same tree without dropping or
// corrupting anything on the way across.

#include <cstdio>
#include <cstdlib>
#include <filesystem>
#include <fstream>
#include <iostream>
#include <sstream>
#include <string>
#include <vector>

#include "xtxt.hpp"

namespace {

int failures = 0;

void check(bool cond, const std::string& what) {
  if (!cond) {
    std::cerr << "FAIL: " << what << "\n";
    failures++;
  }
}

template <typename A, typename B>
void check_eq(const A& got, const B& want, const std::string& what) {
  if (!(got == want)) {
    std::cerr << "FAIL: " << what << "\n  got:  " << got << "\n  want: " << want << "\n";
    failures++;
  }
}

void test_text_blocks() {
  auto res = xtxt::parse("# Title\n\nA para\nspanning lines.\n\n- one\n- [x] done\n");
  check(!res.has_errors(), "no errors");
  check_eq(res.doc.nodes.size(), std::size_t{3}, "three nodes");
  check(res.doc.nodes[0].kind == xtxt::Kind::Heading, "heading");
  check_eq(res.doc.nodes[0].level, 1, "level");
  check_eq(res.doc.nodes[1].text, std::string("A para spanning lines."), "soft wrap");
  check(res.doc.nodes[2].items[1].checked.value_or(false), "checklist");
}

void test_arguments() {
  auto res = xtxt::parse("@image(\n  src=\"a.png\",\n  caption=\"A, B\"\n)\n\nafter\n");
  check_eq(res.doc.nodes[0].resolve("src"), std::string("a.png"), "src");
  check_eq(res.doc.nodes[0].arg("caption"), std::string("A, B"), "comma inside quotes");
  check_eq(res.doc.nodes[1].text, std::string("after"), "parser kept its place");

  auto pos = xtxt::parse("@video(\"demo.mp4\")");
  check_eq(pos.doc.nodes[0].resolve("src"), std::string("demo.mp4"), "positional resolve");
}

void test_records() {
  auto res = xtxt::parse("@task\nTitle: Ship it\nStatus: Done\nOwner: Subbu\n@endtask\n");
  auto f = res.doc.nodes[0].fields();
  check_eq(f.size(), std::size_t{3}, "three fields");
  check_eq(f.get("title"), std::string("Ship it"), "case-insensitive get");
  check_eq(f.list[1].key, std::string("Status"), "order preserved");

  auto e = xtxt::extract(res.doc);
  check_eq(e.tasks.size(), std::size_t{1}, "one task");
  check_eq(e.tasks[0].title, std::string("Ship it"), "task title");
  check(e.tasks[0].done, "task done");
  check_eq(e.blocks[0].order.size(), std::size_t{3}, "field order captured");
}

void test_prose_is_not_a_field() {
  auto f = xtxt::parse_fields("There is one rule that matters here: keep it readable.");
  check_eq(f.size(), std::size_t{1}, "one entry");
  check(f.list[0].key.empty(), "prose is not a field");
}

void test_inline() {
  check_eq(xtxt::inline_html("**b** and `a<b`"),
           std::string("<strong>b</strong> and <code>a&lt;b</code>"), "inline html");
  check_eq(xtxt::inline_text("**b** and [x](y)"), std::string("b and x"), "inline text");

  auto utf8 = xtxt::inline_html("an em dash — and 日本語 & more");
  check(utf8.find("—") != std::string::npos, "em dash survives");
  check(utf8.find("日本語") != std::string::npos, "cjk survives");
  check(utf8.find("&amp;") != std::string::npos, "ampersand escaped");
}

void test_extract_and_render() {
  auto res = xtxt::parse(
      "# T\n\nProse with a [link](https://example.com).\n\n"
      "@decision\nTitle: Use one pass\nWhy: simplicity\n@enddecision\n");
  auto e = xtxt::extract(res.doc);
  check_eq(e.outline.size(), std::size_t{1}, "one heading");
  check_eq(e.links.size(), std::size_t{1}, "one link");
  check_eq(e.links[0].href, std::string("https://example.com"), "link href");
  check(e.words > 0, "word count");

  auto html = xtxt::render_html(res.doc);
  check(html.find("data-type=\"decision\"") != std::string::npos, "record rendered");
  check(html.find("<dt>Title</dt><dd>Use one pass</dd>") != std::string::npos, "record fields");
  check(html.find("<h1 id=\"t\">T</h1>") != std::string::npos, "heading rendered with anchor");
}

void test_table() {
  auto res = xtxt::parse("@table\nName | Age\n-----|----:\nJohn | 20\n@endtable\n");
  auto t = xtxt::parse_table(res.doc.nodes[0]);
  check_eq(t.header.size(), std::size_t{2}, "two columns");
  check_eq(t.header[0], std::string("Name"), "header");
  check_eq(t.rows.size(), std::size_t{1}, "one row");
  check_eq(t.align[1], std::string("right"), "alignment");
}

void test_validate() {
  auto issues = xtxt::validate("@futurething(a=1)\n");
  check_eq(issues.size(), std::size_t{1}, "one warning");
  check(issues[0].severity == xtxt::Severity::Warning, "unknown is a warning, not an error");

  auto declared = xtxt::validate("@youtube(id=\"abc\")\n", {"youtube"});
  check(declared.empty(), "a plugin-declared directive is not unknown");
}

// Every fixture must parse, extract and render without crashing, and every
// node the C parser produced must survive the crossing into C++.
void test_conformance_smoke(const std::filesystem::path& dir) {
  if (!std::filesystem::is_directory(dir)) {
    std::cerr << "FAIL: conformance cases not found at " << dir << "\n";
    failures++;
    return;
  }
  std::size_t cases = 0;
  for (const auto& entry : std::filesystem::directory_iterator(dir)) {
    if (entry.path().extension() != ".xtxt") continue;
    cases++;
    std::ifstream in(entry.path());
    std::stringstream ss;
    ss << in.rdbuf();
    auto source = ss.str();

    auto res = xtxt::parse(source);
    for (const auto& n : res.doc.nodes) {
      check(std::string(xtxt::kind_name(n.kind)) != "unknown", "every kind is nameable");
      check(n.line > 0, entry.path().filename().string() + ": every node has a line");
    }
    (void)xtxt::extract(res.doc);
    (void)xtxt::render_html(res.doc);
    (void)xtxt::validate(source);
  }
  check(cases >= 10, "found the conformance cases");
}

}  // namespace

int main(int argc, char** argv) {
  test_text_blocks();
  test_arguments();
  test_records();
  test_prose_is_not_a_field();
  test_inline();
  test_extract_and_render();
  test_table();
  test_validate();
  test_conformance_smoke(argc > 1 ? argv[1] : "../../conformance/cases");

  if (failures) {
    std::cerr << failures << " check(s) failed\n";
    return 1;
  }
  std::cout << "all C++ tests passed\n";
  return 0;
}
