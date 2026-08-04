package io.github.saimouli3.xtxt;

import java.util.ArrayList;
import java.util.Arrays;
import java.util.HashMap;
import java.util.HashSet;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.Set;

/**
 * XTXT — a plain-text document format with structure.
 *
 * <p>A dependency-free implementation of the format defined in SPEC.md. This is
 * a port of the Go reference implementation, not a binding: it agrees with it
 * because both are checked against the same conformance suite.
 *
 * <pre>{@code
 * var res = Xtxt.parse("# Hello\n\nworld\n");
 * res.doc.nodes.forEach(n -> System.out.println(n.kind));
 * }</pre>
 *
 * @see <a href="https://github.com/SaiMouli3/xtxt/blob/main/SPEC.md">SPEC.md</a>
 */
public final class Xtxt {

  private Xtxt() {}

  // -------------------------------------------------------------------------
  // Model
  // -------------------------------------------------------------------------

  /** The type of a {@link Node}. */
  public enum Kind {
    HEADING("heading"),
    PARAGRAPH("paragraph"),
    QUOTE("quote"),
    LIST("list"),
    /** Inline form: {@code @name(args)}. */
    DIRECTIVE("directive"),
    /** Fenced form: {@code @name … @endname}. */
    BLOCK("block");

    private final String wire;

    Kind(String wire) {
      this.wire = wire;
    }

    /** The name this kind carries in the conformance suite and in JSON. */
    public String wire() {
      return wire;
    }
  }

  /** How serious a diagnostic is. */
  public enum Severity {
    ERROR("error"),
    WARNING("warning");

    private final String wire;

    Severity(String wire) {
      this.wire = wire;
    }

    public String wire() {
      return wire;
    }
  }

  /** One argument of a directive. {@code key} is empty for a positional one. */
  public static final class Arg {
    public final String key;
    public final String value;

    public Arg(String key, String value) {
      this.key = key;
      this.value = value;
    }
  }

  /** A directive's arguments, in source order. */
  public static final class Args {
    public final List<Arg> list;

    public Args(List<Arg> list) {
      this.list = list;
    }

    public static Args empty() {
      return new Args(List.of());
    }

    public String get(String key) {
      for (Arg a : list) {
        if (a.key.equals(key)) return a.value;
      }
      return "";
    }

    public boolean has(String key) {
      return list.stream().anyMatch(a -> a.key.equals(key));
    }

    public String positional(int i) {
      int n = 0;
      for (Arg a : list) {
        if (a.key.isEmpty()) {
          if (n == i) return a.value;
          n++;
        }
      }
      return "";
    }

    /**
     * The named argument, falling back to the first positional one. This is what
     * makes {@code @video("x.mp4")} and {@code @video(src="x.mp4")} equivalent.
     */
    public String resolve(String key) {
      String named = get(key);
      return named.isEmpty() ? positional(0) : named;
    }

    public boolean isEmpty() {
      return list.isEmpty();
    }

    public int size() {
      return list.size();
    }
  }

  /** One entry in a list. {@code checked} is null unless it is a checklist item. */
  public static final class Item {
    public String text = "";
    public boolean ordered;
    public Boolean checked;
  }

  /** A single block in the document tree. */
  public static final class Node {
    public final Kind kind;
    public String name = "";
    public int level;
    public String text = "";
    public Args args = Args.empty();
    public List<Item> items = new ArrayList<>();
    /** 1-based line where the node starts. */
    public final int line;

    Node(Kind kind, int line) {
      this.kind = kind;
      this.line = line;
    }

    /** The payload read as an ordered {@code Key: value} record. */
    public Fields fields() {
      return kind == Kind.BLOCK ? parseFields(text) : new Fields(List.of());
    }
  }

  /** A parsed document. */
  public static final class Document {
    public String version = "";
    public final List<Node> nodes = new ArrayList<>();

    /** The {@code @metadata} block as key/value pairs, keys lowercased. */
    public Map<String, String> metadata() {
      for (Node n : nodes) {
        if (n.kind == Kind.BLOCK && n.name.equals("metadata")) return parseMetadata(n.text);
      }
      return new LinkedHashMap<>();
    }
  }

  /** A diagnostic tied to a line of source. */
  public static final class Issue {
    public final Severity severity;
    public final int line;
    public final String message;

    public Issue(Severity severity, int line, String message) {
      this.severity = severity;
      this.line = line;
      this.message = message;
    }
  }

  /**
   * The outcome of parsing. A document is always returned, even when {@code
   * issues} contains errors — recovery is part of the compatibility guarantee.
   */
  public static final class ParseResult {
    public final Document doc;
    public final List<Issue> issues;

    ParseResult(Document doc, List<Issue> issues) {
      this.doc = doc;
      this.issues = issues;
    }

    public boolean hasErrors() {
      return issues.stream().anyMatch(i -> i.severity == Severity.ERROR);
    }
  }

  // -------------------------------------------------------------------------
  // Parser
  // -------------------------------------------------------------------------

  /**
   * Directives that are blocks even when the closing fence is missing; used only
   * to report a helpful error.
   */
  private static final Set<String> FENCED_BY_DEFAULT =
      Set.of("code", "table", "math", "mermaid", "metadata", "comment", "raw");

  private static boolean isNameStart(char c) {
    return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z');
  }

  private static boolean isNameChar(char c) {
    return isNameStart(c) || c == '-' || (c >= '0' && c <= '9');
  }

  private static boolean isBlank(String s) {
    return s.trim().isEmpty();
  }

  private static String trimLeadingSpace(String s) {
    int i = 0;
    while (i < s.length() && (s.charAt(i) == ' ' || s.charAt(i) == '\t')) i++;
    return s.substring(i);
  }

  private static String trimTrailingSpace(String s) {
    int i = s.length();
    while (i > 0 && (s.charAt(i - 1) == ' ' || s.charAt(i - 1) == '\t')) i--;
    return s.substring(0, i);
  }

