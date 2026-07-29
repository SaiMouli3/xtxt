// xtxt.hpp — C++ interface to the XTXT plain-text document format.
//
// This is a header-only RAII wrapper over the C library rather than a sixth
// hand-written parser. One parser means the C and C++ SDKs cannot disagree
// about the format — only about ergonomics, which is where they should differ.
//
//     auto res = xtxt::parse(source);
//     for (const auto& n : res.doc.nodes) std::cout << kind_name(n.kind) << '\n';
//     auto data = xtxt::extract(res.doc);
//
// Parsing copies out of the C structures immediately, so nothing here can
// dangle and there is nothing to free by hand.
//
// Requires C++17. Link against the C library (see CMakeLists.txt).
//
// Spec: https://github.com/SaiMouli3/xtxt/blob/main/SPEC.md

#ifndef XTXT_HPP
#define XTXT_HPP

#include <algorithm>
#include <map>
#include <memory>
#include <optional>
#include <string>
#include <string_view>
#include <vector>

extern "C" {
#include "xtxt.h"
}

namespace xtxt {

// ---------------------------------------------------------------------------
// Model
// ---------------------------------------------------------------------------

/// The type of a Node.
enum class Kind { Heading, Paragraph, Quote, List, Directive, Block };

/// How serious a diagnostic is.
enum class Severity { Error, Warning };

inline const char* kind_name(Kind k) {
  switch (k) {
    case Kind::Heading: return "heading";
    case Kind::Paragraph: return "paragraph";
    case Kind::Quote: return "quote";
    case Kind::List: return "list";
    case Kind::Directive: return "directive";
    case Kind::Block: return "block";
  }
  return "unknown";
}

inline const char* severity_name(Severity s) {
  return s == Severity::Error ? "error" : "warning";
}

/// One argument of a directive. `key` is empty for a positional argument.
struct Arg {
  std::string key;
  std::string value;
};

/// One entry in a list. `checked` is unset unless it is a checklist item.
struct Item {
  std::string text;
  bool ordered = false;
  std::optional<bool> checked;
};

/// One `Key: value` entry in a block payload.
struct Field {
  std::string key;
  std::string value;
};

/// A block payload read as an ordered record. Order matters: a `@chat` block's
/// turns are fields, and their sequence is the conversation.
class Fields {
 public:
  std::vector<Field> list;

  /// The first value whose key matches case-insensitively, or "".
  std::string get(std::string_view key) const {
    for (const auto& f : list) {
      if (iequals(f.key, key)) return f.value;
    }
    return {};
  }

  /// Flatten to a lowercase-keyed map, keeping the first of any duplicate.
  std::map<std::string, std::string> map() const {
    std::map<std::string, std::string> out;
    for (const auto& f : list) out.emplace(lower(f.key), f.value);
    return out;
  }

  bool empty() const { return list.empty(); }
  std::size_t size() const { return list.size(); }
  auto begin() const { return list.begin(); }
  auto end() const { return list.end(); }

 private:
  static std::string lower(std::string s) {
    std::transform(s.begin(), s.end(), s.begin(),
                   [](unsigned char c) { return static_cast<char>(std::tolower(c)); });
    return s;
  }
  static bool iequals(std::string_view a, std::string_view b) {
    return a.size() == b.size() &&
           std::equal(a.begin(), a.end(), b.begin(), [](char x, char y) {
             return std::tolower(static_cast<unsigned char>(x)) ==
                    std::tolower(static_cast<unsigned char>(y));
           });
  }
};

/// A single block in the document tree.
struct Node {
  Kind kind = Kind::Paragraph;
  std::string name;
  int level = 0;
  std::string text;
  std::vector<Arg> args;
  std::vector<Item> items;
  std::size_t line = 0;

  /// The value for `key`, or "".
  std::string arg(std::string_view key) const {
    for (const auto& a : args) {
      if (a.key == key) return a.value;
    }
    return {};
  }

