import assert from 'node:assert/strict';
import test from 'node:test';
import { tableSortValue } from './table-sort.js';

test('table sorting preserves quantity units, durations, readiness fractions and resource names', () => {
    assert.ok(tableSortValue('12h', 'Age') < tableSortValue('2d', 'Age'));
    assert.ok(tableSortValue('45s', 'Age') < tableSortValue('5m', 'Age'));
    assert.equal(tableSortValue('1h30m', 'Age'), 5400);
    assert.ok(tableSortValue('900Mi', 'Memory') < tableSortValue('1Gi', 'Memory'));
    assert.ok(tableSortValue('900m', 'CPU') < tableSortValue('1', 'CPU'));
    assert.ok(tableSortValue('1/2', 'Ready') < tableSortValue('1/1', 'Ready'));
    assert.equal(tableSortValue('123-api', 'Name'), '123-api');
});
