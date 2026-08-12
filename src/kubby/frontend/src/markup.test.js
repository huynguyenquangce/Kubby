import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const indexHTML = readFileSync(new URL('../index.html', import.meta.url), 'utf8');
const mainJS = readFileSync(new URL('./main.js', import.meta.url), 'utf8');

test('AI settings uses the bounded icon-button SVG class', () => {
    const button = indexHTML.match(/<button id="btn-ai-settings"[\s\S]*?<\/button>/)?.[0] ?? '';
    assert.match(button, /<svg class="btn-ico">/);
    assert.doesNotMatch(button, /<svg class="ico">/);
});

test('modal dismiss controls do not pass MouseEvent as a modal scope', () => {
    for (const id of ['modal-cancel', 'modal-close', 'modal-backdrop']) {
        const escapedID = id.replace('-', '\\-');
        assert.match(
            mainJS,
            new RegExp(`\\$\\('${escapedID}'\\)\\.addEventListener\\('click', \\(\\) => closeModal\\(\\)\\);`),
        );
    }
    assert.doesNotMatch(mainJS, /addEventListener\('click', closeModal\)/);
});

test('port-forward manager exposes global and drawer lifecycle controls', () => {
    for (const id of ['btn-port-forwards', 'pf-manager', 'pf-global-list', 'pf-stop-all', 'pf-keep-running', 'pf-toast']) {
        assert.match(indexHTML, new RegExp(`id="${id}"`));
    }
    assert.match(mainJS, /ListPortForwards\(\)/);
    assert.match(mainJS, /StartPortForward\(operationID, ref\.kind, ref\.namespace, ref\.name, local, remote, keepRunning\)/);
    assert.match(mainJS, /CancelPortForwardStart\(pending\.id\)/);
});

test('terminal uses a PTY emulator instead of a line-mode command input', () => {
    for (const id of ['term-surface', 'term-container', 'term-shell', 'btn-term-start', 'btn-term-stop']) {
        assert.match(indexHTML, new RegExp(`id="${id}"`));
    }
    assert.doesNotMatch(indexHTML, /id="term-input"/);
    assert.match(mainJS, /from '@xterm\/xterm'/);
    assert.match(mainJS, /from '@xterm\/addon-fit'/);
    assert.match(mainJS, /ExecResize/);
});

test('overview uses bundled typography and dashboard-specific table contracts', () => {
    assert.match(mainJS, /@fontsource-variable\/inter\/wght\.css/);
    const overview = indexHTML.match(/<section id="view-overview"[\s\S]*?<section id="view-nodes"/)?.[0] ?? '';
    assert.match(overview, /class="overview-pulse"/);
    assert.match(overview, /class="overview-section-head"/);
    const tables = [...overview.matchAll(/<table([^>]*)>/g)];
    assert.ok(tables.length >= 3);
    for (const table of tables) assert.match(table[1], /class="[^"]*plain[^"]*"/);
});

test('overview loads through one snapshot binding', () => {
    const loader = mainJS.match(/function loadOverview\(scope\) \{[\s\S]*?\n\}/)?.[0] ?? '';
    assert.match(loader, /return OverviewSnapshot\(\)/);
    assert.doesNotMatch(loader, /Promise\.all|\bListNodes\(|\bListNamespaces\(|\bListPods\(|\bListDeployments\(|\bNodeMetrics\(|\bTopPods\(|\bClusterEvents\(/);
});

test('overview avoids duplicate node usage and exposes a one-call cluster structure', () => {
    const overview = indexHTML.match(/<section id="view-overview"[\s\S]*?<section id="view-structure"/)?.[0] ?? '';
    assert.match(overview, /<h3>Node status<\/h3>/);
    assert.doesNotMatch(overview, /<h3>Node usage<\/h3>|id="node-metrics"/);
    for (const id of ['btn-cluster-structure', 'view-structure', 'structure-filter', 'structure-only-unhealthy', 'structure-inspector']) {
        assert.match(indexHTML, new RegExp(`id="${id}"`));
    }
    const loader = mainJS.match(/function loadClusterStructure\(scope\) \{[\s\S]*?\n\}/)?.[0] ?? '';
    assert.match(loader, /return ClusterStructure\(scope\.namespace \|\| ''\)/);
    assert.doesNotMatch(loader, /Promise\.all|\bListServices\(|\bListPods\(|\bListIngresses\(/);
    const clear = mainJS.match(/function clearRenderedView\(view = currentView\) \{[\s\S]*?\n\}/)?.[0] ?? '';
    assert.match(clear, /view === 'structure'/);
    assert.match(clear, /structure-entries/);
    assert.match(clear, /resetStructureInspector\(\)/);
});

test('live logs use owned batches and a bounded buffer', () => {
    assert.match(mainJS, /EventsOn\('loglines'/);
    assert.match(mainJS, /new LineRingBuffer\(5000\)/);
    assert.match(mainJS, /batch\.streamId !== activeLogStreamID/);
    assert.doesNotMatch(mainJS, /EventsOn\('logline'/);
});

test('previously direct cluster writes cross the confirmation boundary', () => {
    for (const binding of ['SetDeploymentPaused', 'SetNodeSchedulable', 'UpdateYAML', 'HelmTest']) {
        const call = mainJS.lastIndexOf(`${binding}(`);
        assert.notEqual(call, -1, `${binding} should remain wired`);
        const nearby = mainJS.slice(Math.max(0, call - 1600), call + 500);
        assert.match(nearby, /confirmedAction\(/, `${binding} must stay behind explicit confirmation`);
    }
});

test('Helm install requires the digest from an exact successful preview', () => {
    assert.match(mainJS, /approvedPreview\.digest/);
    assert.match(mainJS, /diff\.chartDigest/);
    assert.match(mainJS, /Preview this exact release, namespace, version, and values before installing/);
});