  private static List<String> readLines(String src) {
    List<String> lines = new ArrayList<>(Arrays.asList(src.replace("\r\n", "\n").split("\n", -1)));
    if (!lines.isEmpty() && lines.get(lines.size() - 1).isEmpty()) lines.remove(lines.size() - 1);
    if (!lines.isEmpty() && lines.get(0).startsWith("﻿")) lines.set(0, lines.get(0).substring(1));
    return lines;
  }

  /** 1-6 for a heading line, else 0. */
  private static int headingLevel(String s) {
    String t = trimLeadingSpace(s);
    int n = 0;
    while (n < t.length() && t.charAt(n) == '#') n++;
    if (n == 0 || n > 6 || n >= t.length() || t.charAt(n) != ' ') return 0;
    return n;
  }

  private static boolean isDirective(String s) {
    String t = trimLeadingSpace(s);
    return t.length() >= 2 && t.charAt(0) == '@' && isNameStart(t.charAt(1));
  }

  /** Returns {name, rest-of-line}. */
  private static String[] directiveName(String s) {
    String t = trimLeadingSpace(s);
    int i = 1;
    while (i < t.length() && isNameChar(t.charAt(i))) i++;
    return new String[] {t.substring(1, i), t.substring(i)};
  }

  /** The bullet or number prefix of a list item, or "". */
  private static String itemPrefix(String s) {
    String t = trimLeadingSpace(s);
    if (t.length() >= 2 && (t.charAt(0) == '-' || t.charAt(0) == '*') && t.charAt(1) == ' ') {
      return t.substring(0, 2);
    }
    int i = 0;
    while (i < t.length() && Character.isDigit(t.charAt(i))) i++;
    if (i > 0 && i + 1 < t.length() && t.charAt(i) == '.' && t.charAt(i + 1) == ' ') {
      return t.substring(0, i + 2);
    }
    return "";
  }

  /** Parse an XTXT document. Parsing never throws: bad input becomes a diagnostic. */
  public static ParseResult parse(String src) {
    return new Parser(readLines(src)).run();
  }

  private static final class Parser {
    private final List<String> lines;
    private int i;
    private final Document doc = new Document();
    private final List<Issue> issues = new ArrayList<>();

    Parser(List<String> lines) {
      this.lines = lines;
    }

    private void err(int line, String message) {
      issues.add(new Issue(Severity.ERROR, line, message));
    }

    private void warn(int line, String message) {
      issues.add(new Issue(Severity.WARNING, line, message));
    }

    ParseResult run() {
      while (i < lines.size()) {
        String line = lines.get(i);
        if (isBlank(line)) {
          i++;
        } else if (isDirective(line)) {
          directive();
        } else if (headingLevel(line) > 0) {
          int lvl = headingLevel(line);
          Node n = new Node(Kind.HEADING, i + 1);
          n.level = lvl;
          n.text = trimLeadingSpace(line).substring(lvl).trim();
          doc.nodes.add(n);
          i++;
        } else if (line.trim().startsWith(">")) {
          quote();
        } else if (!itemPrefix(line).isEmpty()) {
          list();
        } else {
          paragraph();
        }
      }
      return new ParseResult(doc, issues);
    }

    private void directive() {
      int start = i;
      String[] parts = directiveName(lines.get(i));
      String name = parts[0];

      Args args = parseArgs(parts[1]);
      if (args == null) {
        err(start + 1, "unclosed argument list for @" + name);
        i = start + 1;
        return;
      }

      if (name.startsWith("end")) {
        err(start + 1, "@" + name + " has no matching opening fence");
        i++;
        return;
      }

      if (name.equals("xtxt") && doc.nodes.isEmpty()) {
        doc.version = args.resolve("version");
        i++;
        return;
      }

      // A directive is fenced if a matching @end<name> line follows, which keeps
      // the rule local: no registry of block names to keep in sync.
      int end = findFence(name, i + 1);
      if (end >= 0) {
        List<String> body = new ArrayList<>(lines.subList(i + 1, end));
        i = end + 1;
        Node n = new Node(Kind.BLOCK, start + 1);
        n.name = name;
        n.args = args;
        n.text = trimFenceBody(body);
        doc.nodes.add(n);
        return;
      }

      if (FENCED_BY_DEFAULT.contains(name)) {
        err(start + 1, "unclosed @" + name + " block: no matching @end" + name);
      }
      i++;
      Node n = new Node(Kind.DIRECTIVE, start + 1);
      n.name = name;
      n.args = args;
      doc.nodes.add(n);
    }

    private int findFence(String name, int from) {
      String closer = "@end" + name;
      for (int j = from; j < lines.size(); j++) {
        if (trimTrailingSpace(lines.get(j)).equals(closer)) return j;
      }
      return -1;
    }

    /**
     * Reads an argument list, consuming further lines if it spans them. Advances
     * {@code i} to the last consumed line only on success; returns null if the
     * list never closes.
     */
    private Args parseArgs(String rest) {
      String trimmed = rest.trim();
      if (!trimmed.startsWith("(")) return Args.empty();
      StringBuilder buf = new StringBuilder(trimmed);
      int line = i;
      while (true) {
        String inner = balanced(buf.toString());
        if (inner != null) {
          i = line;
          return splitArgs(inner);
        }
        line++;
        if (line >= lines.size()) return null;
        buf.append('\n').append(lines.get(line));
      }
    }

    private void quote() {
      int start = i;
      List<String> parts = new ArrayList<>();
      while (i < lines.size()) {
        String t = lines.get(i).trim();
        if (!t.startsWith(">")) break;
        parts.add(t.substring(1).trim());
        i++;
      }
      Node n = new Node(Kind.QUOTE, start + 1);
      n.text = String.join(" ", parts);
      doc.nodes.add(n);
    }

