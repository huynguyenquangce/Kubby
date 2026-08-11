import assert from 'node:assert/strict';
import test from 'node:test';

import { createFrameScheduler, LineRingBuffer } from './log-buffer.js';

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