  /// The i-th argument that had no key, or "".
  std::string positional(std::size_t i) const {
    std::size_t n = 0;
    for (const auto& a : args) {
      if (a.key.empty()) {
        if (n == i) return a.value;
        n++;
      }
    }
    return {};
  }

  /// `arg` falling back to the first positional argument, which is what makes
  /// `@video("x.mp4")` and `@video(src="x.mp4")` equivalent.
  std::string resolve(std::string_view key) const {
    auto named = arg(key);
    return named.empty() ? positional(0) : named;
  }

  /// The payload read as an ordered record.
  Fields fields() const;
};

/// A parsed document.
struct Document {
  std::string version;
  std::vector<Node> nodes;

  /// The `@metadata` block as key/value pairs, with keys lowercased.
  std::map<std::string, std::string> metadata() const {
    for (const auto& n : nodes) {
      if (n.kind == Kind::Block && n.name == "metadata") {
        std::map<std::string, std::string> out;
        std::size_t start = 0;
        while (start <= n.text.size()) {
          auto nl = n.text.find('\n', start);
          auto line = n.text.substr(start, nl == std::string::npos ? std::string::npos : nl - start);
          auto eq = line.find('=');
          if (eq != std::string::npos) {
            auto key = trim(line.substr(0, eq));
            std::transform(key.begin(), key.end(), key.begin(),
                           [](unsigned char c) { return static_cast<char>(std::tolower(c)); });
            out.emplace(key, trim(line.substr(eq + 1)));
          }
          if (nl == std::string::npos) break;
          start = nl + 1;
        }
        return out;
      }
    }
    return {};
  }

 private:
  static std::string trim(std::string_view s) {
    auto begin = s.find_first_not_of(" \t\r\n\v\f");
    if (begin == std::string_view::npos) return {};
    auto end = s.find_last_not_of(" \t\r\n\v\f");
    return std::string(s.substr(begin, end - begin + 1));
  }
};

/// A diagnostic tied to a line of source.
struct Issue {
  Severity severity = Severity::Warning;
  std::size_t line = 0;
  std::string message;
};

/// The outcome of parsing. A document is always returned, even when `issues`
/// contains errors — recovery is part of the format's compatibility guarantee.
struct Result {
  Document doc;
  std::vector<Issue> issues;