    private void list() {
      int start = i;
      List<Item> items = new ArrayList<>();
      int baseIndent = -1;
      boolean flagged = false;
      while (i < lines.size()) {
        String pre = itemPrefix(lines.get(i));
        if (pre.isEmpty()) break;
        // Lists do not nest (SPEC 3.4), so an item indented deeper than the
        // first is structure about to be lost. Flattening it silently turns a
        // formatting mistake into data loss; uniform indentation is style.
        String raw = lines.get(i);
        int indent = raw.length() - trimLeadingSpace(raw).length();
        if (baseIndent < 0) {
          baseIndent = indent;
        } else if (indent > baseIndent && !flagged) {
          warn(i + 1, "list item is indented deeper than the first: "
              + "XTXT lists do not nest, so it is flattened");
          flagged = true;
        }
        String body = trimLeadingSpace(lines.get(i)).substring(pre.length()).trim();
        Item item = new Item();
        item.ordered = Character.isDigit(pre.charAt(0));
        if (body.length() >= 3 && body.charAt(0) == '[' && body.charAt(2) == ']') {
          char c = body.charAt(1);
          if (c == ' ' || c == 'x' || c == 'X') {
            item.checked = c != ' ';
            body = body.substring(3).trim();
          }
        }
        item.text = body;
        items.add(item);
        i++;
      }
      Node n = new Node(Kind.LIST, start + 1);
      n.items = items;
      doc.nodes.add(n);
    }

    private void paragraph() {
      int start = i;
      List<String> parts = new ArrayList<>();
      while (i < lines.size()) {
        String line = lines.get(i);
        if (isBlank(line)
            || headingLevel(line) > 0
            || isDirective(line)
            || !itemPrefix(line).isEmpty()
            || line.trim().startsWith(">")) {
          break;
        }
        String t = line.trim();
        if (t.startsWith("\\@")) t = t.substring(1);
        parts.add(t);
        i++;
      }
      Node n = new Node(Kind.PARAGRAPH, start + 1);
      n.text = String.join(" ", parts);
      doc.nodes.add(n);
    }
  }

  /** The text inside the outermost parens, or null if they never close. */
  private static String balanced(String s) {
    int depth = 0;
    boolean inQuote = false, esc = false;
    for (int i = 0; i < s.length(); i++) {
      char c = s.charAt(i);
      if (esc) {
        esc = false;
      } else if (c == '\\') {
        esc = true;
      } else if (inQuote) {
        if (c == '"') inQuote = false;
      } else if (c == '"') {
        inQuote = true;
      } else if (c == '(') {
        depth++;
      } else if (c == ')') {
        depth--;
        if (depth == 0) return s.substring(1, i);
      }
    }
    return null;
  }

  /** Splits on {@code sep}, ignoring separators inside quotes or nested parens. */
  private static List<String> splitTop(String s, char sep) {
    List<String> out = new ArrayList<>();
    int depth = 0, start = 0;
    boolean inQuote = false, esc = false;
    for (int i = 0; i < s.length(); i++) {
      char c = s.charAt(i);
      if (esc) {
        esc = false;
      } else if (c == '\\') {
        esc = true;
      } else if (inQuote) {
        if (c == '"') inQuote = false;
      } else if (c == '"') {
        inQuote = true;
      } else if (c == '(') {
        depth++;
      } else if (c == ')') {
        depth--;
      } else if (c == sep && depth == 0) {
        out.add(s.substring(start, i));
        start = i + 1;
      }
    }
    out.add(s.substring(start));
    return out;
  }

  private static boolean isName(String s) {
    s = s.trim();
    if (s.isEmpty() || !isNameStart(s.charAt(0))) return false;
    for (int i = 1; i < s.length(); i++) {
      if (!isNameChar(s.charAt(i))) return false;
    }
    return true;
  }

  private static Args splitArgs(String s) {
    List<Arg> args = new ArrayList<>();
    for (String raw : splitTop(s, ',')) {
      String field = raw.trim();
      if (field.isEmpty()) continue;
      List<String> parts = splitTop(field, '=');
      String key = "", val = field;
      if (parts.size() >= 2 && isName(parts.get(0))) {
        key = parts.get(0).trim();
        val = field.substring(parts.get(0).length() + 1).trim();
      }
      args.add(new Arg(key, unquote(val)));
    }
    return new Args(args);
  }

  private static String unquote(String s) {
    s = s.trim();
    if (s.length() >= 2 && s.startsWith("\"") && s.endsWith("\"")) {
      return unescape(s.substring(1, s.length() - 1));
    }
    return s;
  }

  private static String unescape(String s) {
    if (s.indexOf('\\') < 0) return s;
    StringBuilder out = new StringBuilder(s.length());
    for (int i = 0; i < s.length(); i++) {
      if (s.charAt(i) == '\\' && i + 1 < s.length()) {
        char c = s.charAt(++i);
        out.append(c == 'n' ? '\n' : c == 't' ? '\t' : c);
      } else {
        out.append(s.charAt(i));
      }
    }
    return out.toString();
  }

  private static String trimFenceBody(List<String> body) {
    if (!body.isEmpty() && isBlank(body.get(0))) body.remove(0);
    if (!body.isEmpty() && isBlank(body.get(body.size() - 1))) body.remove(body.size() - 1);
    List<String> out = new ArrayList<>(body.size());
    for (String l : body) {
      out.add(trimLeadingSpace(l).startsWith("\\@end") ? l.replaceFirst("\\\\@end", "@end") : l);
    }
    return String.join("\n", out);
  }

  private static Map<String, String> parseMetadata(String payload) {
    Map<String, String> out = new LinkedHashMap<>();
    for (String line : payload.split("\n", -1)) {
      if (isBlank(line)) continue;
      int eq = line.indexOf('=');
      if (eq < 0) continue;
      out.putIfAbsent(line.substring(0, eq).trim().toLowerCase(Locale.ROOT), line.substring(eq + 1).trim());
    }
    return out;
  }

