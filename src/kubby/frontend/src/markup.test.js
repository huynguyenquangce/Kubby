import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const indexHTML = readFileSync(new URL('../index.html', import.meta.url), 'utf8');
const mainJS = readFileSync(new URL('./main.js', import.meta.url), 'utf8');
const editorJS = readFileSync(new URL('./editor.js', import.meta.url), 'utf8');
const optionBCSS = readFileSync(new URL('./option-b.css', import.meta.url), 'utf8');
const incidentCSS = readFileSync(new URL('./incident.css', import.meta.url), 'utf8');
const appCSS = readFileSync(new URL('./app.css', import.meta.url), 'utf8');

test('namespace scope is searchable without splitting canonical select state', () => {
    for (const id of ['namespace-toggle', 'namespace-search', 'namespace-options', 'namespace-select']) {
        assert.match(indexHTML, new RegExp(`id="${id}"`));
    }
    assert.match(indexHTML, /id="namespace-search"[^>]*role="combobox"/);
    assert.match(indexHTML, /id="namespace-select"[^>]*aria-hidden="true"[^>]*hidden/);
    assert.match(mainJS, /function selectNamespace\(value\)/);
    assert.match(mainJS, /select\.dispatchEvent\(new Event\('change', \{ bubbles: true \}\)\)/);
    assert.match(mainJS, /setActiveNamespaceOption\(namespaceActiveIndex \+ 1\)/);
    assert.match(mainJS, /setActiveNamespaceOption\(namespaceActiveIndex - 1\)/);
});

test('YAML editor owns Ctrl or Cmd+F with a themed top search panel', () => {
    assert.match(editorJS, /search\(\{ top: true \}\)/);
    assert.match(editorJS, /\.\.\.searchKeymap/);
    assert.match(editorJS, /'\.cm-panel\.cm-search'/);
    assert.match(editorJS, /'\.cm-searchMatch\.cm-searchMatch-selected'/);
});

test('dark surfaces preserve semantic action and status variants', () => {
    for (const token of ['surface-raised', 'surface-soft', 'surface-sunken']) {
        assert.match(optionBCSS, new RegExp(`--${token}:`));
    }
    assert.match(optionBCSS, /:root\[data-theme="dark"\] \.btn\.btn-danger/);
    assert.match(optionBCSS, /:root\[data-theme="dark"\] \.btn\.btn-ai/);
    assert.match(optionBCSS, /:root\[data-theme="dark"\] \.chip\.chip-accent/);
    assert.match(optionBCSS, /tbody tr:nth-child\(even\):not\(\.error-row\)/);
});

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

test('late async results are checked against their owning UI surface', () => {
    assert.match(mainJS, /DeleteResourceOwned[\s\S]*?isCurrentDrawerRequest\(scope\)[\s\S]*?closeDrawer\(\)/);
    assert.match(mainJS, /SaveIncidentReport[\s\S]*?incidentReport === report/);
    assert.match(mainJS, /GetAIConfig\(\)[\s\S]*?isCurrentModalRequest\(scope\)[\s\S]*?!settingsDirty/);
    assert.match(mainJS, /valuesTouched[\s\S]*?your draft was kept/);
    assert.match(mainJS, /welcomeContextRequestID[\s\S]*?fillContexts\(res, requestID\)/);
});

