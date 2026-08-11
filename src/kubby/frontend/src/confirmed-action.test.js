import assert from 'node:assert/strict';
import test from 'node:test';

import { confirmedAction, summarizeLineChanges } from './confirmed-action.js';

test('a cancelled confirmation never invokes the cluster write', async () => {
    let writes = 0;
    const result = await confirmedAction(
        async () => false,
        async () => { writes++; },
    );

    assert.equal(result, false);
    assert.equal(writes, 0);
});

test('an approved confirmation invokes the cluster write exactly once', async () => {
    let writes = 0;
    const result = await confirmedAction(
        async () => true,
        async () => { writes++; },
    );

    assert.equal(result, true);
    assert.equal(writes, 1);
});

test('YAML confirmation summarizes added and removed lines', () => {
    assert.deepEqual(summarizeLineChanges([
        { t: ' ', l: 'same' },
        { t: '-', l: 'old' },
        { t: '+', l: 'new' },
        { t: '+', l: 'another' },
    ]), { added: 2, removed: 1 });
});