  // -------------------------------------------------------------------------
  // Tables
  // -------------------------------------------------------------------------

  /** The interpreted payload of an {@code @table} block. */
  public static final class Table {
    public List<String> header = new ArrayList<>();
    public List<List<String>> rows = new ArrayList<>();
    /** "left", "right" or "center" per column. */
    public List<String> align = new ArrayList<>();
  }

  private static List<String> splitCells(String line) {
    String t = line.trim();
    if (t.startsWith("|")) t = t.substring(1);
    if (t.endsWith("|")) t = t.substring(0, t.length() - 1);
    List<String> out = new ArrayList<>();
    for (String c : t.split("\\|", -1)) out.add(c.trim());
    return out;
  }

  private static boolean isSeparatorRow(List<String> cells) {
    if (cells.isEmpty()) return false;
    for (String c : cells) {
      if (c.isEmpty()) return false;
      for (int i = 0; i < c.length(); i++) {
        char ch = c.charAt(i);
        if (ch != '-' && ch != ':') return false;
      }
    }
    return true;
  }

  public static Table parseTable(Node n) {
    Table t = new Table();
    List<List<String>> rows = new ArrayList<>();
    int sepAt = -1;
    for (String line : n.text.split("\n", -1)) {
      if (isBlank(line)) continue;
      List<String> cells = splitCells(line);
      if (sepAt < 0 && isSeparatorRow(cells)) {
        sepAt = rows.size();
        for (String c : cells) {
          boolean left = c.startsWith(":"), right = c.endsWith(":");
          t.align.add(left && right ? "center" : right ? "right" : "left");
        }
        continue;
      }
      rows.add(cells);
    }
    if (rows.isEmpty()) return t;
    int sep = sepAt > 0 ? sepAt : 1;
    t.header = sep > 1 ? rows.get(sep - 1) : rows.get(0);
    t.rows = new ArrayList<>(rows.subList(Math.min(sep, rows.size()), rows.size()));
    return t;
  }

  // -------------------------------------------------------------------------
  // Records
  // -------------------------------------------------------------------------

  /** One {@code Key: value} entry in a block payload. */
  public static final class Field {
    public final String key;
    public String value;

    Field(String key, String value) {
      this.key = key;
      this.value = value;
    }
  }

  /**
   * A block payload read as an ordered record. Order matters: a {@code @chat}
   * block's turns are fields, and their sequence is the conversation.
   */
  public static final class Fields {
    public final List<Field> list;

    Fields(List<Field> list) {
      this.list = list;
    }

    public String get(String key) {
      for (Field f : list) {
        if (f.key.equalsIgnoreCase(key)) return f.value;
      }
      return "";
    }

    /** Flatten to a lowercase-keyed map, keeping the first of any duplicate. */
    public Map<String, String> map() {
      Map<String, String> out = new LinkedHashMap<>();
      for (Field f : list) out.putIfAbsent(f.key.toLowerCase(Locale.ROOT), f.value);
      return out;
    }

    public boolean isEmpty() {
      return list.isEmpty();
    }

    public int size() {
      return list.size();
    }
  }

  /**
   * A field key is a label, not a sentence: these caps are what keep ordinary
   * prose containing a colon from being read as a record field.
   */
  private static final int MAX_FIELD_KEY_LEN = 32;

  private static final int MAX_FIELD_KEY_WORDS = 3;

  /** Returns {key, value} if the line opens a field, else null. */
  private static String[] isFieldLine(String line) {
    for (int i = 0; i < line.length() && i <= MAX_FIELD_KEY_LEN; i++) {
      char c = line.charAt(i);
      if (c == ':' || c == '=') {
        String k = line.substring(0, i).trim();
        if (k.isEmpty()
            || !isNameStart(k.charAt(0))
            || k.trim().split("\\s+").length > MAX_FIELD_KEY_WORDS) {
          return null;
        }
        return new String[] {k, line.substring(i + 1).trim()};
      }
      if (!(c == ' ' || c == '_' || c == '-' || c == '.' || isNameChar(c))) return null;
    }
    return null;
  }

  /**
   * Interpret a block payload as an ordered record. Lines before the first field
   * become the empty key, so nothing is lost.
   */
  public static Fields parseFields(String payload) {
    List<Field> out = new ArrayList<>();
    List<String> preamble = new ArrayList<>();
    Field cur = null;

    for (String line : payload.split("\n", -1)) {
      String[] kv = isFieldLine(line);
      if (kv != null) {
        cur = new Field(kv[0], kv[1]);
        out.add(cur);
        continue;
      }
      if (cur == null) {
        preamble.add(line);
      } else if (cur.value.isEmpty()) {
        cur.value = line.trim();
      } else {
        cur.value = cur.value + "\n" + line;
      }
    }
    for (Field f : out) f.value = f.value.trim();
    String text = String.join("\n", preamble).trim();
    if (!text.isEmpty()) out.add(0, new Field("", text));
    return new Fields(out);
  }

  // -------------------------------------------------------------------------
  // Inline formatting
  // -------------------------------------------------------------------------

  /** Returns {id, endIndex} for a marker {@code [^id]} at s[i], else null. */
  private static Object[] footnoteRef(String s, int i) {
    if (i + 2 >= s.length() || s.charAt(i + 1) != '^') return null;
    int close = s.indexOf(']', i + 2);
    if (close < 0 || close == i + 2) return null;
    String id = s.substring(i + 2, close);
    if (id.indexOf(' ') >= 0 || id.indexOf('\t') >= 0) return null;
    return new Object[] {id, close};
  }

  /** The index of the next unescaped {@code mark} at or after {@code from}. */
  private static int findClose(String s, int from, String mark) {
    for (int i = from; i + mark.length() <= s.length(); i++) {
      if (s.charAt(i) == '\\') {
        i++;
        continue;
      }
      if (s.startsWith(mark, i)) return i == from ? -1 : i;
    }
    return -1;
  }

