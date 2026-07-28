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
			b.WriteString(html.EscapeString(string(s[i])))
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
			if label, target, end, ok := link(s, i); ok {
				b.WriteString(`<a href="` + html.EscapeString(target) + `">` + InlineHTML(label) + `</a>`)
				i = end
				continue
			}
			b.WriteString("[")
		default:
			b.WriteString(html.EscapeString(string(c)))
		}
	}
	return b.String()
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
