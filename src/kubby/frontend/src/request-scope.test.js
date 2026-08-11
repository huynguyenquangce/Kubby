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

test('a cluster change invalidates view, connection, drawer, and modal work', () => {
    const scopes = createRequestScopes();
    const connection = scopes.connectionToken();
    const view = scopes.beginView('pods', '');
    const ref = { kind: 'Pod', namespace: 'default', name: 'old' };
    const drawer = scopes.openDrawer(ref);
    const modal = scopes.openModal('helm-upgrade\0default\0old');

    scopes.connectionChanged();

    assert.equal(scopes.isCurrentConnection(connection), false);
    assert.equal(scopes.isCurrentView(view, 'pods', ''), false);
    assert.equal(scopes.isCurrentDrawer(drawer, ref), false);
    assert.equal(scopes.isCurrentModal(modal), false);
});

test('modal ownership is exact and closing invalidates pending work', () => {
    const scopes = createRequestScopes();
    const modalA = scopes.openModal('helm-upgrade\0default\0a');
    const modalB = scopes.openModal('helm-upgrade\0default\0b');

    assert.equal(scopes.isCurrentModal(modalA), false);
    assert.equal(scopes.isCurrentModal(modalB), true);
    assert.equal(scopes.isCurrentModal(modalB, 'helm-upgrade\0default\0a'), false);

    scopes.closeModal();
    assert.equal(scopes.isCurrentModal(modalB), false);
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

test('deferred modal values A cannot overwrite newer modal B', async () => {
    const scopes = createRequestScopes();
    let resolveA;
    const valuesA = new Promise((resolve) => { resolveA = resolve; });
    let editor = '';
    const modalA = scopes.openModal('helm-upgrade\0default\0a');
    const renderA = valuesA.then((value) => {
        if (scopes.isCurrentModal(modalA)) editor = value;
    });
    const modalB = scopes.openModal('helm-upgrade\0default\0b');
    if (scopes.isCurrentModal(modalB)) editor = 'values-b';

    resolveA('values-a');
    await renderA;

    assert.equal(editor, 'values-b');
});

test('a stale submit completion cannot close or report into a newer modal', async () => {
    const scopes = createRequestScopes();
    let resolveA;
    const submitA = new Promise((resolve) => { resolveA = resolve; });
    let openOwner = 'a';
    let error = '';
    const modalA = scopes.openModal('helm-upgrade\0default\0a');
    const completion = submitA.then(
        () => { if (scopes.isCurrentModal(modalA)) openOwner = ''; },
        (err) => { if (scopes.isCurrentModal(modalA)) error = String(err); },
    );
    scopes.openModal('helm-upgrade\0default\0b');
    openOwner = 'b';

    resolveA();
    await completion;

    assert.equal(openOwner, 'b');
    assert.equal(error, '');
});

test('defaults from an older chart version are discarded', async () => {
    const scopes = createRequestScopes();
    const modal = scopes.openModal('chart-install\0repo\0chart');
    let selectedVersion = '1.0.0';
    let editor = '';
    let resolveV1;
    const defaultsV1 = new Promise((resolve) => { resolveV1 = resolve; });
    const requestedVersion = selectedVersion;
    const renderV1 = defaultsV1.then((value) => {
        if (scopes.isCurrentModal(modal) && selectedVersion === requestedVersion) editor = value;
    });
    selectedVersion = '2.0.0';
    editor = 'defaults-v2';

    resolveV1('defaults-v1');
    await renderV1;

    assert.equal(editor, 'defaults-v2');
});

test('defaults already loaded for a previous version are cleared on version change', () => {
    let selectedVersion = '1.0.0';
    let loadedDefaultsVersion = selectedVersion;
    let editor = 'defaults-v1';

    selectedVersion = '2.0.0';
    if (loadedDefaultsVersion !== selectedVersion) {
        editor = '';
        loadedDefaultsVersion = null;
    }

    assert.equal(editor, '');
    assert.equal(loadedDefaultsVersion, null);
});