  /** Returns {label, target, endIndex} for {@code [label](target)}, else null. */
  private static Object[] parseLink(String s, int i) {
    int close = findClose(s, i + 1, "]");
    if (close < 0 || close + 1 >= s.length() || s.charAt(close + 1) != '(') return null;
    String inner = balanced(s.substring(close + 1));
    if (inner == null) return null;
    return new Object[] {s.substring(i + 1, close), inner.trim(), close + 1 + inner.length() + 1};
  }

  private static void escapeChar(StringBuilder out, char c) {
    switch (c) {
      case '&' -> out.append("&amp;");
      case '<' -> out.append("&lt;");
      case '>' -> out.append("&gt;");
      case '"' -> out.append("&#34;");
      case '\'' -> out.append("&#39;");
      default -> out.append(c);
    }
  }

  private static String escape(String s) {
    StringBuilder out = new StringBuilder(s.length());
    for (int i = 0; i < s.length(); i++) escapeChar(out, s.charAt(i));
    return out.toString();
  }

  /** Convert inline markup to HTML, escaping everything else. */
  public static String inlineHtml(String s) {
    StringBuilder out = new StringBuilder(s.length());
    for (int i = 0; i < s.length(); i++) {
      char c = s.charAt(i);
      if (c == '\\' && i + 1 < s.length()) {
        escapeChar(out, s.charAt(++i));
      } else if (c == '`') {
        int end = s.indexOf('`', i + 1);
        if (end >= 0) {
          out.append("<code>").append(escape(s.substring(i + 1, end))).append("</code>");
          i = end;
        } else {
          out.append("&#96;");
        }
      } else if (c == '*') {
        String mark = s.startsWith("**", i) ? "**" : "*";
        String tag = mark.equals("**") ? "strong" : "em";
        int end = findClose(s, i + mark.length(), mark);
        if (end >= 0) {
          out.append('<').append(tag).append('>')
              .append(inlineHtml(s.substring(i + mark.length(), end)))
              .append("</").append(tag).append('>');
          i = end + mark.length() - 1;
        } else {
          out.append('*');
        }
      } else if (c == '[') {
        Object[] fn = footnoteRef(s, i);
        if (fn != null) {
          String e = escape((String) fn[0]);
          out.append("<sup class=\"fnref\" id=\"fnref-").append(e).append("\"><a href=\"#fn-")
              .append(e).append("\">").append(e).append("</a></sup>");
          i = (int) fn[1];
          continue;
        }
        Object[] lk = parseLink(s, i);
        if (lk != null) {
          out.append("<a href=\"").append(escape((String) lk[1])).append("\">")
              .append(inlineHtml((String) lk[0])).append("</a>");
          i = (int) lk[2];
        } else {
          out.append('[');
        }
      } else {
        escapeChar(out, c);
      }
    }
    return out.toString();
  }

  /** Strip inline markup, for plain text and analysis. */
  public static String inlineText(String s) {
    StringBuilder out = new StringBuilder(s.length());
    for (int i = 0; i < s.length(); i++) {
      char c = s.charAt(i);
      if (c == '\\' && i + 1 < s.length()) {
        out.append(s.charAt(++i));
      } else if (c == '`') {
        int end = s.indexOf('`', i + 1);
        if (end >= 0) {
          out.append(s, i + 1, end);
          i = end;
        } else {
          out.append(c);
        }
      } else if (c == '*') {
        String mark = s.startsWith("**", i) ? "**" : "*";
        int end = findClose(s, i + mark.length(), mark);
        if (end >= 0) {
          out.append(inlineText(s.substring(i + mark.length(), end)));
          i = end + mark.length() - 1;
        } else {
          out.append(c);
        }
      } else if (c == '[') {
        Object[] fn = footnoteRef(s, i);
        if (fn != null) {
          out.append('[').append(fn[0]).append(']');
          i = (int) fn[1];
          continue;
        }
        Object[] lk = parseLink(s, i);
        if (lk != null) {
          out.append(inlineText((String) lk[0]));
          i = (int) lk[2];
        } else {
          out.append(c);
        }
      } else {
        out.append(c);
      }
    }
    return out.toString();
  }

  // -------------------------------------------------------------------------
  // Validation
  // -------------------------------------------------------------------------

  /**
   * The standard directives and whether each is a fenced block. Anything absent
   * is unknown: a warning, never an error, so a 1.0 reader stays usable on a
   * newer document.
   */
  private static final Map<String, Boolean> KNOWN = new HashMap<>();

  static {
    for (String n : List.of("xtxt", "image", "video", "audio", "attachment", "include", "embed", "hr")) {
      KNOWN.put(n, false);
    }
    for (String n :
        List.of("code", "table", "math", "mermaid", "metadata", "comment", "raw", "chart",
            "footnote", "task", "decision", "knowledge", "ai", "prompt", "chat", "note")) {
      KNOWN.put(n, true);
    }
  }

  private static final Set<String> REQUIRES_SRC =
      Set.of("image", "video", "audio", "attachment", "include");

  /** Semantic checks with no plugin-declared directives. */
  public static List<Issue> validate(Document doc) {
    return validate(doc, List.of());
  }

