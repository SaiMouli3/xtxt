package xtxt

import (
	"html"
	"strings"
)

// InlineHTML converts XTXT inline markup to HTML, escaping everything else.
// Code spans are literal: no other rule applies inside them.
func InlineHTML(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\\' && i+1 < len(s):
			i++
			escapeByte(&b, s[i])
		case c == '`':
			if end := strings.IndexByte(s[i+1:], '`'); end >= 0 {
				b.WriteString("<code>" + html.EscapeString(s[i+1:i+1+end]) + "</code>")
				i += end + 1
				continue
			}
			b.WriteString("&#96;")
		case c == '*':
			tag, mark := "em", "*"
			if strings.HasPrefix(s[i:], "**") {
				tag, mark = "strong", "**"
			}
			if end := findClose(s, i+len(mark), mark); end >= 0 {
				b.WriteString("<" + tag + ">" + InlineHTML(s[i+len(mark):end]) + "</" + tag + ">")
				i = end + len(mark) - 1
				continue
			}
			b.WriteString("*")
		case c == '[':
			if id, end, ok := footnoteRef(s, i); ok {
				e := html.EscapeString(id)
				b.WriteString(`<sup class="fnref" id="fnref-` + e + `"><a href="#fn-` + e + `">` + e + `</a></sup>`)
				i = end
				continue
			}
			if label, target, end, ok := link(s, i); ok {
				b.WriteString(`<a href="` + html.EscapeString(target) + `">` + InlineHTML(label) + `</a>`)
				i = end
				continue
			}
			b.WriteString("[")
		default:
			escapeByte(&b, c)
		}
	}
	return b.String()
}

// escapeByte writes one byte, escaping the five characters that matter in HTML.
// It must work a byte at a time rather than converting to a rune, because the
// scan is byte-oriented: `string(byte)` on a UTF-8 continuation byte produces a
// different character entirely, which silently mangles every non-ASCII
// document. Bytes above ASCII are passed through untouched, so valid UTF-8 in
// gives valid UTF-8 out.
func escapeByte(b *strings.Builder, c byte) {
	switch c {
	case '&':
		b.WriteString("&amp;")
	case '<':
		b.WriteString("&lt;")
	case '>':
		b.WriteString("&gt;")
	case '"':
		b.WriteString("&#34;")
	case '\'':
		b.WriteString("&#39;")
	default:
		b.WriteByte(c)
	}
}

// InlineText strips inline markup, for plain-text and terminal output.
func InlineText(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '\\' && i+1 < len(s):
			i++
			b.WriteByte(s[i])
		case c == '`':
			if end := strings.IndexByte(s[i+1:], '`'); end >= 0 {
				b.WriteString(s[i+1 : i+1+end])
				i += end + 1
				continue
			}
			b.WriteByte(c)
		case c == '*':
			mark := "*"
			if strings.HasPrefix(s[i:], "**") {
				mark = "**"
			}
			if end := findClose(s, i+len(mark), mark); end >= 0 {
				b.WriteString(InlineText(s[i+len(mark) : end]))
				i = end + len(mark) - 1
				continue
			}
			b.WriteByte(c)
		case c == '[':
			if id, end, ok := footnoteRef(s, i); ok {
				b.WriteString("[" + id + "]")
				i = end
				continue
			}
			if label, _, end, ok := link(s, i); ok {
				b.WriteString(InlineText(label))
				i = end
				continue
			}
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// findClose returns the index of the next unescaped mark at or after from.
func findClose(s string, from int, mark string) int {
	for i := from; i+len(mark) <= len(s); i++ {
		if s[i] == '\\' {
			i++
			continue
		}
		if strings.HasPrefix(s[i:], mark) {
			if i == from {
				return -1 // empty span
			}
			return i
		}
	}
	return -1
}

// footnoteRef parses a marker [^id] starting at s[i] == '['. The body lives in
// an @footnote(id="…") block elsewhere in the document.
func footnoteRef(s string, i int) (id string, end int, ok bool) {
	if i+2 >= len(s) || s[i+1] != '^' {
		return "", 0, false
	}
	close := strings.IndexByte(s[i+2:], ']')
	if close <= 0 {
		return "", 0, false
	}
	id = s[i+2 : i+2+close]
	if strings.ContainsAny(id, " \t") {
		return "", 0, false
	}
	return id, i + 2 + close, true
}

// link parses [label](target) starting at s[i] == '['.
func link(s string, i int) (label, target string, end int, ok bool) {
	close := findClose(s, i+1, "]")
	if close < 0 || close+1 >= len(s) || s[close+1] != '(' {
		return "", "", 0, false
	}
	inner, found := balanced(s[close+1:])
	if !found {
		return "", "", 0, false
	}
	return s[i+1 : close], strings.TrimSpace(inner), close + 1 + len(inner) + 1, true
}
