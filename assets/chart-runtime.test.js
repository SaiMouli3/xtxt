import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import { test } from 'node:test';

// The runtime exports its pure half under CommonJS when there is no document,
// which is exactly the half worth testing.
const { visibleSeries, readout, formatValue, canHide } =
  createRequire(import.meta.url)('./chart-runtime.js');

const CHART = {
  type: 'bar',
  unit: 'users',
  labels: ['Jan', 'Feb'],
  series: [
    { name: 'Signups', values: [20, 35] },
    { name: 'Churn', values: [3, 4.5] },
  ],
};

test('the readout names every visible series for one category', () => {
  assert.equal(readout(CHART, 0, [false, false]), 'Jan: Signups 20 users, Churn 3 users');
});

test('a hidden series is absent from the readout, not zero', () => {
  assert.equal(readout(CHART, 1, [false, true]), 'Feb: Signups 35 users');
  assert.doesNotMatch(readout(CHART, 1, [false, true]), /Churn/);
});

test('a category with everything hidden still names itself', () => {
  assert.equal(readout(CHART, 0, [true, true]), 'Jan');
});

test('values lose float noise but keep real decimals', () => {
  assert.equal(formatValue(20, ''), '20');
  assert.equal(formatValue(4.5, ''), '4.50');
  assert.equal(formatValue(20, 'users'), '20 users');
  assert.equal(formatValue(undefined, 'users'), '—');
});

test('the last visible series cannot be hidden', () => {
  // Hiding everything is not a view of the data, it is an absence of one.
  assert.equal(canHide(CHART.series, [false, true], 0), false);
  assert.equal(canHide(CHART.series, [false, false], 0), true);
  // Re-showing an already hidden series is always allowed.
  assert.equal(canHide(CHART.series, [true, false], 0), true);
});

test('visibleSeries drops exactly the hidden ones', () => {
  assert.deepEqual(visibleSeries(CHART.series, [false, true]).map((s) => s.name), ['Signups']);
  assert.equal(visibleSeries(CHART.series, [false, false]).length, 2);
});
