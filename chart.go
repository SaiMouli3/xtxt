package xtxt

import (
	"encoding/json"
	"fmt"
	"html"
	"strconv"
	"strings"
)

// chartJSON serialises a chart for the interactive runtime to read back out of
// the DOM. The SVG beside it is the document; this is only what a reader's
// browser needs in order to redraw the same numbers a different way.
//
// The field names are shared with the JavaScript SDK and the runtime, so they
// are spelled out here rather than taken from Go's struct names.
func chartJSON(c Chart) string {
	type series struct {
		Name   string    `json:"name"`
		Values []float64 `json:"values"`
	}
	payload := struct {
		Type   string   `json:"type"`
		Title  string   `json:"title,omitempty"`
		Unit   string   `json:"unit,omitempty"`
		Labels []string `json:"labels"`
		Series []series `json:"series"`
	}{Type: c.Type, Title: c.Title, Unit: c.Unit, Labels: c.Labels}
	for _, s := range c.Series {
		payload.Series = append(payload.Series, series{Name: s.Name, Values: s.Values})
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(b)
}

// Chart is the interpreted payload of an @chart block: a list of categories and
// one or more series of values over them.
type Chart struct {
	Type     string
	Title    string
	Unit     string
	Labels   []string // one per category
	Series   []Series // one or more
	Warnings []string // reported by Lint, not rendered
}

// Series is one named run of values, aligned to Chart.Labels.
type Series struct {
	Name   string
	Values []float64
}

// maxSeries is the number of categorical slots the palette validates for all
// pairs. Past this, series fold into "Other" rather than inventing hues.
const maxSeries = 3

// ParseChart interprets an @chart payload. Rows may be written as
// `Label | 20`, `Label: 20` or `Label 20`; a first row whose trailing cells are
// all non-numeric is read as a header naming the series.
func ParseChart(n Node) Chart {
	c := Chart{
		Type:  strings.ToLower(n.Args.Resolve("type")),
		Title: n.Args.Get("title"),
		Unit:  n.Args.Get("unit"),
	}
	if c.Type == "" {
		c.Type = "bar"
	}

	var rows [][]string
	for _, line := range strings.Split(n.Text, "\n") {
		if isBlank(line) {
			continue
		}
		cells := chartCells(line)
		if len(cells) >= 2 && !isSeparatorRow(cells) {
			rows = append(rows, cells)
		}
	}
	if len(rows) == 0 {
		return c
	}

	names := []string{""}
	if isHeaderRow(rows[0]) {
		names = rows[0]
		rows = rows[1:]
	}
	if len(rows) == 0 {
		return c
	}

	width := 0
	for _, r := range rows {
		if len(r) > width {
			width = len(r)
		}
	}
	for i := 1; i < width; i++ {
		name := ""
		if i < len(names) {
			name = names[i]
		}
		c.Series = append(c.Series, Series{Name: name})
	}
	for _, r := range rows {
		c.Labels = append(c.Labels, r[0])
		for i := range c.Series {
			v := 0.0
			if i+1 < len(r) {
				v = parseNumber(r[i+1])
			}
			c.Series[i].Values = append(c.Series[i].Values, v)
		}
	}

	foldExtraSeries(&c)
	if c.Type == "pie" || c.Type == "donut" {
		c.Warnings = append(c.Warnings,
			"@chart(type=\"pie\") renders as a proportion bar: angles are much harder to compare than lengths")
	}
	return c
}

// foldExtraSeries keeps the drawn series within the palette that was checked for
// contrast, summing the rest into "Other" rather than inventing hues.
func foldExtraSeries(c *Chart) {
	if len(c.Series) <= maxSeries {
		return
	}
	kept := c.Series[:maxSeries]
	other := Series{Name: "Other", Values: make([]float64, len(c.Labels))}
	for _, s := range c.Series[maxSeries:] {
		for i, v := range s.Values {
			other.Values[i] += v
		}
	}
	c.Series = append(kept, other)
	c.Warnings = append(c.Warnings,
		"chart has more series than the palette validates; the extras were folded into \"Other\"")
}

// TableChart reads a @table that carries a chart= argument as a chart over its
// own rows. The table stays the data and is still rendered in full; this is a
// second view of it, which is why a reader that ignores the argument loses
// nothing.
//
// The bool reports whether there is a chart to draw. Warnings are returned on
// the Chart either way, so the caller can show why one did not appear.
func TableChart(n Node) (Chart, bool) {
	kind := strings.ToLower(strings.TrimSpace(n.Args.Get("chart")))
	if kind == "" {
		return Chart{}, false
	}

	c := Chart{Type: kind, Title: n.Args.Get("title"), Unit: n.Args.Get("unit")}
	switch c.Type {
	case "bar", "line", "area", "stacked", "pie", "donut", "proportion":
	default:
		c.Warnings = append(c.Warnings,
			fmt.Sprintf("unknown chart type %q; drawing a bar chart", kind))
		c.Type = "bar"
	}

	t := ParseTable(n)
	if len(t.Header) == 0 || len(t.Rows) == 0 {
		c.Warnings = append(c.Warnings, "the table has no rows to chart")
		return c, false
	}

	x := 0
	if want := strings.TrimSpace(n.Args.Get("x")); want != "" {
		if i := columnIndex(t.Header, want); i >= 0 {
			x = i
		} else {
			c.Warnings = append(c.Warnings,
				fmt.Sprintf("no column named %q; labelling with %q", want, t.Header[0]))
		}
	}

	ys := valueColumns(t, x, n.Args.Get("y"), &c)
	if len(ys) == 0 {
		c.Warnings = append(c.Warnings, "no numeric column to chart")
		return c, false
	}

	for _, i := range ys {
		c.Series = append(c.Series, Series{Name: header(t, i)})
	}
	for _, row := range t.Rows {
		c.Labels = append(c.Labels, cell(row, x))
		for s, i := range ys {
			// A cell that is not a number counts as zero, which the renderer can
			// draw. Leaving a real gap would mean teaching every SVG builder
			// about absent points; until then the warning is the honest signal.
			text := cell(row, i)
			if text != "" && !parseNumberOK(text) {
				c.Warnings = append(c.Warnings,
					fmt.Sprintf("%q in column %q is not a number; charted as zero", text, header(t, i)))
			}
			c.Series[s].Values = append(c.Series[s].Values, parseNumber(text))
		}
	}

	foldExtraSeries(&c)
	return c, true
}

// valueColumns resolves y= to column indexes, defaulting to every column other
// than the labels that holds a number anywhere.
func valueColumns(t Table, x int, want string, c *Chart) []int {
	var out []int
	if strings.TrimSpace(want) != "" {
		for _, name := range strings.Split(want, ",") {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			if i := columnIndex(t.Header, name); i >= 0 {
				out = append(out, i)
			} else {
				c.Warnings = append(c.Warnings, fmt.Sprintf("no column named %q", name))
			}
		}
		return out
	}
	for i := range t.Header {
		if i == x {
			continue
		}
		for _, row := range t.Rows {
			if parseNumberOK(cell(row, i)) {
				out = append(out, i)
				break
			}
		}
	}
	return out
}

// columnIndex finds a header by name, ignoring case and surrounding space.
func columnIndex(header []string, name string) int {
	name = strings.ToLower(strings.TrimSpace(name))
	for i, h := range header {
		if strings.ToLower(strings.TrimSpace(h)) == name {
			return i
		}
	}
	return -1
}

func header(t Table, i int) string {
	if i < len(t.Header) {
		return strings.TrimSpace(t.Header[i])
	}
	return ""
}

func cell(row []string, i int) string {
	if i < len(row) {
		return strings.TrimSpace(row[i])
	}
	return ""
}

// chartCells splits a data row. Explicit separators win; otherwise the value is
// whatever follows the last run of whitespace, so `New York 20` keeps its label.
func chartCells(line string) []string {
	if strings.Contains(line, "|") {
		return splitCells(line)
	}
	if i := strings.LastIndex(line, ":"); i >= 0 && parseNumberOK(line[i+1:]) {
		return []string{strings.TrimSpace(line[:i]), strings.TrimSpace(line[i+1:])}
	}
	fields := strings.Fields(line)
	for i := len(fields) - 1; i > 0; i-- {
		if !parseNumberOK(fields[i]) {
			return []string{strings.Join(fields[:i+1], " "), strings.Join(fields[i+1:], " ")}
		}
	}
	if len(fields) >= 2 {
		return []string{fields[0], strings.Join(fields[1:], " ")}
	}
	return fields
}

func isHeaderRow(cells []string) bool {
	for _, c := range cells[1:] {
		if parseNumberOK(c) {
			return false
		}
	}
	return len(cells) >= 2
}

func parseNumberOK(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	_, err := strconv.ParseFloat(cleanNumber(s), 64)
	return err == nil
}

func parseNumber(s string) float64 {
	v, _ := strconv.ParseFloat(cleanNumber(s), 64)
	return v
}

func cleanNumber(s string) string {
	return strings.Map(func(r rune) rune {
		if (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '+' || r == 'e' || r == 'E' {
			return r
		}
		return -1
	}, strings.TrimSpace(s))
}

func formatNumber(v float64, unit string) string {
	s := strconv.FormatFloat(v, 'f', -1, 64)
	if v == float64(int64(v)) {
		s = strconv.FormatInt(int64(v), 10)
	}
	return s + unit
}

// RenderChartSVG draws a chart as a self-contained, script-free SVG. Colours
// come from CSS custom properties so the surrounding page controls the theme;
// native <title> elements give hover tooltips without JavaScript.
func RenderChartSVG(c Chart) string {
	if len(c.Labels) == 0 || len(c.Series) == 0 {
		return ""
	}
	switch c.Type {
	case "line", "area":
		return lineSVG(c)
	case "pie", "donut", "proportion", "stacked":
		return proportionSVG(c)
	default:
		return barSVG(c)
	}
}

const (
	chartW    = 640
	labelCol  = 128
	rowH      = 30
	barH      = 18
	padTop    = 14
	padBottom = 10
)

func seriesColor(i int) string {
	return fmt.Sprintf("var(--chart-%d)", i%maxSeries+1)
}

func svgOpen(w, h int, label string) string {
	return fmt.Sprintf(`<svg class="xtxt-chart" viewBox="0 0 %d %d" width="100%%" height="%d" `+
		`role="img" aria-label="%s" xmlns="http://www.w3.org/2000/svg">`, w, h, h, html.EscapeString(label))
}

// barPath draws a bar with its data end rounded and its baseline end square.
func barPath(x, y, w, h, r float64) string {
	if w < r*2 {
		r = w / 2
	}
	if r < 0 {
		r = 0
	}
	return fmt.Sprintf("M%.1f %.1f H%.1f a%.1f %.1f 0 0 1 %.1f %.1f V%.1f a%.1f %.1f 0 0 1 %.1f %.1f H%.1f Z",
		x, y, x+w-r, r, r, r, r, y+h-r, r, r, -r, r, x)
}

func barSVG(c Chart) string {
	n := len(c.Labels)
	groups := len(c.Series)
	each := barH
	if groups > 1 {
		each = (rowH - 8) / groups
	}
	h := padTop + n*rowH + padBottom
	if groups > 1 {
		h += 22 // legend
	}

	max := 0.0
	for _, s := range c.Series {
		for _, v := range s.Values {
			if v > max {
				max = v
			}
		}
	}
	if max <= 0 {
		max = 1
	}
	plot := float64(chartW - labelCol - 64)

	var b strings.Builder
	b.WriteString(svgOpen(chartW, h, chartLabel(c)))
	for i, label := range c.Labels {
		top := float64(padTop + i*rowH)
		fmt.Fprintf(&b, `<text class="c-label" x="%d" y="%.1f" text-anchor="end">%s</text>`,
			labelCol-10, top+float64(rowH)/2+4, html.EscapeString(label))
		for si, s := range c.Series {
			w := plot * s.Values[i] / max
			y := top + (float64(rowH)-float64(each*groups))/2 + float64(si*each)
			gap := 0.0
			if groups > 1 {
				gap = 2 // surface gap between adjacent fills
			}
			// data-index and data-series are the only hooks the interactive
			// runtime needs: one to know which category is under the pointer,
			// one to hide a series without redrawing anything.
			fmt.Fprintf(&b, `<path data-index="%d" data-series="%d" d="%s" fill="%s"><title>%s%s: %s</title></path>`,
				i, si, barPath(float64(labelCol), y, w, float64(each)-gap, 4), seriesColor(si),
				html.EscapeString(label), seriesSuffix(s.Name), html.EscapeString(formatNumber(s.Values[i], c.Unit)))
			if groups == 1 {
				fmt.Fprintf(&b, `<text class="c-value" data-series="%d" x="%.1f" y="%.1f">%s</text>`,
					si, float64(labelCol)+w+8, y+float64(each)/2+4, html.EscapeString(formatNumber(s.Values[i], c.Unit)))
			}
		}
	}
	if groups > 1 {
		b.WriteString(legend(c, float64(padTop+n*rowH)+16))
	}
	b.WriteString("</svg>")
	return b.String()
}

func proportionSVG(c Chart) string {
	// Part-to-whole as one stacked bar: lengths compare, angles do not.
	total := 0.0
	for _, v := range c.Series[0].Values {
		total += v
	}
	if total <= 0 {
		return ""
	}
	const barTop, thick = 16.0, 34.0
	h := int(barTop+thick) + 30 + 22*((len(c.Labels)+2)/3)

	var b strings.Builder
	b.WriteString(svgOpen(chartW, h, chartLabel(c)))
	x := 0.0
	for i, v := range c.Series[0].Values {
		w := float64(chartW) * v / total
		fmt.Fprintf(&b, `<rect data-index="%d" x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s" rx="2">`+
			`<title>%s: %s (%.1f%%)</title></rect>`,
			i, x, barTop, maxf(w-2, 0), thick, seriesColor(i),
			html.EscapeString(c.Labels[i]), html.EscapeString(formatNumber(v, c.Unit)), v/total*100)
		if w > 46 {
			fmt.Fprintf(&b, `<text class="c-inbar" x="%.1f" y="%.1f" text-anchor="middle">%.0f%%</text>`,
				x+w/2-1, barTop+thick/2+4, v/total*100)
		}
		x += w
	}
	for i, label := range c.Labels {
		col, row := i%3, i/3
		lx, ly := float64(col)*float64(chartW)/3, barTop+thick+24+float64(row)*22
		fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="9" height="9" rx="2" fill="%s"/>`, lx, ly-9, seriesColor(i))
		fmt.Fprintf(&b, `<text class="c-label" x="%.1f" y="%.1f">%s %s</text>`,
			lx+14, ly, html.EscapeString(label), html.EscapeString(formatNumber(c.Series[0].Values[i], c.Unit)))
	}
	b.WriteString("</svg>")
	return b.String()
}

func lineSVG(c Chart) string {
	const h, top, bottom = 260.0, 18.0, 46.0
	left := 44.0
	plotW := float64(chartW) - left - 20
	plotH := h - top - bottom

	max, min := 0.0, 0.0
	for _, s := range c.Series {
		for _, v := range s.Values {
			max = maxf(max, v)
			min = minf(min, v)
		}
	}
	if max == min {
		max = min + 1
	}
	x := func(i int) float64 {
		if len(c.Labels) == 1 {
			return left + plotW/2
		}
		return left + plotW*float64(i)/float64(len(c.Labels)-1)
	}
	y := func(v float64) float64 { return top + plotH*(1-(v-min)/(max-min)) }

	var b strings.Builder
	b.WriteString(svgOpen(chartW, int(h), chartLabel(c)))
	for _, frac := range []float64{0, 0.5, 1} {
		gy := top + plotH*frac
		fmt.Fprintf(&b, `<line class="c-grid" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`, left, gy, left+plotW, gy)
		fmt.Fprintf(&b, `<text class="c-axis" x="%.1f" y="%.1f" text-anchor="end">%s</text>`,
			left-8, gy+4, html.EscapeString(formatNumber(min+(max-min)*(1-frac), "")))
	}
	for si, s := range c.Series {
		var pts []string
		for i, v := range s.Values {
			pts = append(pts, fmt.Sprintf("%.1f,%.1f", x(i), y(v)))
		}
		if c.Type == "area" {
			fmt.Fprintf(&b, `<polygon data-series="%d" points="%.1f,%.1f %s %.1f,%.1f" fill="%s" opacity="0.14"/>`,
				si, x(0), top+plotH, strings.Join(pts, " "), x(len(s.Values)-1), top+plotH, seriesColor(si))
		}
		fmt.Fprintf(&b, `<polyline data-series="%d" points="%s" fill="none" stroke="%s" stroke-width="2" `+
			`stroke-linejoin="round" stroke-linecap="round"/>`, si, strings.Join(pts, " "), seriesColor(si))
		for i, v := range s.Values {
			fmt.Fprintf(&b, `<circle data-index="%d" data-series="%d" cx="%.1f" cy="%.1f" r="4" fill="%s" stroke="var(--chart-surface)" stroke-width="2">`+
				`<title>%s%s: %s</title></circle>`,
				i, si, x(i), y(v), seriesColor(si), html.EscapeString(c.Labels[i]), seriesSuffix(s.Name),
				html.EscapeString(formatNumber(v, c.Unit)))
		}
		// Label the ends and the peak only — never every point. Deduplicate:
		// when the peak is an endpoint the same label would be drawn twice, and
		// two identical strings at one position render as a smudge. Endpoint
		// labels anchor outward so they clear the axis rather than sit on it.
		last := len(s.Values) - 1
		for _, i := range dedupe(0, indexOfMax(s.Values), last) {
			anchor, dx := "middle", 0.0
			switch i {
			case 0:
				anchor, dx = "start", 2
			case last:
				anchor, dx = "end", -2
			}
			fmt.Fprintf(&b, `<text class="c-value" x="%.1f" y="%.1f" text-anchor="%s">%s</text>`,
				x(i)+dx, y(s.Values[i])-10, anchor, html.EscapeString(formatNumber(s.Values[i], c.Unit)))
		}
	}
	for i, label := range c.Labels {
		if len(c.Labels) > 12 && i%(len(c.Labels)/8) != 0 {
			continue
		}
		fmt.Fprintf(&b, `<text class="c-axis" x="%.1f" y="%.1f" text-anchor="middle">%s</text>`,
			x(i), top+plotH+18, html.EscapeString(label))
	}
	if len(c.Series) > 1 {
		b.WriteString(legend(c, h-14))
	}
	b.WriteString("</svg>")
	return b.String()
}

func legend(c Chart, y float64) string {
	var b strings.Builder
	x := float64(labelCol)
	for i, s := range c.Series {
		name := s.Name
		if name == "" {
			name = "Series " + itoa(i+1)
		}
		fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="9" height="9" rx="2" fill="%s"/>`, x, y-9, seriesColor(i))
		fmt.Fprintf(&b, `<text class="c-label" x="%.1f" y="%.1f">%s</text>`, x+14, y, html.EscapeString(name))
		x += 24 + float64(len([]rune(name)))*7
	}
	return b.String()
}

func seriesSuffix(name string) string {
	if name == "" {
		return ""
	}
	return " · " + name
}

func chartLabel(c Chart) string {
	if c.Title != "" {
		return c.Title
	}
	return c.Type + " chart of " + strings.Join(c.Labels, ", ")
}

// dedupe keeps the first occurrence of each index, preserving order.
func dedupe(idx ...int) []int {
	seen := map[int]bool{}
	out := idx[:0:0]
	for _, i := range idx {
		if !seen[i] {
			seen[i] = true
			out = append(out, i)
		}
	}
	return out
}

func indexOfMax(v []float64) int {
	best := 0
	for i := range v {
		if v[i] > v[best] {
			best = i
		}
	}
	return best
}

func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func minf(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// chartTableHTML is the always-available text alternative. The light-mode
// palette has a slot below 3:1 against white, so a table view is required, not
// optional.
func chartTableHTML(c Chart) string {
	var b strings.Builder
	b.WriteString(`<details class="chart-data"><summary>Data</summary><table>` + "\n<thead><tr><th></th>")
	for i, s := range c.Series {
		name := s.Name
		if name == "" {
			name = "Value"
			if len(c.Series) > 1 {
				name = "Series " + itoa(i+1)
			}
		}
		fmt.Fprintf(&b, `<th style="text-align:right">%s</th>`, html.EscapeString(name))
	}
	b.WriteString("</tr></thead>\n<tbody>\n")
	for i, label := range c.Labels {
		fmt.Fprintf(&b, "<tr><td>%s</td>", html.EscapeString(label))
		for _, s := range c.Series {
			fmt.Fprintf(&b, `<td style="text-align:right">%s</td>`, html.EscapeString(formatNumber(s.Values[i], c.Unit)))
		}
		b.WriteString("</tr>\n")
	}
	b.WriteString("</tbody>\n</table></details>\n")
	return b.String()
}