  /**
   * Semantic checks on top of the parser's syntactic ones.
   *
   * @param declared names supplied by plugins; a plugin manifest is a
   *     declaration that the directive exists, so it is not reported as unknown.
   */
  public static List<Issue> validate(Document doc, List<String> declared) {
    List<Issue> issues = new ArrayList<>();
    Set<String> extra = new HashSet<>(declared);
    boolean metadataSeen = false;
    List<Object[]> notes = new ArrayList<>();

    for (Node n : doc.nodes) {
      if (n.kind != Kind.DIRECTIVE && n.kind != Kind.BLOCK) continue;
      Boolean fenced = KNOWN.get(n.name);
      if (fenced == null) {
        if (!extra.contains(n.name)) {
          issues.add(warn(n.line,
              "unknown directive @" + n.name + " (preserved, but this reader cannot render it)"));
        }
        continue;
      }
      if (fenced && n.kind != Kind.BLOCK) {
        issues.add(warn(n.line,
            "@" + n.name + " is a block directive and should be closed with @end" + n.name));
      }
      if (!fenced && n.kind == Kind.BLOCK) {
        issues.add(warn(n.line,
            "@" + n.name + " is not a block directive but was closed with @end" + n.name));
      }
      if (REQUIRES_SRC.contains(n.name) && n.args.resolve("src").isEmpty()) {
        issues.add(warn(n.line, "@" + n.name + " has no src"));
      }

      switch (n.name) {
        case "metadata" -> {
          if (metadataSeen) issues.add(warn(n.line, "duplicate @metadata block"));
          metadataSeen = true;
        }
        case "table" -> {
          Table t = parseTable(n);
          if (t.header.isEmpty()) {
            issues.add(warn(n.line, "@table is empty"));
          } else {
            for (int i = 0; i < t.rows.size(); i++) {
              int len = t.rows.get(i).size();
              if (len != t.header.size()) {
                issues.add(warn(n.line + 1 + i,
                    "table row has " + len + " cells, header has " + t.header.size()));
              }
            }
          }
        }
        case "code" -> {
          if (n.args.resolve("language").isEmpty()) {
            issues.add(warn(n.line, "@code has no language; syntax highlighting will be skipped"));
          }
        }
        case "chart" -> {
          if (chartRows(n) == 0) issues.add(warn(n.line, "@chart has no readable data rows"));
        }
        case "task" -> {
          if (n.fields().get("title").isEmpty()) issues.add(warn(n.line, "@task has no Title field"));
        }
        case "footnote" -> {
          String id = n.args.resolve("id");
          if (id.isEmpty()) {
            issues.add(warn(n.line, "@footnote has no id; references cannot point at it"));
          }
          notes.add(new Object[] {id, n.line});
        }
        default -> {}
      }
    }

    issues.addAll(checkFootnoteRefs(doc, notes));
    return issues;
  }

  private static Issue warn(int line, String message) {
    return new Issue(Severity.WARNING, line, message);
  }

  private static int chartRows(Node n) {
    int count = 0;
    for (String line : n.text.split("\n", -1)) {
      if (isBlank(line)) continue;
      List<String> cells;
      if (line.indexOf('|') >= 0) {
        cells = splitCells(line);
      } else {
        String[] f = line.trim().split("\\s+");
        if (f.length >= 2) {
          cells = List.of(String.join(" ", Arrays.copyOfRange(f, 0, f.length - 1)), f[f.length - 1]);
        } else {
          cells = Arrays.asList(f);
        }
      }
      if (cells.size() >= 2 && !isSeparatorRow(cells)) count++;
    }
    return count;
  }

  private static List<Issue> checkFootnoteRefs(Document doc, List<Object[]> notes) {
    List<Issue> issues = new ArrayList<>();
    Set<String> cited = new HashSet<>();
    Set<String> ids = new HashSet<>();
    for (Object[] note : notes) ids.add((String) note[0]);

    for (Node n : doc.nodes) {
      List<String> texts;
      if (n.kind == Kind.HEADING || n.kind == Kind.PARAGRAPH || n.kind == Kind.QUOTE) {
        texts = List.of(n.text);
      } else if (n.kind == Kind.LIST) {
        texts = n.items.stream().map(it -> it.text).toList();
      } else {
        continue;
      }
      for (String text : texts) {
        for (int i = 0; i < text.length(); i++) {
          if (text.charAt(i) == '\\') {
            i++;
            continue;
          }
          if (text.charAt(i) != '[') continue;
          Object[] fn = footnoteRef(text, i);
          if (fn == null) continue;
          String id = (String) fn[0];
          cited.add(id);
          if (!ids.contains(id)) {
            issues.add(warn(n.line,
                "footnote reference [^" + id + "] has no matching @footnote(id=\"" + id + "\")"));
          }
          i = (int) fn[1];
        }
      }
    }
    for (Object[] note : notes) {
      String id = (String) note[0];
      if (!id.isEmpty() && !cited.contains(id)) {
        issues.add(warn((int) note[1], "@footnote(id=\"" + id + "\") is never referenced"));
      }
    }
    return issues;
  }

  /** Order issues by line for stable reporting. */
  public static List<Issue> sortIssues(List<Issue> issues) {
    List<Issue> out = new ArrayList<>(issues);
    out.sort((a, b) -> Integer.compare(a.line, b.line));
    return out;
  }

  // -------------------------------------------------------------------------
  // Extraction
  // -------------------------------------------------------------------------

  /** One heading in the table of contents. */
  public static final class Outline {
    public final int level;
    public final String text;
    public final int line;

    Outline(int level, String text, int line) {
      this.level = level;
      this.text = text;
      this.line = line;
    }
  }

  /** A structured directive as an agent sees it. */
  public static final class Block {
    public String type = "";
    public int line;
    public Map<String, String> args = new LinkedHashMap<>();
    public Map<String, String> fields = new LinkedHashMap<>();
    /** Field keys in source order. */
    public List<String> order = new ArrayList<>();
    public String text = "";
  }

  public static final class Link {
    public final String text;
    public final String href;
    public final int line;

    Link(String text, String href, int line) {
      this.text = text;
      this.href = href;
      this.line = line;
    }
  }

  public static final class Media {
    public final String kind;
    public final String src;
    public final String caption;
    public final int line;

    Media(String kind, String src, String caption, int line) {
      this.kind = kind;
      this.src = src;
      this.caption = caption;
      this.line = line;
    }
  }

  public static final class Code {
    public final String language;
    public final int lines;
    public final int line;
    public final String source;

    Code(String language, int lines, int line, String source) {
      this.language = language;
      this.lines = lines;
      this.line = line;
      this.source = source;
    }
  }

