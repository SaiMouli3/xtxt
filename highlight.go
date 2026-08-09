package xtxt

import (
	"html"
	"strings"
)

// Syntax highlighting for @code blocks, without a dependency and without a
// script in the output.
//
// This is one generic tokeniser parameterised per language rather than a lexer
// per language. It recognises comments, strings, numbers and keywords, which is
// what carries almost all of the readability gain; it does not attempt to parse
// anything. A tokeniser that is approximately right for thirty languages beats
// an exact one for three, and it cannot drift out of step with a grammar it
// never modelled.
//
// The output is four span classes styled by CSS, so a page owns its own theme
// and the markup stays legible with styling absent.

type langSpec struct {
	line     []string  // line comment markers
	block    [2]string // block comment open and close, empty when absent
	quotes   string    // characters that open a string
	keywords map[string]bool
}

func kw(words ...string) map[string]bool {
	m := make(map[string]bool, len(words))
	for _, w := range words {
		m[w] = true
	}
	return m
}

// A short keyword list per language on purpose. The common ones colour the
// shape of the code; an exhaustive list adds noise and maintenance for very
// little extra signal.
var languages = map[string]langSpec{
	"go": {line: []string{"//"}, block: [2]string{"/*", "*/"}, quotes: "\"'`",
		keywords: kw("break", "case", "chan", "const", "continue", "default", "defer",
			"else", "fallthrough", "for", "func", "go", "goto", "if", "import",
			"interface", "map", "package", "range", "return", "select", "struct",
			"switch", "type", "var", "nil", "true", "false")},
	"javascript": {line: []string{"//"}, block: [2]string{"/*", "*/"}, quotes: "\"'`",
		keywords: kw("async", "await", "break", "case", "catch", "class", "const",
			"continue", "default", "delete", "do", "else", "export", "extends",
			"finally", "for", "from", "function", "if", "import", "in", "instanceof",
			"let", "new", "of", "return", "super", "switch", "this", "throw", "try",
			"typeof", "var", "void", "while", "yield", "null", "true", "false")},
	"python": {line: []string{"#"}, quotes: "\"'",
		keywords: kw("and", "as", "assert", "async", "await", "break", "class",
			"continue", "def", "del", "elif", "else", "except", "finally", "for",
			"from", "global", "if", "import", "in", "is", "lambda", "None", "nonlocal",
			"not", "or", "pass", "raise", "return", "try", "while", "with", "yield",
			"True", "False")},
	"rust": {line: []string{"//"}, block: [2]string{"/*", "*/"}, quotes: "\"'",
		keywords: kw("as", "async", "await", "break", "const", "continue", "crate",
			"dyn", "else", "enum", "extern", "fn", "for", "if", "impl", "in", "let",
			"loop", "match", "mod", "move", "mut", "pub", "ref", "return", "self",
			"static", "struct", "trait", "type", "unsafe", "use", "where", "while",
			"true", "false")},
	"c": {line: []string{"//"}, block: [2]string{"/*", "*/"}, quotes: "\"'",
		keywords: kw("auto", "break", "case", "char", "const", "continue", "default",
			"do", "double", "else", "enum", "extern", "float", "for", "goto", "if",
			"int", "long", "return", "short", "signed", "sizeof", "static", "struct",
			"switch", "typedef", "union", "unsigned", "void", "volatile", "while")},
	"java": {line: []string{"//"}, block: [2]string{"/*", "*/"}, quotes: "\"'",
		keywords: kw("abstract", "boolean", "break", "case", "catch", "class", "const",
			"continue", "default", "do", "double", "else", "enum", "extends", "final",
			"finally", "float", "for", "if", "implements", "import", "instanceof",
			"int", "interface", "long", "new", "package", "private", "protected",
			"public", "return", "static", "super", "switch", "this", "throw", "throws",
			"try", "void", "while", "null", "true", "false")},
	"shell": {line: []string{"#"}, quotes: "\"'",
		keywords: kw("case", "do", "done", "elif", "else", "esac", "export", "fi",
			"for", "function", "if", "in", "local", "return", "then", "while")},
	"sql": {line: []string{"--"}, block: [2]string{"/*", "*/"}, quotes: "'\"",
		keywords: kw("AND", "AS", "BY", "CREATE", "DELETE", "DROP", "FROM", "GROUP",
			"HAVING", "INSERT", "INTO", "JOIN", "LEFT", "LIMIT", "NOT", "NULL", "ON",
			"OR", "ORDER", "SELECT", "SET", "TABLE", "UPDATE", "VALUES", "WHERE")},
	"json": {quotes: "\"", keywords: kw("true", "false", "null")},
	"yaml": {line: []string{"#"}, quotes: "\"'", keywords: kw("true", "false", "null")},
}

