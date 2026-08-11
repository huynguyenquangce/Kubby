import assert from 'node:assert/strict';
import test from 'node:test';

import { createRequestScopes, resourceKey } from './request-scope.js';

test('a newer view request invalidates an older response', () => {
    const scopes = createRequestScopes();
    const oldRequest = scopes.beginView('pods', 'team-a');
    const currentRequest = scopes.beginView('pods', 'team-b');

    assert.equal(scopes.isCurrentView(oldRequest, 'pods', 'team-a'), false);
    assert.equal(scopes.isCurrentView(currentRequest, 'pods', 'team-b'), true);
});

test('a cluster change invalidates view, connection, and drawer work', () => {
    const scopes = createRequestScopes();
    const connection = scopes.connectionToken();
    const view = scopes.beginView('pods', '');
    const ref = { kind: 'Pod', namespace: 'default', name: 'old' };
    const drawer = scopes.openDrawer(ref);

    scopes.connectionChanged();

    assert.equal(scopes.isCurrentConnection(connection), false);
    assert.equal(scopes.isCurrentView(view, 'pods', ''), false);
    assert.equal(scopes.isCurrentDrawer(drawer, ref), false);
});

test('drawer ownership is exact and closing invalidates pending work', () => {
    const scopes = createRequestScopes();
    const first = { kind: 'ConfigMap', namespace: 'default', name: 'a' };
    const second = { kind: 'ConfigMap', namespace: 'default', name: 'b' };
    const firstScope = scopes.openDrawer(first);
    const secondScope = scopes.openDrawer(second);

    assert.equal(scopes.isCurrentDrawer(firstScope, first), false);
    assert.equal(scopes.isCurrentDrawer(secondScope, second), true);
    assert.notEqual(scopes.drawerOwnerKey(firstScope), scopes.drawerOwnerKey(secondScope));

    scopes.closeDrawer();
    assert.equal(scopes.isCurrentDrawer(secondScope, second), false);
});

test('resource key includes kind, namespace, and name', () => {
    assert.notEqual(
        resourceKey({ kind: 'Pod', namespace: 'a', name: 'same' }),
        resourceKey({ kind: 'Pod', namespace: 'b', name: 'same' }),
    );
});

test('deferred view response A cannot overwrite newer response B', async () => {
    const scopes = createRequestScopes();
    let resolveA;
    let resolveB;
    const a = new Promise((resolve) => { resolveA = resolve; });
    const b = new Promise((resolve) => { resolveB = resolve; });
    let rendered = '';
    const scopeA = scopes.beginView('pods', 'team-a');
    const renderA = a.then((value) => {
        if (scopes.isCurrentView(scopeA, 'pods', 'team-a')) rendered = value;
    });
    const scopeB = scopes.beginView('pods', 'team-b');
    const renderB = b.then((value) => {
        if (scopes.isCurrentView(scopeB, 'pods', 'team-b')) rendered = value;
    });

    resolveB('B');
    await renderB;
    resolveA('A');
    await renderA;

    assert.equal(rendered, 'B');
});

test('deferred drawer YAML A cannot overwrite newer drawer B', async () => {
    const scopes = createRequestScopes();
    const refA = { kind: 'ConfigMap', namespace: 'default', name: 'a' };
    const refB = { kind: 'ConfigMap', namespace: 'default', name: 'b' };
    let resolveA;
    const a = new Promise((resolve) => { resolveA = resolve; });
    let editor = '';
    const scopeA = scopes.openDrawer(refA);
    const renderA = a.then((value) => {
        if (scopes.isCurrentDrawer(scopeA, refA)) editor = value;
    });
    const scopeB = scopes.openDrawer(refB);
    if (scopes.isCurrentDrawer(scopeB, refB)) editor = 'yaml-b';

    resolveA('yaml-a');
    await renderA;

    assert.equal(editor, 'yaml-b');
});