  /** A unit of work, from either a {@code @task} block or a checklist item. */
  public static final class Task {
    public String title = "";
    public boolean done;
    public String status = "";
    public String owner = "";
    public String due = "";
    public int line;
  }

  /** The machine-facing view of a document. */
  public static final class Extraction {
    public String version = "";
    public Map<String, String> metadata = new LinkedHashMap<>();
    public List<Outline> outline = new ArrayList<>();
    public List<Task> tasks = new ArrayList<>();
    public List<Block> blocks = new ArrayList<>();
    public List<Link> links = new ArrayList<>();
    public List<Media> media = new ArrayList<>();
    public List<Code> code = new ArrayList<>();
    public String text = "";
    public int words;
  }

  private static final Set<String> PRESENTATIONAL =
      Set.of("code", "table", "math", "mermaid", "metadata", "comment", "raw", "image", "video",
          "audio", "attachment", "hr", "xtxt", "include", "embed", "footnote");

  private static List<Link> linksIn(String s, int line) {
    List<Link> out = new ArrayList<>();
    for (int i = 0; i < s.length(); i++) {
      if (s.charAt(i) == '\\') {
        i++;
        continue;
      }
      if (s.charAt(i) != '[') continue;
      Object[] lk = parseLink(s, i);
      if (lk != null) {
        out.add(new Link(inlineText((String) lk[0]), (String) lk[1], line));
        i = (int) lk[2];
      }
    }
    return out;
  }

  /** Everything an agent needs without inferring structure from prose. */
  public static Extraction extract(Document doc) {
    Extraction e = new Extraction();
    e.version = doc.version;
    e.metadata = doc.metadata();
    List<String> prose = new ArrayList<>();

    for (Node n : doc.nodes) {
      switch (n.kind) {
        case HEADING -> {
          String text = inlineText(n.text);
          e.outline.add(new Outline(n.level, text, n.line));
          prose.add(text);
          e.links.addAll(linksIn(n.text, n.line));
        }
        case PARAGRAPH, QUOTE -> {
          prose.add(inlineText(n.text));
          e.links.addAll(linksIn(n.text, n.line));
        }
        case LIST -> {
          for (Item it : n.items) {
            prose.add(inlineText(it.text));
            e.links.addAll(linksIn(it.text, n.line));
            if (it.checked != null) {
              Task t = new Task();
              t.title = inlineText(it.text);
              t.done = it.checked;
              t.line = n.line;
              e.tasks.add(t);
            }
          }
        }
        case DIRECTIVE, BLOCK -> absorb(e, n, prose);
      }
    }

    e.text = String.join("\n\n", prose);
    e.words = e.text.isBlank() ? 0 : e.text.trim().split("\\s+").length;
    return e;
  }

  private static void absorb(Extraction e, Node n, List<String> prose) {
    switch (n.name) {
      case "comment", "metadata" -> {
        return;
      }
      case "image", "video", "audio", "attachment" -> {
        e.media.add(new Media(n.name, n.args.resolve("src"),
            inlineText(n.args.get("caption")), n.line));
        if (!n.args.get("caption").isEmpty()) prose.add(inlineText(n.args.get("caption")));
        return;
      }
      case "code" -> {
        e.code.add(new Code(n.args.resolve("language"),
            n.text.split("\n", -1).length, n.line, n.text));
        return;
      }
      case "table" -> {
        Table t = parseTable(n);
        prose.add(String.join(" | ", t.header));
        for (List<String> row : t.rows) prose.add(String.join(" | ", row));
        return;
      }
      default -> {}
    }

    if (PRESENTATIONAL.contains(n.name)) {
      if (n.kind == Kind.BLOCK && !n.text.isEmpty()) prose.add(n.text);
      return;
    }

    Fields f = n.fields();
    Block block = new Block();
    block.type = n.name;
    block.line = n.line;
    block.text = n.text;
    for (int i = 0; i < n.args.size(); i++) {
      Arg a = n.args.list.get(i);
      block.args.put(a.key.isEmpty() ? String.valueOf(i) : a.key, a.value);
    }
    if (!f.isEmpty()) {
      block.fields = f.map();
      for (Field x : f.list) block.order.add(x.key);
    }
    e.blocks.add(block);

    if (n.name.equals("task")) {
      Map<String, String> m = f.map();
      Task t = new Task();
      t.title = m.getOrDefault("title", "");
      if (t.title.isEmpty()) t.title = m.getOrDefault("", "");
      t.status = m.getOrDefault("status", "");
      t.owner = m.getOrDefault("owner", "");
      t.due = m.getOrDefault("due", "");
      t.done = t.status.equalsIgnoreCase("done") || t.status.equalsIgnoreCase("complete");
      t.line = n.line;
      e.tasks.add(t);
    }
    if (n.kind == Kind.BLOCK && !n.text.isEmpty()) prose.add(n.text);
  }

  // -------------------------------------------------------------------------
  // HTML
  // -------------------------------------------------------------------------

  /** Render a document to an HTML fragment. */
  public static String renderHtml(Document doc) {
    List<String> body = new ArrayList<>();
    List<Node> notes = new ArrayList<>();

    for (Node n : doc.nodes) {
      switch (n.kind) {
        case HEADING -> body.add("<h" + n.level + ">" + inlineHtml(n.text) + "</h" + n.level + ">");
        case PARAGRAPH -> body.add("<p>" + inlineHtml(n.text) + "</p>");
        case QUOTE -> body.add("<blockquote><p>" + inlineHtml(n.text) + "</p></blockquote>");
        case LIST -> body.add(listHtml(n));
        case DIRECTIVE, BLOCK -> {
          if (n.name.equals("footnote")) {
            notes.add(n);
          } else {
            body.add(directiveHtml(n));
          }
        }
      }
    }

    if (!notes.isEmpty()) {
      StringBuilder items = new StringBuilder();
      for (int i = 0; i < notes.size(); i++) {
        Node n = notes.get(i);
        String id = n.args.resolve("id");
        if (id.isEmpty()) id = String.valueOf(i + 1);
        String e = escape(id);
        items.append("<li id=\"fn-").append(e).append("\">").append(inlineHtml(n.text))
            .append(" <a class=\"fnback\" href=\"#fnref-").append(e).append("\">&#8617;</a></li>");
      }
      body.add("<section class=\"footnotes\"><ol>" + items + "</ol></section>");
    }

    body.removeIf(String::isEmpty);
    return String.join("\n", body);
  }