// Aliases keep the table small without making authors guess the canonical name.
var langAliases = map[string]string{
	"js": "javascript", "jsx": "javascript", "ts": "javascript",
	"typescript": "javascript", "tsx": "javascript", "mjs": "javascript",
	"py": "python", "rs": "rust", "sh": "shell", "bash": "shell", "zsh": "shell",
	"cpp": "c", "c++": "c", "h": "c", "hpp": "c", "cc": "c",
	"yml": "yaml", "golang": "go",
}

// HighlightHTML renders source as escaped HTML with span classes around
// comments, strings, numbers and keywords. An unknown language is escaped and
// returned unchanged, which is the correct outcome rather than a guess.
func HighlightHTML(source, language string) string {
	name := strings.ToLower(strings.TrimSpace(language))
	if alias, ok := langAliases[name]; ok {
		name = alias
	}
	spec, ok := languages[name]
	if !ok {
		return html.EscapeString(source)
	}

	var b strings.Builder
	// Plain runs are escaped in one go rather than per byte; only the spans
	// need bookkeeping.
	plain := 0
	flush := func(upto int) {
		if upto > plain {
			b.WriteString(html.EscapeString(source[plain:upto]))
		}
	}
	span := func(class, text string) {
		b.WriteString(`<span class="tok-` + class + `">`)
		b.WriteString(html.EscapeString(text))
		b.WriteString(`</span>`)
	}

	for i := 0; i < len(source); {
		// Line comment.
		if marker, n := matchAny(source, i, spec.line); n > 0 {
			_ = marker
			end := strings.IndexByte(source[i:], '\n')
			if end < 0 {
				end = len(source)
			} else {
				end += i
			}
			flush(i)
			span("com", source[i:end])
			i, plain = end, end
			continue
		}
		// Block comment.
		if spec.block[0] != "" && strings.HasPrefix(source[i:], spec.block[0]) {
			end := strings.Index(source[i+len(spec.block[0]):], spec.block[1])
			if end < 0 {
				end = len(source)
			} else {
				end = i + len(spec.block[0]) + end + len(spec.block[1])
			}
			flush(i)
			span("com", source[i:end])
			i, plain = end, end
			continue
		}
		// String. An unterminated quote runs to end of line rather than
		// swallowing the rest of the file.
		if strings.IndexByte(spec.quotes, source[i]) >= 0 {
			end := stringEnd(source, i)
			flush(i)
			span("str", source[i:end])
			i, plain = end, end
			continue
		}
		// Number.
		if isDigit(source[i]) && (i == 0 || !isWordByte(source[i-1])) {
			end := i
			for end < len(source) && (isDigit(source[end]) || source[end] == '.' ||
				source[end] == 'x' || isHex(source[end])) {
				end++
			}
			flush(i)
			span("num", source[i:end])
			i, plain = end, end
			continue
		}
		// Word, which may be a keyword.
		if isWordByte(source[i]) && (i == 0 || !isWordByte(source[i-1])) {
			end := i
			for end < len(source) && isWordByte(source[end]) {
				end++
			}
			if spec.keywords[source[i:end]] {
				flush(i)
				span("kw", source[i:end])
				plain = end
			}
			i = end
			continue
		}
		i++
	}
	flush(len(source))
	return b.String()
}

func matchAny(s string, i int, markers []string) (string, int) {
	for _, m := range markers {
		if m != "" && strings.HasPrefix(s[i:], m) {
			return m, len(m)
		}
	}
	return "", 0
}

// stringEnd finds the closing quote, honouring backslash escapes and stopping
// at a newline so one stray quote cannot colour the remainder of the listing.
func stringEnd(s string, start int) int {
	quote := s[start]
	for i := start + 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '\n':
			if quote != '`' {
				return i
			}
		case quote:
			return i + 1
		}
	}
	return len(s)
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }
func isHex(c byte) bool   { return c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F' }
func isWordByte(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || isDigit(c)
}
