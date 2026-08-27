import assert from 'node:assert/strict';
import test from 'node:test';

import { overviewHealthModel } from './overview-health.js';

const readyNode = { name: 'worker-a', ready: true, pressure: [] };

test('healthy Pods and Nodes produce a truthful cluster-green state', () => {
    assert.deepEqual(overviewHealthModel({ total: 3, errored: [], nodes: [readyNode] }), {
        tone: 'ok',
        icon: '✓',
        title: 'Cluster healthy',
        summary: 'All workloads and Nodes are ready. No active Pod failures were reported.',
        ratio: '3 / 3',
    });
});

test('a NotReady or pressured Node prevents a false-green cluster banner', () => {
    for (const node of [
        { name: 'worker-a', ready: false, pressure: [] },
        { name: 'worker-a', ready: true, pressure: ['MemoryPressure'] },
    ]) {
        const model = overviewHealthModel({ total: 3, errored: [], nodes: [node] });
        assert.equal(model.tone, 'warning');
        assert.equal(model.title, '1 node needs attention');
        assert.match(model.summary, /infrastructure is degraded/);
    }
});

test('unavailable Node health does not claim the whole cluster is healthy', () => {
    const model = overviewHealthModel({ total: 3, errored: [], nodes: null });
    assert.equal(model.tone, 'warning');
    assert.equal(model.title, 'Workloads healthy');
    assert.match(model.summary, /Node health could not be loaded/);
});

test('Pod failures remain the primary actionable health state', () => {
    const model = overviewHealthModel({
        total: 10,
        errored: [{ name: 'api' }],
        nodes: [{ name: 'worker-a', ready: false, pressure: [] }],
    });
    assert.equal(model.tone, 'warning');
    assert.equal(model.title, '1 pod needs attention');
    assert.equal(model.ratio, '9 / 10');
    assert.match(model.summary, /Workload and infrastructure health are degraded/);
});

test('missing Pod data remains unavailable regardless of Node state', () => {
    const model = overviewHealthModel({ total: null, errored: [], nodes: [readyNode] });
    assert.equal(model.tone, 'unavailable');
    assert.equal(model.ratio, '– / –');
});
