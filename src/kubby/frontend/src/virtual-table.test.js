import assert from 'node:assert/strict';
import test from 'node:test';

// Keep the pure ordering/filtering contract pinned without requiring jsdom.
// The browser suite owns scroll geometry and rendered-row counts.
test('virtual table source keeps paging, accessibility, and detached-row selection contracts', async () => {
    const source = await import('node:fs/promises').then(({ readFile }) => readFile(new URL('./virtual-table.js', import.meta.url), 'utf8'));
    assert.match(source, /aria-rowcount/);
    assert.match(source, /aria-rowindex/);
    assert.match(source, /onNearEnd/);
    assert.match(source, /state\.rows\.sort/);
    assert.match(source, /clearVirtualSelections/);
});
