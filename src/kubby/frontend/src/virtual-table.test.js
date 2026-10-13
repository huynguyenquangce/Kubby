import assert from 'node:assert/strict';
import test from 'node:test';
import { allTableRows, filteredTableRows, setVirtualRows, appendVirtualRows, filterVirtualRows, sortVirtualRows, clearVirtualRows } from './virtual-table.js';

function fixture() {
    const owner = { scrollTop: 0, clientHeight: 600, getBoundingClientRect: () => ({ top: 0 }), addEventListener() {}, removeEventListener() {} };
    const table = { attributes: {}, setAttribute(k, v) { this.attributes[k] = v; }, removeAttribute(k) { delete this.attributes[k]; }, querySelectorAll: () => [1], focus() { document.activeElement = this; } };
    const body = { children: [], closest: (selector) => selector === '.content' ? owner : table,
        getBoundingClientRect: () => ({ top: -owner.scrollTop }), contains: (node) => body.children.includes(node),
        replaceChildren(fragment) { body.children = fragment?.children ?? []; } };
    globalThis.document = { activeElement: null,
        createDocumentFragment: () => ({ children: [], appendChild(row) { this.children.push(row); } }),
        createElement: () => ({ style: {}, setAttribute() {}, appendChild() {} }) };
    const row = (name) => ({ name, hidden: false, setAttribute() {}, closest: () => body.children.includes(rows.get(name)) ? table : null });
    const rows = new Map();
    return { owner, table, body, row(name) { const result = row(name); rows.set(name, result); return result; } };
}

test('select-all uses filtered logical rows, including matching rows outside the window', () => {
    const f = fixture();
    const rows = Array.from({ length: 200 }, (_, i) => f.row(i % 2 ? `api-${i}` : `other-${i}`));
    setVirtualRows(f.body, rows);
    assert.deepEqual(filterVirtualRows(f.body, (row) => row.name.startsWith('api')), { shown: 100, total: 200 });
    assert.equal(allTableRows(f.body).length, 200);
    assert.equal(filteredTableRows(f.body).length, 100);
    assert.ok(filteredTableRows(f.body).every((row) => row.name.startsWith('api')));
    clearVirtualRows(f.body);
});

test('sort survives append and refresh, and append respects the current filter', () => {
    const f = fixture();
    setVirtualRows(f.body, [f.row('checkout'), f.row('api')]);
    sortVirtualRows(f.body, (a, b) => a.name.localeCompare(b.name));
    filterVirtualRows(f.body, (row) => row.name.startsWith('a'));
    appendVirtualRows(f.body, [f.row('zzz'), f.row('aaa')]);
    assert.deepEqual(filteredTableRows(f.body).map((row) => row.name), ['aaa', 'api']);
    filterVirtualRows(f.body, null);
    assert.deepEqual(filteredTableRows(f.body).map((row) => row.name), ['aaa', 'api', 'checkout', 'zzz']);
    setVirtualRows(f.body, [f.row('checkout'), f.row('api')]);
    assert.deepEqual(allTableRows(f.body).map((row) => row.name), ['api', 'checkout']);
    clearVirtualRows(f.body);
});

test('a focused distant row cannot expand the mounted viewport to all 10000 rows', () => {
    const f = fixture();
    const rows = Array.from({ length: 10000 }, (_, i) => f.row(`row-${i}`));
    setVirtualRows(f.body, rows);
    document.activeElement = { closest: () => rows[0] };
    f.owner.scrollTop = 9990 * 48;
    filterVirtualRows(f.body, null);
    assert.ok(f.body.children.length < 80);
    assert.equal(document.activeElement, f.table);
    assert.equal(f.table.attributes['aria-rowcount'], '10001');
    clearVirtualRows(f.body);
});