  private static String listHtml(Node n) {
    if (n.items.isEmpty()) return "";
    String tag = n.items.get(0).ordered ? "ol" : "ul";
    String cls = n.items.get(0).checked != null ? " class=\"checklist\"" : "";
    StringBuilder items = new StringBuilder();
    for (Item it : n.items) {
      String box = it.checked == null
          ? ""
          : "<input type=\"checkbox\" disabled" + (it.checked ? " checked" : "") + "> ";
      items.append("<li>").append(box).append(inlineHtml(it.text)).append("</li>");
    }
    return "<" + tag + cls + ">" + items + "</" + tag + ">";
  }

  private static String directiveHtml(Node n) {
    String caption = n.args.get("caption");
    String figcap = caption.isEmpty() ? "" : "<figcaption>" + inlineHtml(caption) + "</figcaption>";

    switch (n.name) {
      case "comment", "metadata":
        return "";
      case "hr":
        return "<hr>";
      case "image": {
        String alt = n.args.get("alt").isEmpty() ? inlineText(caption) : n.args.get("alt");
        StringBuilder attrs = new StringBuilder();
        for (String k : List.of("width", "height")) {
          if (!n.args.get(k).isEmpty()) {
            attrs.append(' ').append(k).append("=\"").append(escape(n.args.get(k))).append('"');
          }
        }
        return "<figure><img src=\"" + escape(n.args.resolve("src")) + "\" alt=\"" + escape(alt)
            + "\"" + attrs + ">" + figcap + "</figure>";
      }
      case "video", "audio": {
        String extra = n.name.equals("video") ? " controls playsinline" : " controls";
        return "<figure><" + n.name + " src=\"" + escape(n.args.resolve("src")) + "\"" + extra
            + "></" + n.name + ">" + figcap + "</figure>";
      }
      case "attachment": {
        String src = n.args.resolve("src");
        String name = n.args.get("name").isEmpty() ? src : n.args.get("name");
        return "<p class=\"attachment\"><a href=\"" + escape(src) + "\" download>" + escape(name)
            + "</a></p>";
      }
      case "code": {
        String lang = n.args.resolve("language");
        String cls = lang.isEmpty() ? "" : " class=\"language-" + escape(lang) + "\"";
        return "<pre><code" + cls + ">" + escape(n.text) + "</code></pre>";
      }
      case "math":
        return "<div class=\"math\">" + escape(n.text) + "</div>";
      case "mermaid":
        return "<pre class=\"mermaid\">" + escape(n.text) + "</pre>";
      case "raw":
        return n.args.resolve("format").equals("html") ? n.text : "<pre>" + escape(n.text) + "</pre>";
      case "table":
        return tableHtml(n);
      default: {
        if (n.kind == Kind.BLOCK) {
          Fields f = n.fields();
          if (!f.isEmpty()) {
            StringBuilder rows = new StringBuilder();
            for (Field x : f.list) {
              rows.append("<dt>").append(escape(x.key.isEmpty() ? "—" : x.key))
                  .append("</dt><dd>").append(inlineHtml(x.value)).append("</dd>");
            }
            return "<section class=\"record\" data-type=\"" + escape(n.name)
                + "\"><h4 class=\"record-type\">" + escape(n.name) + "</h4><dl>" + rows
                + "</dl></section>";
          }
        }
        return "<div class=\"unknown\" data-directive=\"" + escape(n.name) + "\">"
            + escape(sourceOf(n)) + "</div>";
      }
    }
  }

  private static String tableHtml(Node n) {
    Table t = parseTable(n);
    if (t.header.isEmpty()) return "";
    StringBuilder head = new StringBuilder();
    for (int i = 0; i < t.header.size(); i++) {
      head.append("<th").append(style(t, i)).append('>').append(inlineHtml(t.header.get(i)))
          .append("</th>");
    }
    StringBuilder rows = new StringBuilder();
    for (List<String> r : t.rows) {
      rows.append("<tr>");
      for (int i = 0; i < r.size(); i++) {
        rows.append("<td").append(style(t, i)).append('>').append(inlineHtml(r.get(i)))
            .append("</td>");
      }
      rows.append("</tr>");
    }
    return "<table>\n<thead><tr>" + head + "</tr></thead>\n<tbody>" + rows + "</tbody>\n</table>";
  }

  private static String style(Table t, int i) {
    if (i >= t.align.size() || t.align.get(i).equals("left")) return "";
    return " style=\"text-align:" + t.align.get(i) + "\"";
  }

  /**
   * Reconstruct a directive's source, for round-tripping and for showing unknown
   * directives.
   */
  public static String sourceOf(Node n) {
    StringBuilder out = new StringBuilder("@").append(n.name);
    if (!n.args.isEmpty()) {
      List<String> parts = new ArrayList<>();
      for (Arg a : n.args.list) {
        String v = a.value;
        if (v.isEmpty() || v.matches(".*[ ,)\"].*")) {
          v = "\"" + v.replace("\\", "\\\\").replace("\"", "\\\"") + "\"";
        }
        parts.add(a.key.isEmpty() ? v : a.key + "=" + v);
      }
      out.append('(').append(String.join(", ", parts)).append(')');
    }
    if (n.kind == Kind.BLOCK) out.append('\n').append(n.text).append("\n@end").append(n.name);
    return out.toString();
  }
}