  bool has_errors() const {
    return std::any_of(issues.begin(), issues.end(),
                       [](const Issue& i) { return i.severity == Severity::Error; });
  }
};

// ---------------------------------------------------------------------------
// Bridging
// ---------------------------------------------------------------------------

namespace detail {

/// Frees a C string returned by the library.
struct CFree {
  void operator()(char* p) const noexcept { std::free(p); }
};
using CString = std::unique_ptr<char, CFree>;

inline std::string take(char* owned) {
  CString guard(owned);
  return owned ? std::string(owned) : std::string();
}

inline Kind from_c(xtxt_kind k) {
  switch (k) {
    case XTXT_HEADING: return Kind::Heading;
    case XTXT_PARAGRAPH: return Kind::Paragraph;
    case XTXT_QUOTE: return Kind::Quote;
    case XTXT_LIST: return Kind::List;
    case XTXT_DIRECTIVE: return Kind::Directive;
    case XTXT_BLOCK: return Kind::Block;
  }
  return Kind::Paragraph;
}

inline Severity from_c(xtxt_severity s) {
  return s == XTXT_ERROR ? Severity::Error : Severity::Warning;
}

/// Owns a C parse result for exactly as long as it takes to copy out of it.
struct ResultGuard {
  xtxt_result* raw = nullptr;
  explicit ResultGuard(xtxt_result* r) : raw(r) {}
  ~ResultGuard() { xtxt_result_free(raw); }
  ResultGuard(const ResultGuard&) = delete;
  ResultGuard& operator=(const ResultGuard&) = delete;
};

struct IssuesGuard {
  xtxt_issues raw{};
  explicit IssuesGuard(xtxt_issues i) : raw(i) {}
  ~IssuesGuard() { xtxt_issues_free(raw); }
  IssuesGuard(const IssuesGuard&) = delete;
  IssuesGuard& operator=(const IssuesGuard&) = delete;
};

struct FieldsGuard {
  xtxt_fields raw{};
  explicit FieldsGuard(xtxt_fields f) : raw(f) {}
  ~FieldsGuard() { xtxt_fields_free(raw); }
  FieldsGuard(const FieldsGuard&) = delete;
  FieldsGuard& operator=(const FieldsGuard&) = delete;
};

}  // namespace detail

/// Parse a document. Never throws on malformed input: problems become issues.
inline Result parse(std::string_view src) {
  Result out;
  detail::ResultGuard guard(xtxt_parse(src.data(), src.size()));
  if (!guard.raw) return out;

  const xtxt_result* r = guard.raw;
  out.doc.version = r->doc.version ? r->doc.version : "";
  out.doc.nodes.reserve(r->doc.node_count);
  for (std::size_t i = 0; i < r->doc.node_count; i++) {
    const xtxt_node& cn = r->doc.nodes[i];
    Node n;
    n.kind = detail::from_c(cn.kind);
    n.name = cn.name;
    n.level = cn.level;
    n.text = cn.text;
    n.line = cn.line;
    n.args.reserve(cn.arg_count);
    for (std::size_t k = 0; k < cn.arg_count; k++) {
      n.args.push_back(Arg{cn.args[k].key, cn.args[k].value});
    }
    n.items.reserve(cn.item_count);
    for (std::size_t k = 0; k < cn.item_count; k++) {
      Item it;
      it.text = cn.items[k].text;
      it.ordered = cn.items[k].ordered != 0;
      if (cn.items[k].checked != XTXT_UNCHECKED_NONE) {
        it.checked = cn.items[k].checked == XTXT_CHECKED;
      }
      n.items.push_back(std::move(it));
    }
    out.doc.nodes.push_back(std::move(n));
  }
  out.issues.reserve(r->issue_count);
  for (std::size_t i = 0; i < r->issue_count; i++) {
    out.issues.push_back(
        Issue{detail::from_c(r->issues[i].severity), r->issues[i].line, r->issues[i].message});
  }
  return out;
}

/// Interpret a block payload as an ordered record.
inline Fields parse_fields(std::string_view payload) {
  std::string owned(payload);
  detail::FieldsGuard guard(xtxt_parse_fields(owned.c_str()));
  Fields out;
  out.list.reserve(guard.raw.count);
  for (std::size_t i = 0; i < guard.raw.count; i++) {
    out.list.push_back(Field{guard.raw.fields[i].key, guard.raw.fields[i].value});
  }
  return out;
}

inline Fields Node::fields() const {
  return kind == Kind::Block ? parse_fields(text) : Fields{};
}

/// Convert inline markup to HTML, escaping everything else.
inline std::string inline_html(std::string_view s) {
  std::string owned(s);
  return detail::take(xtxt_inline_html(owned.c_str()));
}

/// Strip inline markup, for plain text and analysis.
inline std::string inline_text(std::string_view s) {
  std::string owned(s);
  return detail::take(xtxt_inline_text(owned.c_str()));
}

/// The interpreted payload of an `@table` block.
struct Table {
  std::vector<std::string> header;
  std::vector<std::vector<std::string>> rows;
  std::vector<std::string> align;
};

inline Table parse_table(const Node& n) {
  // The C table parser reads a node, so rebuild the smallest document that
  // carries the payload rather than duplicating the row logic here.
  Table out;
  const std::string source = "@table\n" + n.text + "\n@endtable\n";
  detail::ResultGuard guard(xtxt_parse(source.c_str(), source.size()));
  if (!guard.raw || guard.raw->doc.node_count == 0) return out;

  xtxt_table t = xtxt_parse_table(&guard.raw->doc.nodes[0]);
  out.header.assign(t.header, t.header + t.column_count);
  out.align.assign(t.align, t.align + (t.align ? t.column_count : 0));
  out.rows.reserve(t.row_count);
  for (std::size_t i = 0; i < t.row_count; i++) {
    out.rows.emplace_back(t.rows[i], t.rows[i] + t.row_widths[i]);
  }
  xtxt_table_free(t);
  return out;
}

/// Semantic checks on top of the parser's syntactic ones. Names in `declared`
/// are treated as known, for directives a plugin manifest supplies.
///
/// This takes the source rather than a Document: validation runs in the C
/// library, which needs its own tree, and re-parsing is cheaper and far less
/// error-prone than marshalling a C++ Document back across the boundary.
inline std::vector<Issue> validate(std::string_view src,
                                   const std::vector<std::string>& declared = {}) {
  detail::ResultGuard guard(xtxt_parse(src.data(), src.size()));
  std::vector<Issue> out;
  if (!guard.raw) return out;

  std::vector<const char*> names;
  names.reserve(declared.size());
  for (const auto& d : declared) names.push_back(d.c_str());

  detail::IssuesGuard issues(
      xtxt_validate(&guard.raw->doc, names.empty() ? nullptr : names.data(), names.size()));
  out.reserve(issues.raw.count);
  for (std::size_t i = 0; i < issues.raw.count; i++) {
    out.push_back(Issue{detail::from_c(issues.raw.issues[i].severity), issues.raw.issues[i].line,
                        issues.raw.issues[i].message});
  }
  return out;
}

// ---------------------------------------------------------------------------
// Extraction — the machine-facing view
// ---------------------------------------------------------------------------

struct Outline {
  int level = 0;
  std::string text;
  std::size_t line = 0;
};

struct Task {
  std::string title;
  bool done = false;
  std::string status, owner, due;
  std::size_t line = 0;
};

struct Block {
  std::string type;
  std::size_t line = 0;
  std::map<std::string, std::string> args;
  std::map<std::string, std::string> fields;
  std::vector<std::string> order;
  std::string text;
};

struct Link {
  std::string text, href;
  std::size_t line = 0;
};

struct Media {
  std::string kind, src, caption;
  std::size_t line = 0;
};

struct Code {
  std::string language;
  std::size_t lines = 0, line = 0;
  std::string source;
};

/// Everything an agent needs without inferring structure from prose.
struct Extraction {
  std::string version;
  std::map<std::string, std::string> metadata;
  std::vector<Outline> outline;
  std::vector<Task> tasks;
  std::vector<Block> blocks;
  std::vector<Link> links;
  std::vector<Media> media;
  std::vector<Code> code;
  std::string text;
  std::size_t words = 0;
};

namespace detail {

inline bool is_presentational(const std::string& name) {
  static const std::vector<std::string> names = {
      "code",  "table", "math",       "mermaid", "metadata", "comment", "raw",     "image",
      "video", "audio", "attachment", "hr",      "xtxt",     "include", "embed",   "footnote"};
  return std::find(names.begin(), names.end(), name) != names.end();
}

/// Links found in a run of text, using the C inline scanner for the label so
/// nested markup is stripped identically to every other implementation.
inline std::vector<Link> links_in(const std::string& s, std::size_t line) {
  std::vector<Link> out;
  for (std::size_t i = 0; i < s.size(); i++) {
    if (s[i] == '\\') {
      i++;
      continue;
    }
    if (s[i] != '[') continue;
    // Find the matching ']' then '(' … ')', mirroring the C scanner.
    std::size_t close = std::string::npos;
    for (std::size_t j = i + 1; j < s.size(); j++) {
      if (s[j] == '\\') {
        j++;
        continue;
      }
      if (s[j] == ']') {
        close = j;
        break;
      }
    }
    if (close == std::string::npos || close + 1 >= s.size() || s[close + 1] != '(') continue;
    int depth = 0;
    std::size_t end = std::string::npos;
    for (std::size_t j = close + 1; j < s.size(); j++) {
      if (s[j] == '(') depth++;
      if (s[j] == ')' && --depth == 0) {
        end = j;
        break;
      }
    }
    if (end == std::string::npos) continue;
    auto target = s.substr(close + 2, end - close - 2);
    auto begin = target.find_first_not_of(" \t");
    auto stop = target.find_last_not_of(" \t");
    target = begin == std::string::npos ? "" : target.substr(begin, stop - begin + 1);
    out.push_back(Link{inline_text(s.substr(i + 1, close - i - 1)), target, line});
    i = end;
  }
  return out;
}

inline bool iequal(const std::string& a, const char* b) {
  std::size_t n = std::strlen(b);
  if (a.size() != n) return false;
  for (std::size_t i = 0; i < n; i++) {
    if (std::tolower(static_cast<unsigned char>(a[i])) !=
        std::tolower(static_cast<unsigned char>(b[i]))) {
      return false;
    }
  }
  return true;
}

}  // namespace detail

inline Extraction extract(const Document& doc) {
  Extraction e;
  e.version = doc.version;
  e.metadata = doc.metadata();
  std::vector<std::string> prose;

  for (const auto& n : doc.nodes) {
    switch (n.kind) {
      case Kind::Heading: {
        auto text = inline_text(n.text);
        e.outline.push_back(Outline{n.level, text, n.line});
        prose.push_back(text);
        auto ls = detail::links_in(n.text, n.line);
        e.links.insert(e.links.end(), ls.begin(), ls.end());
        break;
      }
      case Kind::Paragraph:
      case Kind::Quote: {
        prose.push_back(inline_text(n.text));
        auto ls = detail::links_in(n.text, n.line);
        e.links.insert(e.links.end(), ls.begin(), ls.end());
        break;
      }
      case Kind::List:
        for (const auto& it : n.items) {
          prose.push_back(inline_text(it.text));
          auto ls = detail::links_in(it.text, n.line);
          e.links.insert(e.links.end(), ls.begin(), ls.end());
          if (it.checked.has_value()) {
            Task t;
            t.title = inline_text(it.text);
            t.done = *it.checked;
            t.line = n.line;
            e.tasks.push_back(std::move(t));
          }
        }
        break;
      case Kind::Directive:
      case Kind::Block: {
        if (n.name == "comment" || n.name == "metadata") break;
        if (n.name == "image" || n.name == "video" || n.name == "audio" ||
            n.name == "attachment") {
          e.media.push_back(Media{n.name, n.resolve("src"), inline_text(n.arg("caption")), n.line});
          if (!n.arg("caption").empty()) prose.push_back(inline_text(n.arg("caption")));
          break;
        }
        if (n.name == "code") {
          e.code.push_back(Code{n.resolve("language"),
                                static_cast<std::size_t>(
                                    std::count(n.text.begin(), n.text.end(), '\n')) + 1,
                                n.line, n.text});
          break;
        }
        if (n.name == "table") {
          auto t = parse_table(n);
          auto join = [](const std::vector<std::string>& cells) {
            std::string out;
            for (std::size_t i = 0; i < cells.size(); i++) {
              if (i) out += " | ";
              out += cells[i];
            }
            return out;
          };
          prose.push_back(join(t.header));
          for (const auto& row : t.rows) prose.push_back(join(row));
          break;
        }
        if (detail::is_presentational(n.name)) {
          if (n.kind == Kind::Block && !n.text.empty()) prose.push_back(n.text);
          break;
        }

        auto f = n.fields();
        Block b;
        b.type = n.name;
        b.line = n.line;
        b.text = n.text;
        for (std::size_t i = 0; i < n.args.size(); i++) {
          const auto& a = n.args[i];
          b.args.emplace(a.key.empty() ? std::to_string(i) : a.key, a.value);
        }
        if (!f.empty()) {
          b.fields = f.map();
          for (const auto& x : f) b.order.push_back(x.key);
        }
        e.blocks.push_back(std::move(b));

        if (n.name == "task") {
          auto m = f.map();
          Task t;
          auto at = [&m](const char* k) {
            auto it = m.find(k);
            return it == m.end() ? std::string() : it->second;
          };
          t.title = at("title");
          if (t.title.empty()) t.title = at("");
          t.status = at("status");
          t.owner = at("owner");
          t.due = at("due");
          t.done = detail::iequal(t.status, "done") || detail::iequal(t.status, "complete");
          t.line = n.line;
          e.tasks.push_back(std::move(t));
        }
        if (n.kind == Kind::Block && !n.text.empty()) prose.push_back(n.text);
        break;
      }
    }
  }

  for (std::size_t i = 0; i < prose.size(); i++) {
    if (i) e.text += "\n\n";
    e.text += prose[i];
  }
  bool in_word = false;
  for (char c : e.text) {
    bool space = c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\v' || c == '\f';
    if (space) {
      in_word = false;
    } else if (!in_word) {
      in_word = true;
      e.words++;
    }
  }
  return e;
}

// ---------------------------------------------------------------------------
// HTML
// ---------------------------------------------------------------------------

namespace detail {

inline std::string escape(std::string_view s) {
  std::string out;
  out.reserve(s.size());
  for (char c : s) {
    switch (c) {
      case '&': out += "&amp;"; break;
      case '<': out += "&lt;"; break;
      case '>': out += "&gt;"; break;
      case '"': out += "&#34;"; break;
      case '\'': out += "&#39;"; break;
      default: out += c;
    }
  }
  return out;
}

}  // namespace detail

/// Render a document to an HTML fragment.
inline std::string render_html(const Document& doc) {
  std::vector<std::string> body;
  std::vector<const Node*> notes;

  for (const auto& n : doc.nodes) {
    switch (n.kind) {
      case Kind::Heading:
        body.push_back("<h" + std::to_string(n.level) + ">" + inline_html(n.text) + "</h" +
                       std::to_string(n.level) + ">");
        break;
      case Kind::Paragraph:
        body.push_back("<p>" + inline_html(n.text) + "</p>");
        break;
      case Kind::Quote:
        body.push_back("<blockquote><p>" + inline_html(n.text) + "</p></blockquote>");
        break;
      case Kind::List: {
        if (n.items.empty()) break;
        std::string tag = n.items[0].ordered ? "ol" : "ul";
        std::string cls = n.items[0].checked.has_value() ? " class=\"checklist\"" : "";
        std::string out = "<" + tag + cls + ">";
        for (const auto& it : n.items) {
          std::string box;
          if (it.checked.has_value()) {
            box = std::string("<input type=\"checkbox\" disabled") + (*it.checked ? " checked" : "") +
                  "> ";
          }
          out += "<li>" + box + inline_html(it.text) + "</li>";
        }
        body.push_back(out + "</" + tag + ">");
        break;
      }
      case Kind::Directive:
      case Kind::Block: {
        if (n.name == "footnote") {
          notes.push_back(&n);
          break;
        }
        if (n.name == "comment" || n.name == "metadata") break;
        if (n.name == "hr") {
          body.push_back("<hr>");
          break;
        }
        auto caption = n.arg("caption");
        std::string figcap =
            caption.empty() ? "" : "<figcaption>" + inline_html(caption) + "</figcaption>";
        if (n.name == "image") {
          auto alt = n.arg("alt").empty() ? inline_text(caption) : n.arg("alt");
          std::string attrs;
          for (const char* k : {"width", "height"}) {
            if (!n.arg(k).empty()) {
              attrs += std::string(" ") + k + "=\"" + detail::escape(n.arg(k)) + "\"";
            }
          }
          body.push_back("<figure><img src=\"" + detail::escape(n.resolve("src")) + "\" alt=\"" +
                         detail::escape(alt) + "\"" + attrs + ">" + figcap + "</figure>");
          break;
        }
        if (n.name == "video" || n.name == "audio") {
          std::string extra = n.name == "video" ? " controls playsinline" : " controls";
          body.push_back("<figure><" + n.name + " src=\"" + detail::escape(n.resolve("src")) +
                         "\"" + extra + "></" + n.name + ">" + figcap + "</figure>");
          break;
        }
        if (n.name == "attachment") {
          auto src = n.resolve("src");
          auto label = n.arg("name").empty() ? src : n.arg("name");
          body.push_back("<p class=\"attachment\"><a href=\"" + detail::escape(src) +
                         "\" download>" + detail::escape(label) + "</a></p>");
          break;
        }
        if (n.name == "code") {
          auto lang = n.resolve("language");
          std::string cls = lang.empty() ? "" : " class=\"language-" + detail::escape(lang) + "\"";
          body.push_back("<pre><code" + cls + ">" + detail::escape(n.text) + "</code></pre>");
          break;
        }
        if (n.name == "math") {
          body.push_back("<div class=\"math\">" + detail::escape(n.text) + "</div>");
          break;
        }
        if (n.name == "mermaid") {
          body.push_back("<pre class=\"mermaid\">" + detail::escape(n.text) + "</pre>");
          break;
        }
        if (n.name == "raw") {
          body.push_back(n.resolve("format") == "html" ? n.text
                                                       : "<pre>" + detail::escape(n.text) + "</pre>");
          break;
        }
        if (n.name == "table") {
          auto t = parse_table(n);
          if (t.header.empty()) break;
          auto style = [&t](std::size_t i) {
            return (i < t.align.size() && t.align[i] != "left")
                       ? " style=\"text-align:" + t.align[i] + "\""
                       : std::string();
          };
          std::string out = "<table>\n<thead><tr>";
          for (std::size_t i = 0; i < t.header.size(); i++) {
            out += "<th" + style(i) + ">" + inline_html(t.header[i]) + "</th>";
          }
          out += "</tr></thead>\n<tbody>";
          for (const auto& row : t.rows) {
            out += "<tr>";
            for (std::size_t i = 0; i < row.size(); i++) {
              out += "<td" + style(i) + ">" + inline_html(row[i]) + "</td>";
            }
            out += "</tr>";
          }
          body.push_back(out + "</tbody>\n</table>");
          break;
        }

        if (n.kind == Kind::Block) {
          auto f = n.fields();
          if (!f.empty()) {
            std::string rows;
            for (const auto& x : f) {
              rows += "<dt>" + detail::escape(x.key.empty() ? "—" : x.key) + "</dt><dd>" +
                      inline_html(x.value) + "</dd>";
            }
            body.push_back("<section class=\"record\" data-type=\"" + detail::escape(n.name) +
                           "\"><h4 class=\"record-type\">" + detail::escape(n.name) + "</h4><dl>" +
                           rows + "</dl></section>");
            break;
          }
        }
        body.push_back("<div class=\"unknown\" data-directive=\"" + detail::escape(n.name) +
                       "\">@" + detail::escape(n.name) + "</div>");
        break;
      }
    }
  }

  if (!notes.empty()) {
    std::string items;
    for (std::size_t i = 0; i < notes.size(); i++) {
      auto id = notes[i]->resolve("id");
      if (id.empty()) id = std::to_string(i + 1);
      auto e = detail::escape(id);
      items += "<li id=\"fn-" + e + "\">" + inline_html(notes[i]->text) +
               " <a class=\"fnback\" href=\"#fnref-" + e + "\">&#8617;</a></li>";
    }
    body.push_back("<section class=\"footnotes\"><ol>" + items + "</ol></section>");
  }

  std::string out;
  for (const auto& part : body) {
    if (part.empty()) continue;
    if (!out.empty()) out += "\n";
    out += part;
  }
  return out;
}

}  // namespace xtxt

#endif  // XTXT_HPP
