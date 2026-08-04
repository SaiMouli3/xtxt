/**
 * Chart rendering for XTXT `@chart` blocks: script-free inline SVG.
 *
 * Colours come from CSS custom properties (`--chart-1..3`), so the surrounding
 * page owns the theme. The palette is capped at three slots because that is
 * what validates for colour-vision-deficient and normal-vision separation on
 * both the light and dark surfaces; further series fold into "Other" rather
 * than inventing hues.
 *
 * @module xtxt/chart
 */

const MAX_SERIES = 3;
const CHART_W = 640;
const LABEL_COL = 128;
const ROW_H = 30;
const BAR_H = 18;
const PAD_TOP = 14;
const PAD_BOTTOM = 10;

const esc = (s) => String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;')
  .replace(/>/g, '&gt;').replace(/"/g, '&#34;').replace(/'/g, '&#39;');

const isBlank = (s) => s.trim() === '';
const cleanNumber = (s) => String(s).trim().replace(/[^0-9.eE+-]/g, '');
// Note the empty-string guard: `Number('')` is 0, not NaN, so without it a
// label like "York" cleans down to "" and reads as the number zero — which
// silently turns "New York 20" into a series named "New".
const isNumeric = (s) => {
  if (s === undefined) return false;
  const clean = cleanNumber(s);
  return clean !== '' && !Number.isNaN(Number(clean));
};
const toNumber = (s) => (Number(cleanNumber(s)) || 0);
const seriesColor = (i) => `var(--chart-${(i % MAX_SERIES) + 1})`;

function formatNumber(v, unit = '') {
  return (Number.isInteger(v) ? String(v) : String(v)) + unit;
}

/** Split one data row: explicit separators win, else the value is the last field. */
function chartCells(line) {
  if (line.includes('|')) return line.trim().replace(/^\||\|$/g, '').split('|').map((c) => c.trim());
  const colon = line.lastIndexOf(':');
  if (colon >= 0 && isNumeric(line.slice(colon + 1))) {
    return [line.slice(0, colon).trim(), line.slice(colon + 1).trim()];
  }
  const f = line.trim().split(/\s+/);
  for (let i = f.length - 1; i > 0; i--) {
    if (!isNumeric(f[i])) return [f.slice(0, i + 1).join(' '), f.slice(i + 1).join(' ')];
  }
  return f.length >= 2 ? [f[0], f.slice(1).join(' ')] : f;
}

const isSeparatorRow = (cells) => cells.length > 0 && cells.every((c) => c && !c.replace(/[-:]/g, ''));
const isHeaderRow = (cells) => cells.length >= 2 && cells.slice(1).every((c) => !isNumeric(c));

/** Interpret the payload of an `@chart` block. */
export function parseChart(n) {
  const c = {
    type: (n.args.resolve('type') || 'bar').toLowerCase(),
    title: n.args.get('title'),
    unit: n.args.get('unit'),
    labels: [], series: [], warnings: [],
  };
  let rows = [];
  for (const line of n.text.split('\n')) {
    if (isBlank(line)) continue;
    const cells = chartCells(line);
    if (cells.length >= 2 && !isSeparatorRow(cells)) rows.push(cells);
  }
  if (!rows.length) return c;

  let names = [''];
  if (isHeaderRow(rows[0])) { names = rows[0]; rows = rows.slice(1); }
  if (!rows.length) return c;

  const width = Math.max(...rows.map((r) => r.length));
  for (let i = 1; i < width; i++) c.series.push({ name: names[i] ?? '', values: [] });
  for (const r of rows) {
    c.labels.push(r[0]);
    c.series.forEach((s, i) => s.values.push(toNumber(r[i + 1])));
  }

  if (c.series.length > MAX_SERIES) {
    const other = { name: 'Other', values: new Array(c.labels.length).fill(0) };
    for (const s of c.series.slice(MAX_SERIES)) s.values.forEach((v, i) => { other.values[i] += v; });
    c.series = [...c.series.slice(0, MAX_SERIES), other];
    c.warnings.push('chart has more series than the palette validates; the extras were folded into "Other"');
  }
  if (c.type === 'pie' || c.type === 'donut') {
    c.warnings.push('@chart(type="pie") renders as a proportion bar: angles are much harder to compare than lengths');
  }
  return c;
}

const CHART_TYPES = ['bar', 'line', 'area', 'stacked', 'pie', 'donut', 'proportion'];

const columnIndex = (header, name) => header.findIndex(
  (h) => String(h).trim().toLowerCase() === String(name).trim().toLowerCase());
const headerAt = (t, i) => String(t.header[i] ?? '').trim();
const cellAt = (row, i) => String(row[i] ?? '').trim();

/**
 * Reads a `@table` carrying a `chart=` argument as a chart over its own rows.
 * The table stays the data and is still rendered in full; this is a second view
 * of it, so a reader that ignores the argument loses nothing.
 *
 * Takes the parsed table rather than parsing it, because `parseTable` lives in
 * the main module and importing it here would make the two files circular.
 *
 * Returns null when there is no chart to draw; `warnings` is carried on the
 * returned object so a caller can say why one did not appear.
 */
export function tableChart(n, table) {
  const kind = String(n.args.get('chart') ?? '').trim().toLowerCase();
  if (!kind) return null;

  const c = {
    type: kind, title: n.args.get('title'), unit: n.args.get('unit'),
    labels: [], series: [], warnings: [],
  };
  if (!CHART_TYPES.includes(c.type)) {
    c.warnings.push(`unknown chart type "${kind}"; drawing a bar chart`);
    c.type = 'bar';
  }
  if (!table.header.length || !table.rows.length) {
    c.warnings.push('the table has no rows to chart');
    return c;
  }

  let x = 0;
  const wantX = String(n.args.get('x') ?? '').trim();
  if (wantX) {
    const i = columnIndex(table.header, wantX);
    if (i >= 0) x = i;
    else c.warnings.push(`no column named "${wantX}"; labelling with "${headerAt(table, 0)}"`);
  }

  const ys = valueColumns(table, x, n.args.get('y'), c);
  if (!ys.length) {
    c.warnings.push('no numeric column to chart');
    return c;
  }

  for (const i of ys) c.series.push({ name: headerAt(table, i), values: [] });
  for (const row of table.rows) {
    c.labels.push(cellAt(row, x));
    ys.forEach((i, s) => {
      // A cell that is not a number counts as zero, which the renderer can
      // draw. A real gap would mean teaching every SVG builder about absent
      // points; until then the warning is the honest signal.
      const text = cellAt(row, i);
      if (text && !isNumeric(text)) {
        c.warnings.push(`"${text}" in column "${headerAt(table, i)}" is not a number; charted as zero`);
      }
      c.series[s].values.push(toNumber(text));
    });
  }

  if (c.series.length > MAX_SERIES) {
    const other = { name: 'Other', values: new Array(c.labels.length).fill(0) };
    for (const s of c.series.slice(MAX_SERIES)) s.values.forEach((v, i) => { other.values[i] += v; });
    c.series = [...c.series.slice(0, MAX_SERIES), other];
    c.warnings.push('chart has more series than the palette validates; the extras were folded into "Other"');
  }
  return c;
}

/**
 * Resolves `y=` to column indexes, defaulting to every column other than the
 * labels that holds a number anywhere.
 */
function valueColumns(table, x, want, c) {
  const named = String(want ?? '').trim();
  if (named) {
    const out = [];
    for (const raw of named.split(',')) {
      const name = raw.trim();
      if (!name) continue;
      const i = columnIndex(table.header, name);
      if (i >= 0) out.push(i);
      else c.warnings.push(`no column named "${name}"`);
    }
    return out;
  }
  return table.header
    .map((_, i) => i)
    .filter((i) => i !== x && table.rows.some((row) => isNumeric(cellAt(row, i))));
}

/**
 * Serialises a chart for the interactive runtime to read back out of the DOM.
 * Key order and the omission of empty title/unit match the Go renderer exactly,
 * so the same document produces the same attribute from either.
 */
export function chartData(c) {
  const payload = { type: c.type };
  if (c.title) payload.title = c.title;
  if (c.unit) payload.unit = c.unit;
  payload.labels = c.labels;
  payload.series = c.series.map((s) => ({ name: s.name, values: s.values }));
  return JSON.stringify(payload);
}

/** The chart a `@table` asked for, above the table itself. */
export function renderTableChart(n, table) {
  const c = tableChart(n, table);
  if (!c) return '';
  let out = '';
  const svg = renderChartSVG(c);
  if (svg) {
    const caption = c.title ? `<figcaption>${esc(c.title)}</figcaption>` : '';
    out = `<figure class="chart" data-xtxt-chart="${esc(chartData(c))}">${svg}${caption}</figure>`;
  }
  // Shown rather than swallowed: these do not reach validate(), so this is the
  // only way an author learns that x="Moth" matched no column.
  return out + c.warnings.map((w) => `<p class="chart-warning">${esc(w)}</p>`).join('');
}

const chartLabel = (c) => c.title || `${c.type} chart of ${c.labels.join(', ')}`;
const svgOpen = (w, h, label) => `<svg class="xtxt-chart" viewBox="0 0 ${w} ${h}" width="100%" `
  + `height="${h}" role="img" aria-label="${esc(label)}" xmlns="http://www.w3.org/2000/svg">`;
const suffix = (name) => (name ? ` · ${name}` : '');

/** A bar with its data end rounded and its baseline end square. */
function barPath(x, y, w, h, r) {
  if (w < r * 2) r = w / 2;
  if (r < 0) r = 0;
  return `M${x.toFixed(1)} ${y.toFixed(1)} H${(x + w - r).toFixed(1)} `
    + `a${r.toFixed(1)} ${r.toFixed(1)} 0 0 1 ${r.toFixed(1)} ${r.toFixed(1)} `
    + `V${(y + h - r).toFixed(1)} a${r.toFixed(1)} ${r.toFixed(1)} 0 0 1 ${(-r).toFixed(1)} ${r.toFixed(1)} `
    + `H${x.toFixed(1)} Z`;
}

function legend(c, y) {
  let out = '';
  let x = LABEL_COL;
  c.series.forEach((s, i) => {
    const name = s.name || `Series ${i + 1}`;
    out += `<rect x="${x.toFixed(1)}" y="${(y - 9).toFixed(1)}" width="9" height="9" rx="2" fill="${seriesColor(i)}"/>`;
    out += `<text class="c-label" x="${(x + 14).toFixed(1)}" y="${y.toFixed(1)}">${esc(name)}</text>`;
    x += 24 + name.length * 7;
  });
  return out;
}

function barSVG(c) {
  const n = c.labels.length;
  const groups = c.series.length;
  const each = groups > 1 ? Math.floor((ROW_H - 8) / groups) : BAR_H;
  let h = PAD_TOP + n * ROW_H + PAD_BOTTOM;
  if (groups > 1) h += 22;

  const max = Math.max(1, ...c.series.flatMap((s) => s.values));
  const plot = CHART_W - LABEL_COL - 64;

  let out = svgOpen(CHART_W, h, chartLabel(c));
  c.labels.forEach((label, i) => {
    const top = PAD_TOP + i * ROW_H;
    out += `<text class="c-label" x="${LABEL_COL - 10}" y="${(top + ROW_H / 2 + 4).toFixed(1)}" `
      + `text-anchor="end">${esc(label)}</text>`;
    c.series.forEach((s, si) => {
      const w = plot * s.values[i] / max;
      const y = top + (ROW_H - each * groups) / 2 + si * each;
      const gap = groups > 1 ? 2 : 0;
      out += `<path data-index="${i}" data-series="${si}" d="${barPath(LABEL_COL, y, w, each - gap, 4)}" fill="${seriesColor(si)}">`
        + `<title>${esc(label)}${esc(suffix(s.name))}: ${esc(formatNumber(s.values[i], c.unit))}</title></path>`;
      if (groups === 1) {
        out += `<text class="c-value" data-series="${si}" x="${(LABEL_COL + w + 8).toFixed(1)}" `
          + `y="${(y + each / 2 + 4).toFixed(1)}">${esc(formatNumber(s.values[i], c.unit))}</text>`;
      }
    });
  });
  if (groups > 1) out += legend(c, PAD_TOP + n * ROW_H + 16);
  return out + '</svg>';
}

function proportionSVG(c) {
  const values = c.series[0].values;
  const total = values.reduce((a, b) => a + b, 0);
  if (total <= 0) return '';
  const barTop = 16, thick = 34;
  const h = barTop + thick + 30 + 22 * Math.ceil((c.labels.length + 2) / 3);

  let out = svgOpen(CHART_W, h, chartLabel(c));
  let x = 0;
  values.forEach((v, i) => {
    const w = CHART_W * v / total;
    out += `<rect data-index="${i}" x="${x.toFixed(1)}" y="${barTop}" width="${Math.max(w - 2, 0).toFixed(1)}" `
      + `height="${thick}" fill="${seriesColor(i)}" rx="2"><title>${esc(c.labels[i])}: `
      + `${esc(formatNumber(v, c.unit))} (${(v / total * 100).toFixed(1)}%)</title></rect>`;
    if (w > 46) {
      out += `<text class="c-inbar" x="${(x + w / 2 - 1).toFixed(1)}" y="${barTop + thick / 2 + 4}" `
        + `text-anchor="middle">${Math.round(v / total * 100)}%</text>`;
    }
    x += w;
  });
  c.labels.forEach((label, i) => {
    const lx = (i % 3) * CHART_W / 3;
    const ly = barTop + thick + 24 + Math.floor(i / 3) * 22;
    out += `<rect x="${lx.toFixed(1)}" y="${(ly - 9).toFixed(1)}" width="9" height="9" rx="2" fill="${seriesColor(i)}"/>`;
    out += `<text class="c-label" x="${(lx + 14).toFixed(1)}" y="${ly.toFixed(1)}">`
      + `${esc(label)} ${esc(formatNumber(values[i], c.unit))}</text>`;
  });
  return out + '</svg>';
}

function lineSVG(c) {
  const h = 260, top = 18, bottom = 46, left = 44;
  const plotW = CHART_W - left - 20;
  const plotH = h - top - bottom;
  const all = c.series.flatMap((s) => s.values);
  let max = Math.max(0, ...all);
  const min = Math.min(0, ...all);
  if (max === min) max = min + 1;

  const X = (i) => (c.labels.length === 1 ? left + plotW / 2
    : left + plotW * i / (c.labels.length - 1));
  const Y = (v) => top + plotH * (1 - (v - min) / (max - min));

  let out = svgOpen(CHART_W, h, chartLabel(c));
  for (const frac of [0, 0.5, 1]) {
    const gy = top + plotH * frac;
    out += `<line class="c-grid" x1="${left}" y1="${gy.toFixed(1)}" x2="${left + plotW}" y2="${gy.toFixed(1)}"/>`;
    out += `<text class="c-axis" x="${left - 8}" y="${(gy + 4).toFixed(1)}" text-anchor="end">`
      + `${esc(formatNumber(min + (max - min) * (1 - frac)))}</text>`;
  }
  c.series.forEach((s, si) => {
    const pts = s.values.map((v, i) => `${X(i).toFixed(1)},${Y(v).toFixed(1)}`).join(' ');
    if (c.type === 'area') {
      out += `<polygon data-series="${si}" points="${X(0).toFixed(1)},${(top + plotH).toFixed(1)} ${pts} `
        + `${X(s.values.length - 1).toFixed(1)},${(top + plotH).toFixed(1)}" `
        + `fill="${seriesColor(si)}" opacity="0.14"/>`;
    }
    out += `<polyline data-series="${si}" points="${pts}" fill="none" stroke="${seriesColor(si)}" stroke-width="2" `
      + 'stroke-linejoin="round" stroke-linecap="round"/>';
    s.values.forEach((v, i) => {
      out += `<circle data-index="${i}" data-series="${si}" cx="${X(i).toFixed(1)}" cy="${Y(v).toFixed(1)}" r="4" fill="${seriesColor(si)}" `
        + `stroke="var(--chart-surface)" stroke-width="2"><title>${esc(c.labels[i])}`
        + `${esc(suffix(s.name))}: ${esc(formatNumber(v, c.unit))}</title></circle>`;
    });
    // Ends and peak only — never every point, and deduplicated so an endpoint
    // that is also the peak is not drawn twice on top of itself.
    const last = s.values.length - 1;
    const peak = s.values.indexOf(Math.max(...s.values));
    for (const i of [...new Set([0, peak, last])]) {
      const anchor = i === 0 ? 'start' : i === last ? 'end' : 'middle';
      const dx = i === 0 ? 2 : i === last ? -2 : 0;
      out += `<text class="c-value" x="${(X(i) + dx).toFixed(1)}" y="${(Y(s.values[i]) - 10).toFixed(1)}" `
        + `text-anchor="${anchor}">${esc(formatNumber(s.values[i], c.unit))}</text>`;
    }
  });
  c.labels.forEach((label, i) => {
    if (c.labels.length > 12 && i % Math.floor(c.labels.length / 8) !== 0) return;
    out += `<text class="c-axis" x="${X(i).toFixed(1)}" y="${top + plotH + 18}" `
      + `text-anchor="middle">${esc(label)}</text>`;
  });
  if (c.series.length > 1) out += legend(c, h - 14);
  return out + '</svg>';
}

/** Draw a chart as script-free inline SVG. */
export function renderChartSVG(c) {
  if (!c.labels.length || !c.series.length) return '';
  if (c.type === 'line' || c.type === 'area') return lineSVG(c);
  if (['pie', 'donut', 'proportion', 'stacked'].includes(c.type)) return proportionSVG(c);
  return barSVG(c);
}

/**
 * The text alternative, which is mandatory rather than a nicety: part of the
 * palette sits below 3:1 against the light surface, and print or forced-colours
 * modes may drop the fills entirely.
 */
export function chartTableHTML(c) {
  const head = c.series.map((s, i) => {
    const name = s.name || (c.series.length > 1 ? `Series ${i + 1}` : 'Value');
    return `<th style="text-align:right">${esc(name)}</th>`;
  }).join('');
  const rows = c.labels.map((label, i) => `<tr><td>${esc(label)}</td>`
    + c.series.map((s) => `<td style="text-align:right">${esc(formatNumber(s.values[i], c.unit))}</td>`).join('')
    + '</tr>').join('\n');
  return `<details class="chart-data"><summary>Data</summary><table>\n`
    + `<thead><tr><th></th>${head}</tr></thead>\n<tbody>\n${rows}\n</tbody>\n</table></details>\n`;
}

/** Full `@chart` figure: SVG, optional caption, and the table view. */
export function renderChart(n) {
  const c = parseChart(n);
  const svg = renderChartSVG(c);
  if (!svg) return '';
  const caption = c.title ? `<figcaption>${esc(c.title)}</figcaption>` : '';
  return `<figure class="chart">${svg}${caption}${chartTableHTML(c)}</figure>`;
}
