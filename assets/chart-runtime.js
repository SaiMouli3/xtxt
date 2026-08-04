/**
 * Upgrades a rendered XTXT chart in place.
 *
 * This is progressive enhancement, not a renderer. The static SVG and the table
 * beside it are the document; if this file never loads, or throws, the reader
 * still has the chart and every number behind it. Nothing here is required for
 * the page to be correct — which is why the export that omits it is the default.
 *
 * It deliberately cannot redraw. Switching chart type in the browser would mean
 * a third implementation of chart drawing beside the Go and JavaScript
 * renderers, and a duplicate renderer is the one thing this codebase keeps
 * refusing to have. Choosing the type is the author's job, at write time, which
 * is what `chart=` and the editor's picker are for. So the two things offered
 * here are exactly the two that need no drawing: reading a value, and hiding a
 * series that is already drawn.
 *
 * Shipped as one asset embedded by both renderers, so there is a single
 * implementation of this behaviour to be right about.
 */
(function () {
  'use strict';

  // ---------------------------------------------------------------------
  // Pure logic — the part worth testing, kept clear of the DOM.
  // ---------------------------------------------------------------------

  /** The series a reader has not switched off. */
  function visibleSeries(series, hidden) {
    return series.filter(function (s, i) { return !hidden[i]; });
  }

  /**
   * What the readout says for one category. A hidden series is left out
   * entirely rather than shown as zero, because zero is a different claim.
   */
  function readout(chart, index, hidden) {
    var parts = [];
    chart.series.forEach(function (s, i) {
      if (hidden[i]) return;
      parts.push((s.name ? s.name + ' ' : '') + formatValue(s.values[index], chart.unit));
    });
    return chart.labels[index] + (parts.length ? ': ' + parts.join(', ') : '');
  }

  /** Trims the noise a float picks up, then appends any unit. */
  function formatValue(v, unit) {
    if (typeof v !== 'number' || !isFinite(v)) return '—';
    var s = Math.abs(v % 1) < 1e-9 ? String(Math.round(v)) : v.toFixed(2);
    return unit ? s + ' ' + unit : s;
  }

  /**
   * Whether this series may be hidden. Never let a reader empty the chart:
   * nothing drawn is not a view of the data, it is an absence of one.
   */
  function canHide(series, hidden, i) {
    return hidden[i] || visibleSeries(series, hidden).length > 1;
  }

  if (typeof module !== 'undefined' && module.exports) {
    module.exports = { visibleSeries: visibleSeries, readout: readout,
      formatValue: formatValue, canHide: canHide };
    return;
  }

  // ---------------------------------------------------------------------
  // DOM glue — thin on purpose, so the logic above carries the weight.
  // ---------------------------------------------------------------------

  function upgrade(figure) {
    var chart;
    try {
      chart = JSON.parse(figure.getAttribute('data-xtxt-chart'));
    } catch (e) {
      return; // A payload we cannot read leaves the static chart untouched.
    }
    var svg = figure.querySelector('svg');
    if (!chart || !chart.labels || !chart.series || !chart.series.length || !svg) return;
    if (figure.getAttribute('data-xtxt-interactive')) return; // already upgraded

    var hidden = chart.series.map(function () { return false; });

    var status = document.createElement('p');
    status.className = 'xtxt-chart-readout';
    // Polite, so a screen reader finishes its sentence before the value
    // under the pointer interrupts it.
    status.setAttribute('aria-live', 'polite');
    figure.appendChild(status);

    each(svg.querySelectorAll('[data-index]'), function (band) {
      band.addEventListener('mouseenter', function () {
        status.textContent = readout(chart, Number(band.getAttribute('data-index')), hidden);
      });
    });
    svg.addEventListener('mouseleave', function () { status.textContent = ''; });

    if (chart.series.length > 1) figure.appendChild(legend(chart, hidden, svg));
    figure.setAttribute('data-xtxt-interactive', 'true');
  }

  function legend(chart, hidden, svg) {
    var box = document.createElement('div');
    box.className = 'xtxt-chart-legend';
    chart.series.forEach(function (s, i) {
      var button = document.createElement('button');
      button.type = 'button';
      button.className = 'xtxt-chart-toggle';
      button.textContent = s.name || 'Series ' + (i + 1);
      button.setAttribute('data-series', String(i));
      button.setAttribute('aria-pressed', 'true');
      button.addEventListener('click', function () {
        if (!canHide(chart.series, hidden, i)) return;
        hidden[i] = !hidden[i];
        button.setAttribute('aria-pressed', String(!hidden[i]));
        each(svg.querySelectorAll('[data-series="' + i + '"]'), function (el) {
          el.style.display = hidden[i] ? 'none' : '';
        });
      });
      box.appendChild(button);
    });
    return box;
  }

  function each(list, fn) {
    Array.prototype.forEach.call(list, fn);
  }

  function start() {
    each(document.querySelectorAll('[data-xtxt-chart]'), upgrade);
  }

  // Exposed so a host that re-renders — the browser demo as you type, an
  // editor preview — can upgrade the new charts without reloading the script.
  // Upgrading twice is harmless: an already-upgraded figure is skipped.
  window.xtxtCharts = start;

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', start);
  } else {
    start();
  }
})();
