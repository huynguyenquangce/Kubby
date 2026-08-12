import assert from 'node:assert/strict';
import test from 'node:test';

import {
    canApplyForwardHydration,
    forwardsToStopOnDrawerClose,
    isCurrentForwardEvent,
    removeForward,
    shouldCancelPendingForward,
    shouldRetainStartedForward,
    upsertForward,
} from './port-forward-state.js';

const podA = {
    key: 'a', kind: 'Pod', namespace: 'default', name: 'api', podName: 'api-1',
    localPort: 49152, remotePort: 8080, keepRunning: false,
};
const serviceB = {
    key: 'b', kind: 'Service', namespace: 'default', name: 'web', podName: 'web-1',
    localPort: 49153, remotePort: 80, keepRunning: true,
};

test('backend hydration and start completion upsert by tunnel key', () => {
    const updated = { ...podA, localPort: 50000 };
    const list = upsertForward([podA, serviceB], updated);

    assert.equal(list.length, 2);
    assert.equal(list.find((item) => item.key === 'a').localPort, 50000);
    assert.deepEqual(list.find((item) => item.key === 'b'), serviceB);
});

test('a late backend hydration cannot overwrite newer local tunnel state', () => {
    assert.equal(canApplyForwardHydration({ connectionCurrent: true, requestedVersion: 4, currentVersion: 4 }), true);
    assert.equal(canApplyForwardHydration({ connectionCurrent: true, requestedVersion: 4, currentVersion: 5 }), false);
    assert.equal(canApplyForwardHydration({ connectionCurrent: false, requestedVersion: 4, currentVersion: 4 }), false);
});

test('closing a drawer stops only its non-background tunnels', () => {
    const anotherPod = { ...podA, key: 'c', name: 'worker', localPort: 49154 };
    const backgroundForSamePod = { ...podA, key: 'd', keepRunning: true, localPort: 49155 };
    const keys = forwardsToStopOnDrawerClose(
        [podA, serviceB, anotherPod, backgroundForSamePod],
        { kind: 'Pod', namespace: 'default', name: 'api' },
    );

    assert.deepEqual(keys, ['a']);
});

test('a late Start result survives only for a current drawer or background mode', () => {
    assert.equal(shouldRetainStartedForward({ drawerStillOwnsRequest: true, keepRunning: false }), true);
    assert.equal(shouldRetainStartedForward({ drawerStillOwnsRequest: false, keepRunning: true }), true);
    assert.equal(shouldRetainStartedForward({ drawerStillOwnsRequest: false, keepRunning: false }), false);
});

test('drawer close cancels only its pending foreground start', () => {
    const ref = { kind: 'Pod', namespace: 'default', name: 'api' };
    assert.equal(shouldCancelPendingForward({ ...ref, id: 'pf-a', keepRunning: false }, ref), true);
    assert.equal(shouldCancelPendingForward({ ...ref, id: 'pf-a', keepRunning: true }, ref), false);
    assert.equal(shouldCancelPendingForward({ ...ref, name: 'worker', id: 'pf-a', keepRunning: false }, ref), false);
});

test('closed events belong to the exact active connection', () => {
    assert.equal(isCurrentForwardEvent({ connectionId: 'connection-a', key: 'pf-a' }, 'connection-a'), true);
    assert.equal(isCurrentForwardEvent({ connectionId: 'connection-a', key: 'pf-a' }, 'connection-b'), false);
    assert.equal(isCurrentForwardEvent({ connectionId: 'connection-a' }, 'connection-a'), false);
});

test('backend closed events remove the exact tunnel only', () => {
    assert.deepEqual(removeForward([podA, serviceB], 'a'), [serviceB]);
});
