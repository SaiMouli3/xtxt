package io.github.saimouli3.xtxt;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ArrayNode;
import com.fasterxml.jackson.databind.node.ObjectNode;
import java.io.IOException;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.ArrayList;
import java.util.List;
import java.util.stream.Stream;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.MethodSource;

/**
 * The Java SDK is checked against the same fixtures as the Go reference.
 *
 * <p>{@code canonical} lives here rather than in the library so that the
 * library itself stays dependency-free: only the test needs to speak JSON.
 */
class ConformanceTest {

  private static final ObjectMapper MAPPER = new ObjectMapper();

  private static Path casesDir() {
    return Path.of(System.getProperty("user.dir"), "..", "..", "conformance", "cases")
        .normalize();
  }

  static Stream<Path> cases() throws IOException {
    Path dir = casesDir();
    assertTrue(Files.isDirectory(dir), "conformance cases not found at " + dir);
    try (var files = Files.list(dir)) {
      List<Path> out = files.filter(p -> p.toString().endsWith(".xtxt")).sorted().toList();
      assertFalse(out.isEmpty(), "no conformance cases in " + dir);
      return out.stream();
    }
  }

  /**
   * The normalised shape every implementation must produce. Every field is
   * present with an explicit default, so an implementation cannot pass by
   * omitting something the reference emits.
   */
  private static ObjectNode canonical(Xtxt.Document doc) {
    ObjectNode root = MAPPER.createObjectNode();
    root.put("version", doc.version);
    ArrayNode nodes = root.putArray("nodes");
    for (Xtxt.Node n : doc.nodes) {
      ObjectNode o = nodes.addObject();
      o.put("kind", n.kind.wire());
      o.put("name", n.name);
      o.put("level", n.level);
      o.put("text", n.text);
      ArrayNode args = o.putArray("args");
      for (Xtxt.Arg a : n.args.list) {
        ObjectNode ao = args.addObject();
        ao.put("key", a.key);
        ao.put("value", a.value);
      }
      ArrayNode items = o.putArray("items");
      for (Xtxt.Item it : n.items) {
        ObjectNode io = items.addObject();
        io.put("text", it.text);
        io.put("ordered", it.ordered);
        if (it.checked == null) {
          io.putNull("checked");
        } else {
          io.put("checked", it.checked);
        }
      }
      o.put("line", n.line);
    }
    return root;
  }

  private static ArrayNode canonicalIssues(List<Xtxt.Issue> issues) {
    ArrayNode out = MAPPER.createArrayNode();
    for (Xtxt.Issue i : Xtxt.sortIssues(issues)) {
      ObjectNode o = out.addObject();
      o.put("severity", i.severity.wire());
      o.put("line", i.line);
    }
    return out;
  }

  @ParameterizedTest(name = "{0}")
  @MethodSource("cases")
  void matchesTheReferenceImplementation(Path input) throws IOException {
    String src = Files.readString(input);
    JsonNode expected =
        MAPPER.readTree(Files.readString(Path.of(input.toString().replace(".xtxt", ".json"))));

    Xtxt.ParseResult res = Xtxt.parse(src);
    List<Xtxt.Issue> issues = new ArrayList<>(res.issues);
    issues.addAll(Xtxt.validate(res.doc));

    assertEquals(expected.get("ast"), (JsonNode) canonical(res.doc), "AST mismatch");
    assertEquals(expected.get("issues"), (JsonNode) canonicalIssues(issues), "issues mismatch");
  }

  @Test
  void inlineFormatting() {
    assertEquals("<strong>b</strong> and <code>a&lt;b</code>", Xtxt.inlineHtml("**b** and `a<b`"));
    assertEquals("b and x", Xtxt.inlineText("**b** and [x](y)"));
  }

  @Test
  void inlinePreservesNonAscii() {
    for (String s : List.of("an em dash — here", "日本語", "café", "emoji 🎉")) {
      String got = Xtxt.inlineHtml(s);
      s.codePoints()
          .filter(c -> c > 127)
          .forEach(c ->
              assertTrue(got.contains(new String(Character.toChars(c))),
                  s + " lost a character: " + got));
    }
    assertEquals("a — b &amp; c", Xtxt.inlineHtml("a — b & c"));
  }

  @Test
  void proseIsNotMistakenForAField() {
    Xtxt.Fields f = Xtxt.parseFields("There is one rule that matters here: keep it readable.");
    assertEquals(1, f.size());
    assertEquals("", f.list.get(0).key);
  }

  @Test
  void recordsAndTasks() {
    var res = Xtxt.parse("# T\n\n- [x] done\n\n@task\nTitle: Ship it\nStatus: Done\n@endtask\n");
    var e = Xtxt.extract(res.doc);
    assertEquals(List.of("done", "Ship it"), e.tasks.stream().map(t -> t.title).toList());
    assertTrue(e.tasks.stream().allMatch(t -> t.done));
    assertEquals(List.of("Title", "Status"), e.blocks.get(0).order);
    assertEquals(1, e.outline.size());
    assertEquals("T", e.outline.get(0).text);
  }

  @Test
  void unknownDirectivesAreWarningsNotErrors() {
    var res = Xtxt.parse("@futurething(a=1)\n\n@newblock\nbody\n@endnewblock\n");
    assertFalse(res.hasErrors());
    assertEquals(2, res.doc.nodes.size());
    assertEquals("body", res.doc.nodes.get(1).text);
    assertEquals(2, Xtxt.validate(res.doc).size());
    assertTrue(Xtxt.renderHtml(res.doc).contains("futurething"));
  }
}
