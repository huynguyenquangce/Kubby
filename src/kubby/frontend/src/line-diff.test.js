import assert from 'node:assert/strict';
import { performance } from 'node:perf_hooks';
import test from 'node:test';

import { lineDiff } from './line-diff.js';

function reconstructed(diff, type) {
    return diff.filter((part) => part.t === ' ' || part.t === type).map((part) => part.l).join('\n');
}

function assertTransforms(before, after, options) {
    const diff = lineDiff(before, after, options);
    assert.equal(reconstructed(diff, '-'), before);
    assert.equal(reconstructed(diff, '+'), after);
    return diff;
}

test('line diff preserves identical input as context', () => {
    assert.deepEqual(lineDiff('a\nb', 'a\nb'), [{ t: ' ', l: 'a' }, { t: ' ', l: 'b' }]);
});

test('line diff reconstructs insertions, deletions and repeated lines', () => {
    assertTransforms('a\nrepeat\nold\nrepeat\nz', 'a\nnew\nrepeat\nrepeat\nz');
    assertTransforms('', 'created');
    assertTransforms('deleted', '');
});

test('pathological input falls back to a bounded valid replacement', () => {
    const before = Array.from({ length: 10_000 }, (_, i) => `old-${i}`).join('\n');
    const after = Array.from({ length: 10_000 }, (_, i) => `new-${i}`).join('\n');
    const diff = assertTransforms(before, after, { maxWork: 1000 });
    assert.equal(diff.length, 20_000);
});

test('ten-thousand-line small diff stays inside the regression budget', () => {
    const lines = Array.from({ length: 10_000 }, (_, i) => `line-${i}`);
    const changed = [...lines];
    changed[5000] = 'line-5000-updated';
    const started = performance.now();
    const diff = assertTransforms(lines.join('\n'), changed.join('\n'));
    const elapsed = performance.now() - started;
    assert.equal(diff.length, 10_001);
    assert.ok(elapsed < 1000, `10k-line diff took ${elapsed.toFixed(1)}ms`);
});
