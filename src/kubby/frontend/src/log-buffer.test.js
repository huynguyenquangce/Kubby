import assert from 'node:assert/strict';
import test from 'node:test';

import { createFrameScheduler, IncrementalLogView, LineRingBuffer } from './log-buffer.js';

function fakeTextView() {
    const view = {
        nodes: [],
        rawText: '',
        appendChild(node) {
            this.rawText = '';
            this.nodes.push(node);
        },
        removeChild(node) {
            this.nodes.splice(this.nodes.indexOf(node), 1);
        },
        get textContent() {
            return this.nodes.length > 0 ? this.nodes.map((node) => node.data).join('') : this.rawText;
        },
        set textContent(value) {
            this.nodes = [];
            this.rawText = String(value);
        },
    };
    return view;
}

test('ring buffer retains only the newest lines in order', () => {
    const buffer = new LineRingBuffer(5);
    buffer.pushMany(['0', '1', '2']);
    buffer.pushMany(['3', '4', '5', '6']);
    assert.deepEqual(buffer.toArray(), ['2', '3', '4', '5', '6']);
});

test('large streams stay bounded without slicing the accumulated array', () => {
    const buffer = new LineRingBuffer(5000);
    for (let offset = 0; offset < 20_000; offset += 64) {
        buffer.pushMany(Array.from({ length: 64 }, (_, i) => `line-${offset + i}`));
    }
    const lines = buffer.toArray();
    assert.equal(lines.length, 5000);
    assert.equal(lines.at(-1), 'line-20031');
    assert.equal(lines[0], 'line-15032');
});

test('live view appends chunks without replacing retained text', () => {
    const view = fakeTextView();
    const logs = new IncrementalLogView(view, 5, (text) => ({ data: text }));
    logs.replace(['a', 'b']);
    const retained = view.nodes[0];

    logs.append(['c', 'd']);

    assert.equal(view.nodes[0], retained);
    assert.equal(view.nodes.length, 2);
    assert.equal(view.textContent, 'a\nb\nc\nd');
    assert.equal(logs.lineCount, 4);
});

test('live view trims only oldest lines at capacity', () => {
    const view = fakeTextView();
    const logs = new IncrementalLogView(view, 5, (text) => ({ data: text }));
    logs.replace(['a', 'b', 'c']);
    logs.append(['d', 'e']);
    logs.append(['f', 'g']);

    assert.equal(view.textContent, 'c\nd\ne\nf\ng');
    assert.equal(logs.lineCount, 5);

    logs.append(['h', 'i', 'j', 'k', 'l', 'm']);
    assert.equal(view.textContent, 'i\nj\nk\nl\nm');
    assert.equal(logs.lineCount, 5);
});

test('live view replaces an empty stream with its first batch', () => {
    const view = fakeTextView();
    const logs = new IncrementalLogView(view, 5, (text) => ({ data: text }));
    logs.replace([]);
    assert.equal(view.textContent, '(no logs)');

    logs.append(['first']);
    assert.equal(view.textContent, 'first');
});

test('frame scheduler coalesces many batches into one render', () => {
    const callbacks = new Map();
    let nextHandle = 0;
    let renders = 0;
    const scheduler = createFrameScheduler(
        () => { renders++; },
        (cb) => { callbacks.set(++nextHandle, cb); return nextHandle; },
        (handle) => callbacks.delete(handle),
    );

    for (let i = 0; i < 1000; i++) scheduler.request();
    assert.equal(callbacks.size, 1);
    callbacks.get(1)();
    assert.equal(renders, 1);
});

test('flush cancels a scheduled frame and renders immediately', () => {
    const callbacks = new Map();
    let renders = 0;
    const scheduler = createFrameScheduler(
        () => { renders++; },
        (cb) => { callbacks.set(1, cb); return 1; },
        (handle) => callbacks.delete(handle),
    );
    scheduler.request();
    scheduler.flush();
    assert.equal(callbacks.size, 0);
    assert.equal(renders, 1);
});