test('cluster switcher retains a visible keyboard focus indicator', () => {
    assert.match(appCSS, /\.cluster-select:focus-visible\s*\{[\s\S]*?outline:\s*2px solid/);
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

test('terminal uses a PTY emulator with owned clipboard controls instead of a line-mode command input', () => {
    const terminal = indexHTML.match(/<!-- Terminal panel \(Pod\) -->[\s\S]*?<!-- Port Forward panel/)?.[0] ?? '';
    for (const id of ['term-surface', 'term-container', 'term-shell', 'btn-term-start', 'btn-term-stop', 'btn-term-clear', 'btn-term-copy', 'btn-term-paste']) {
        assert.match(terminal, new RegExp(`id="${id}"`));
    }
    assert.doesNotMatch(terminal, /id="term-input"/);
    assert.match(terminal, /<option value="auto">Auto · Bash first<\/option>/);
    assert.doesNotMatch(terminal, />Connect<\/button>/);
    assert.doesNotMatch(mainJS, /import\s+\{\s*Terminal\s*\}\s+from '@xterm\/xterm'/);
    assert.doesNotMatch(mainJS, /import\s+\{\s*FitAddon\s*\}\s+from '@xterm\/addon-fit'/);
    assert.match(mainJS, /import\('@xterm\/xterm'\)/);
    assert.match(mainJS, /import\('@xterm\/addon-fit'\)/);
    assert.match(mainJS, /ExecResize/);
    assert.match(mainJS, /requestAnimationFrame\(\(\) => startTerminal\(scope\)\)/);
    assert.match(mainJS, /\.then\(\(resolvedShell\) =>/);
    assert.match(mainJS, /ClipboardGetText\(\)/);
    assert.match(mainJS, /CopyToClipboard\(selection\)/);
    assert.match(mainJS, /attachCustomKeyEventHandler/);
});

test('Kubby brand is an accessible Overview navigation control', () => {
    assert.match(indexHTML, /<button id="btn-brand-home"[^>]*aria-label="Go to Overview"/);
    assert.match(mainJS, /btn-brand-home'\)\.addEventListener\('click', \(\) => selectView\('overview'\)\)/);
});

test('command palette trigger stays concise and reserves focus treatment for keyboard navigation', () => {
    assert.match(indexHTML, /id="btn-command-palette"[\s\S]*?aria-label="Search resources, actions, or commands"/);
    assert.match(indexHTML, /class="command-search-label">Search Kubby…<\/span>/);
    assert.match(optionBCSS, /\.command-search:focus-visible/);
    assert.doesNotMatch(optionBCSS, /\.command-search:hover,\s*\n\.command-search:focus\s*\{/);
});

test('overview uses bundled typography and dashboard-specific table contracts', () => {
    assert.match(mainJS, /@fontsource-variable\/inter\/wght\.css/);
    const overview = indexHTML.match(/<section id="view-overview"[\s\S]*?<section id="view-nodes"/)?.[0] ?? '';
    assert.match(overview, /class="overview-health-banner"/);
    assert.match(overview, /class="overview-metrics"/);
    assert.match(overview, /class="overview-section-head"/);
    assert.doesNotMatch(overview, /Cluster pulse|cluster-health-score/);
    const tables = [...overview.matchAll(/<table([^>]*)>/g)];
    assert.ok(tables.length >= 2);
    for (const table of tables) assert.match(table[1], /class="[^"]*plain[^"]*"/);
});

test('resource filtering caches normalized row text between keystrokes', () => {
    assert.match(mainJS, /tr\.dataset\.searchText \?\?= tr\.textContent\.toLowerCase\(\)/);
    assert.match(mainJS, /searchText\.includes\(term\)/);
});

test('the shell has one navigation owner and a mode switcher for topology', () => {
    assert.match(indexHTML, /<aside id="sidebar" class="sidebar">/);
    assert.doesNotMatch(indexHTML, /class="app-rail"|data-rail/);
    assert.doesNotMatch(mainJS, /data-rail/);
    assert.match(indexHTML, /id="topology-switcher"[^>]*role="tablist"/);
    for (const view of ['structure', 'traffic']) {
        const tab = indexHTML.match(new RegExp(`<button[^>]*data-topology-view="${view}"[^>]*>`))?.[0] ?? '';
        assert.match(tab, /role="tab"/);
        assert.match(tab, /aria-selected="false"/);
    }
});

test('resource surfaces expose keyboard and assistive-technology contracts', () => {
    assert.match(indexHTML, /id="drawer"[^>]*role="dialog"[^>]*aria-modal="true"/);
    assert.match(indexHTML, /class="drawer-tabs" role="tablist"/);
    assert.match(indexHTML, /id="palette"[^>]*role="dialog"[^>]*aria-modal="true"/);
    assert.match(indexHTML, /id="palette-list"[^>]*role="listbox"/);
    assert.match(mainJS, /tr\.tabIndex = 0/);
    assert.match(mainJS, /sort-button/);
    assert.match(mainJS, /aria-sort/);
    assert.match(mainJS, /document\.querySelector\('\.main'\)\.inert = true/);
    assert.match(mainJS, /target\?\.isConnected[\s\S]*?target\.focus\(\)/);
});

test('refresh preserves same-scope content and announces freshness', () => {
    assert.match(indexHTML, /id="view-status"[^>]*role="status"[^>]*aria-live="polite"/);
    assert.match(mainJS, /const refreshing = owner === renderedViewOwner/);
    assert.match(mainJS, /if \(!refreshing\) clearRenderedView\(scope\.view\)/);
    assert.match(mainJS, /setViewStatus\(refreshing \? 'refreshing' : 'loading', refreshing \? 'Refreshing…' : 'Loading…'\)/);
    assert.match(mainJS, /setViewStatus\('current', 'Updated just now'\)/);
});

test('overview empty-state icon sizing does not constrain its message', () => {
    assert.match(optionBCSS, /\.overview-empty > span:first-child\s*\{/);
    assert.doesNotMatch(optionBCSS, /\.overview-empty > span\s*\{/);
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

test('pods load through one snapshot binding', () => {
    const loader = mainJS.match(/function loadPods\(scope\) \{[\s\S]*?\n\}/)?.[0] ?? '';
    assert.match(loader, /return PodsSnapshot\(scope\.namespace\)/);
    assert.doesNotMatch(loader, /Promise\.all|\bListPods\(|\bPodMetricsList\(/);
});

test('live refresh is single-flight', () => {
    assert.match(mainJS, /let liveRefreshPending = false/);
    assert.match(mainJS, /if \(liveRefreshPending\) return/);
    assert.match(mainJS, /\.finally\(\(\) => \{ liveRefreshPending = false; \}\)/);
});

test('helm release modal uses one detail and resources snapshot', () => {
    const modal = mainJS.slice(mainJS.indexOf('function openHelmDetailModal'), mainJS.indexOf('function loadHelmHistory'));
    assert.match(modal, /HelmSnapshot\(ref\.namespace, ref\.name\)/);
    assert.doesNotMatch(modal, /Promise\.all|\bHelmGet\(|\bHelmReleaseResources\(/);
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

test('drawer Details loads through one backend snapshot', () => {
    assert.match(mainJS, /GetDrawerSnapshotOwned\(connectionID, operationID, ref\.kind, ref\.namespace, ref\.name\)/);
    assert.match(mainJS, /CancelDrawerSnapshot\(requestScopes\.drawerOwnerKey\(/);
    assert.match(mainJS, /renderDetailMeta\(snapshot\.detail\)/);
    assert.match(mainJS, /renderDrawerEvents\(snapshot\.events, snapshot\.sectionErrors\?\.events, scope\)/);
    assert.match(mainJS, /loadRelations\(scope, snapshot\)/);
    assert.doesNotMatch(mainJS, /\b(DeploymentTree|ServiceTree|IngressTree|PodsOnNode|NamespaceSummary)\(/);
});

test('Incident Studio is evidence-first, one-call, owned, and responsive', () => {
    for (const id of ['drawer-tab-investigate', 'dpanel-investigate', 'incident-findings', 'incident-timeline', 'btn-incident-watch', 'btn-incident-export']) {
        assert.match(indexHTML, new RegExp(`id="${id}"`));
    }
    assert.match(mainJS, /InvestigateResource\(ref\.kind, ref\.namespace, ref\.name\)/);
    assert.match(mainJS, /stopIncidentWatch\(\)/);
    assert.match(mainJS, /requestScopes\.drawerOwnerKey\(scope\)/);
    assert.match(mainJS, /const report = incidentReport;[\s\S]*?SaveIncidentReport\(JSON\.stringify\(report\)\)/);
    assert.match(incidentCSS, /\.incident-two-column/);
    assert.match(incidentCSS, /@media \(max-width: 840px\)/);
    assert.match(incidentCSS, /@media \(prefers-reduced-motion: reduce\)/);
});

test('pasted kubeconfig uses the shared YAML editor handle', () => {
	assert.match(indexHTML, /id="paste-area"[^>]*class="[^"]*yaml-host/);
	assert.doesNotMatch(indexHTML, /<textarea[^>]*id="paste-area"/);
	assert.match(mainJS, /const pasteEditor = createYamlEditor\(\$\('paste-area'\)/);
	assert.match(mainJS, /pasteEditor\.getValue\(\)/);
	assert.match(mainJS, /source\.content = ''/);
	assert.match(mainJS, /pasteEditor\.setValue\(''\)/);
});

test('AI sends the same reviewed evidence snapshot for the whole thread', () => {
	assert.match(mainJS, /AskAboutResource\(ref\.kind, ref\.namespace, ref\.name, aiContext\.text, aiThread\)/);
	assert.match(mainJS, /if \(!q \|\| aiBusy \|\| !drawerRef \|\| !aiContext\?\.text\) return/);
	assert.doesNotMatch(mainJS, /AskAboutResource\(ref\.kind, ref\.namespace, ref\.name, aiThread\)/);
});

test('cluster writes cross confirmation and connection-ownership boundaries', () => {
    for (const binding of ['SetDeploymentPausedOwned', 'SetNodeSchedulableOwned', 'UpdateYAML', 'HelmTestOwned']) {
        const call = mainJS.lastIndexOf(`${binding}(`);
        assert.notEqual(call, -1, `${binding} should remain wired`);
        const nearby = mainJS.slice(Math.max(0, call - 1600), call + 500);
        assert.match(nearby, /confirmedAction\(/, `${binding} must stay behind explicit confirmation`);
    }
	assert.match(mainJS, /connectionOwnershipChanged\(\)[\s\S]*closeDialog\(false\)/);
	for (const binding of ['DeleteResourceOwned', 'RestartDeploymentOwned', 'DrainNodeOwned', 'RunCronJobNowOwned', 'HelmUninstallOwned']) {
		assert.match(mainJS, new RegExp(`${binding}\\(connectionID|${binding}\\(cluster`));
	}
});

test('permission gates match the exact Kubernetes request shape', () => {
	assert.match(mainJS, /btn-yaml-save'\), ownsYAML && allowed\(set, 'update'\)/);
	assert.doesNotMatch(mainJS, /btn-yaml-save'\)[^\n]*allowed\(set, 'patch'\)/);
	assert.match(mainJS, /label: 'Scale…', need: 'scale'/);
	assert.match(mainJS, /btn-scale'\), allowed\(set, 'scale'\)/);
});

test('create and import YAML apply to the modal-owning connection only', () => {
    const modal = mainJS.slice(mainJS.indexOf('function openYamlApplyModal'), mainJS.indexOf('function applyReport'));
    assert.match(modal, /const connectionID = \$\('cluster-select'\)\.value/);
    assert.match(modal, /ApplyYAMLOwned\(connectionID, v\)/);
    assert.doesNotMatch(modal, /\bApplyYAML\(/);
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
