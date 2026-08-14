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

test('all application modals share the structured header, footer, and focus contract', () => {
    for (const id of ['modal-eyebrow', 'modal-description', 'modal-close', 'modal-foot', 'modal-error']) {
        assert.match(indexHTML, new RegExp(`id="${id}"|class="[^"]*${id}[^"]*"`));
    }
    assert.match(indexHTML, /class="modal-foot-secondary"/);
    assert.match(indexHTML, /class="modal-foot-actions"/);
    assert.match(mainJS, /function trapOverlayFocus\(container, event\)/);
    assert.match(mainJS, /modalReturnFocus/);
    assert.match(mainJS, /dialogReturnFocus/);
    assert.match(mainJS, /danger && cancelText !== null \? cancelBtn/);
});

test('Helm repository modal uses aligned source and optional credential sections', () => {
    const opener = mainJS.match(/function openRepoAddModal\(\) \{[\s\S]*?\n\}/)?.[0] ?? '';
    assert.match(opener, /eyebrow: 'Helm repositories'/);
    assert.doesNotMatch(opener, /wide: true/);
    assert.match(opener, /class="modal-form-grid"/);
    assert.match(opener, /<strong>Authentication<\/strong>/);
    assert.match(opener, /Leave both fields empty for a public repository/);
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
    assert.match(mainJS, /new IncrementalLogView\(\$\('logs-view'\), logLines\.capacity\)/);
    assert.match(mainJS, /batch\.streamId !== activeLogStreamID/);
    assert.doesNotMatch(mainJS, /EventsOn\('logline'/);
});

test('drawer lazily loads tabs and shares one Pod container request', () => {
    const opener = mainJS.match(/function openDrawer\(ref\) \{[\s\S]*?\n\}/)?.[0] ?? '';
    assert.match(opener, /resetDrawerLoads\(scope\)/);
    assert.match(opener, /setDrawerTab\(ref\.tab \?\? 'details'\)/);
    assert.doesNotMatch(opener, /\bloadDetails\(|\bloadYAML\(|\bprepareLogs\(|\bprepareTerminal\(/);

    const tabSetter = mainJS.match(/function setDrawerTab\(name\) \{[\s\S]*?\n\}/)?.[0] ?? '';
    assert.match(tabSetter, /ensureDrawerTabLoaded\(name\)/);
    assert.equal([...mainJS.matchAll(/\bPodContainers\(/g)].length, 1);
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

test('Helm is one workflow-oriented workspace instead of split navigation', () => {
    assert.equal((indexHTML.match(/data-view="helm"/g) ?? []).length, 1);
    assert.doesNotMatch(indexHTML, /data-view="helmrepos"/);
    for (const section of ['releases', 'catalog', 'repositories']) {
        assert.match(indexHTML, new RegExp(`data-helm-section="${section}"`));
        assert.match(indexHTML, new RegExp(`data-helm-panel="${section}"`));
    }
    assert.match(mainJS, /helmCatalogPreferredSource/);
    assert.match(mainJS, /openHelmDetailModal\(ref, initialTab = 'resources'\)/);
});

test('Helm upgrade is disabled until an exact preview owns values and revision', () => {
    assert.match(mainJS, /Values changed — preview again before upgrading/);
    assert.match(mainJS, /approvedPreview\.revision, approvedPreview\.valuesDigest/);
    assert.match(mainJS, /diff\.releaseRevision/);
    assert.match(mainJS, /diff\.valuesDigest/);
    assert.doesNotMatch(mainJS, /HelmUpgradeValues\(ref\.namespace, ref\.name, vals\)\.then/);
});
