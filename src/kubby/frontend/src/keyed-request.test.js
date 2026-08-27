import assert from 'node:assert/strict';
import test from 'node:test';

import { createKeyedRequestOwner } from './keyed-request.js';

test('same-key remount preserves the in-flight owner', () => {
    const owner = createKeyedRequestOwner();
    assert.equal(owner.select('cluster-a/pod/default/api'), true);
    const request = owner.begin('cluster-a/pod/default/api');

    assert.equal(owner.select('cluster-a/pod/default/api'), false);
    assert.equal(owner.isCurrent(request), true);
});

test('selecting another resource or starting another request invalidates old work', () => {
    const owner = createKeyedRequestOwner();
    owner.select('cluster-a/pod/default/api');
    const first = owner.begin('cluster-a/pod/default/api');
    const second = owner.begin('cluster-a/pod/default/api');

    assert.equal(owner.isCurrent(first), false);
    assert.equal(owner.isCurrent(second), true);

    owner.select('cluster-a/pod/default/worker');
    assert.equal(owner.isCurrent(second), false);
});

test('a request cannot start for an unselected key', () => {
    const owner = createKeyedRequestOwner();
    owner.select('cluster-a/pod/default/api');

    assert.throws(() => owner.begin('cluster-b/pod/default/api'), /not selected/);
});
