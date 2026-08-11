import assert from 'node:assert/strict';
import test from 'node:test';

import { createSerialWriter, validTerminalSize } from './terminal-io.js';

test('terminal input is delivered in order even when writes resolve out of order', async () => {
    const calls = [];
    let releaseFirst;
    const first = new Promise((resolve) => { releaseFirst = resolve; });
    const write = createSerialWriter(async (data) => {
        calls.push(data);
        if (data === 'a') await first;
    });

    const a = write('a');
    const tab = write('\t');
    const enter = write('\r');
    await Promise.resolve();
    assert.deepEqual(calls, ['a']);
    releaseFirst();
    await Promise.all([a, tab, enter]);

    assert.deepEqual(calls, ['a', '\t', '\r']);
});

test('a failed write does not poison later terminal input', async () => {
    const calls = [];
    const errors = [];
    const write = createSerialWriter(async (data) => {
        calls.push(data);
        if (data === 'bad') throw new Error('closed');
    }, (err) => errors.push(err.message));

    await write('bad');
    await write('next');

    assert.deepEqual(calls, ['bad', 'next']);
    assert.deepEqual(errors, ['closed']);
});

test('terminal size accepts positive integer dimensions only', () => {
    assert.deepEqual(validTerminalSize(120, 36), { cols: 120, rows: 36 });
    assert.equal(validTerminalSize(0, 36), null);
    assert.equal(validTerminalSize(80.5, 24), null);
    assert.equal(validTerminalSize(80, Number.NaN), null);
});
