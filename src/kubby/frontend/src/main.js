import './style.css';
import './app.css';
import './option-b.css';

import { createYamlEditor, parseApplyFailures } from './editor.js';
import { createRequestScopes } from './request-scope.js';

import {
    PickKubeconfigFile,
    ContextsFromPath,
    ContextsFromContent,
    ConnectWithPath,
    ConnectWithContent,
    ConnectedClusters,
    SwitchCluster,
    DisconnectCluster,
    ListNodes,
    ListNamespaces,
    ListPods,
    ListDeployments,
    ListServices,
    ListConfigMaps,
    ListSecrets,
    ListStatefulSets,
    ListDaemonSets,
    ListJobs,
    ListCronJobs,
    ListIngresses,
    ListPVCs,
    ListServiceAccounts,
    ListPersistentVolumes,
    ListStorageClasses,
    ListRoles,
    ListRoleBindings,
    ListClusterRoles,
    ListClusterRoleBindings,
    ListCRDs,
    ListHelmReleases,
    ListResourceQuotas,
    ListLimitRanges,
    HelmGet,
    HelmHistory,
    HelmRollback,
    HelmUninstall,
    HelmUpgradeValues,
    HelmInstall,
    SearchCharts,
    ChartDetails,
    ChartDefaultValues,
    HelmInstallPreview,
    HelmUpgradePreview,
    HelmGetRevision,
    HelmReleaseResources,
    HelmTest,
    ListHelmRepos,
    AddHelmRepo,
    RemoveHelmRepo,
    UpdateHelmRepos,
    BrowseHelmRepo,
    GetYAML,
    UpdateYAML,
    ApplyYAML,
    ApplyPreview,
    CanI,
    Sizing,
    GetDetail,
    ListEvents,
    DeleteResource,
    ScaleDeployment,
    RestartDeployment,
    RestartStatefulSet,
    RestartDaemonSet,
    PodsOnNode,
    NamespaceSummary,
    SearchResources,
    SidebarCounts,
    CustomKinds,
    ListCustom,
    NetworkFlows,
    AskAboutResource,
    AIResourceContext,
    GetAIStatus,
    GetAIConfig,
    SaveAIConfig,
    AppVersion,
    Diagnostics,
    CopyToClipboard,
    SetDeploymentPaused,
    RolloutHistory,
    RollbackDeployment,
    SetNodeSchedulable,
    DrainNode,
    RunCronJobNow,
    SecretData,
    RecentConnections,
    ForgetConnection,
    DeploymentTree,
    ServiceTree,
    IngressTree,
    NodeMetrics,
    TopPods,
    ClusterEvents,
    PodMetricsList,
    PodContainers,
    PodLogs,
    StartLogStream,
    StopLogStream,
    SaveTextToFile,
    StartPortForward,
    StopPortForward,
    StartExec,
    ExecWrite,
    StopExec,
} from '../wailsjs/go/main/App';
import { EventsOn, BrowserOpenURL } from '../wailsjs/runtime/runtime';

const PAGE_TITLES = {
    overview: 'Overview',
    nodes: 'Nodes',
    namespaces: 'Namespaces',
    sizing: 'Right-sizing',
    pods: 'Pods',
    deployments: 'Deployments',
    services: 'Services',
    traffic: 'Traffic flow',
    configmaps: 'ConfigMaps',
    secrets: 'Secrets',
    statefulsets: 'StatefulSets',
    daemonsets: 'DaemonSets',
    jobs: 'Jobs',
    cronjobs: 'CronJobs',
    ingresses: 'Ingresses',
    pvcs: 'PersistentVolumeClaims',
    serviceaccounts: 'ServiceAccounts',
    pvs: 'PersistentVolumes',
    storageclasses: 'StorageClasses',
    roles: 'Roles',
    rolebindings: 'RoleBindings',
    clusterroles: 'ClusterRoles',
    clusterrolebindings: 'ClusterRoleBindings',
    crds: 'CustomResourceDefinitions',
    helm: 'Helm Releases',
    helmrepos: 'Helm Repositories',
    resourcequotas: 'ResourceQuotas',
    limitranges: 'LimitRanges',
};

const PAGE_SUBTITLES = {
    overview: 'Live health and capacity across the connected cluster.',
    nodes: 'Inspect cluster machines, readiness, versions, and scheduled workloads.',
    namespaces: 'Browse logical scopes and the resources running inside them.',
    sizing: 'Compare requested resources with live usage and find waste or risk.',
    pods: 'Monitor workload health, resource usage, logs, terminals, and events.',
    deployments: 'Review rollout health and safely scale, restart, pause, or roll back.',
    services: 'Inspect stable network endpoints and the workloads behind them.',
    traffic: 'Trace ingress and service paths through to their backing pods.',
    configmaps: 'Browse application configuration stored in the cluster.',
    secrets: 'Inspect secret metadata and reveal values only when explicitly requested.',
    statefulsets: 'Monitor ordered, stateful workloads and rolling restarts.',
    daemonsets: 'Review node-wide workloads and their rollout health.',
    jobs: 'Track one-time workloads and completion status.',
    cronjobs: 'Inspect schedules, suspension state, and trigger jobs safely.',
    ingresses: 'Review external routes, hosts, and ingress configuration.',
    pvcs: 'Inspect namespaced storage claims and their binding state.',
    serviceaccounts: 'Review workload identities in the selected scope.',
    pvs: 'Inspect cluster-wide volumes, claims, and storage classes.',
    storageclasses: 'Review dynamic provisioning and reclaim policies.',
    roles: 'Inspect namespaced access rules.',
    rolebindings: 'See which subjects receive namespaced permissions.',
    clusterroles: 'Inspect cluster-wide access rules.',
    clusterrolebindings: 'See which subjects receive cluster-wide permissions.',
    crds: 'Browse the custom APIs installed in this cluster.',
    helm: 'Manage releases, values, history, tests, and upgrades.',
    helmrepos: 'Manage chart repositories and browse available packages.',
    resourcequotas: 'Review namespace resource limits and current usage.',
    limitranges: 'Inspect default and enforced container resource policies.',
};

const NAMESPACED_VIEWS = new Set([
    'pods', 'deployments', 'services', 'configmaps', 'secrets',
    'statefulsets', 'daemonsets', 'jobs', 'cronjobs', 'ingresses', 'pvcs', 'serviceaccounts',
    'roles', 'rolebindings', 'helm', 'resourcequotas', 'limitranges',
    'sizing',
]);

// Maps a view to the resource kind it creates (views omitted here get no Create button).
const VIEW_KIND = {
    namespaces: 'Namespace',
    pods: 'Pod',
    deployments: 'Deployment',
    services: 'Service',
    configmaps: 'ConfigMap',
    secrets: 'Secret',
    statefulsets: 'StatefulSet',
    daemonsets: 'DaemonSet',
    jobs: 'Job',
    cronjobs: 'CronJob',
    ingresses: 'Ingress',
    pvcs: 'PersistentVolumeClaim',
    serviceaccounts: 'ServiceAccount',
    pvs: 'PersistentVolume',
    storageclasses: 'StorageClass',
    roles: 'Role',
    rolebindings: 'RoleBinding',
    clusterroles: 'ClusterRole',
    clusterrolebindings: 'ClusterRoleBinding',
    resourcequotas: 'ResourceQuota',
    limitranges: 'LimitRange',
};
const LOG_TAIL_LINES = 500;

const $ = (id) => document.getElementById(id);

let source = { mode: 'path', path: '', content: '' };
let currentView = 'overview';
let currentNamespace = '';
const requestScopes = createRequestScopes();

function isCurrentViewRequest(scope) {
    return requestScopes.isCurrentView(scope, currentView, currentNamespace);
}

function viewError(scope, err) {
    if (isCurrentViewRequest(scope)) showDashError(err);
}

// ============ Welcome screen ============

document.querySelectorAll('.tab').forEach((tab) => {
    tab.addEventListener('click', () => {
        document.querySelectorAll('.tab').forEach((t) => t.classList.toggle('active', t === tab));
        $('tab-file').hidden = tab.dataset.tab !== 'file';
        $('tab-paste').hidden = tab.dataset.tab !== 'paste';
    });
});

$('btn-pick-kubeconfig').addEventListener('click', () => {
    clearWelcomeError();
    PickKubeconfigFile()
        .then((path) => {
            if (!path) return;
            source = { mode: 'path', path, content: '' };
            const picked = $('picked-path');
            picked.textContent = path;
            picked.hidden = false;
            return ContextsFromPath(path).then(fillContexts);
        })
        .catch(showWelcomeError);
});

$('btn-load-paste').addEventListener('click', () => {
    clearWelcomeError();
    const content = $('paste-area').value.trim();
    if (!content) {
        showWelcomeError('Paste your kubeconfig content first.');
        return;
    }
    source = { mode: 'content', path: '', content };
    ContextsFromContent(content).then(fillContexts).catch(showWelcomeError);
});

function fillContexts(res) {
    const select = $('context-select');
    select.innerHTML = '';
    for (const name of res.contexts ?? []) {
        const opt = document.createElement('option');
        opt.value = name;
        opt.textContent = name;
        if (name === res.currentContext) opt.selected = true;
        select.appendChild(opt);
    }
    $('context-row').hidden = false;
}

$('btn-connect').addEventListener('click', () => {
    clearWelcomeError();
    const ctx = $('context-select').value;
    const btn = $('btn-connect');
    btn.disabled = true;
    btn.textContent = 'Connecting…';
    $('connecting-overlay').hidden = false;

    const connect = source.mode === 'path'
        ? ConnectWithPath(source.path, ctx)
        : ConnectWithContent(source.content, ctx);

    connect
        .then(() => enterDashboard(ctx))
        .catch(showWelcomeError)
        .finally(() => {
            $('connecting-overlay').hidden = true;
            btn.disabled = false;
            btn.textContent = 'Connect';
        });
});

function showWelcomeError(err) {
    const el = $('welcome-error');
    el.textContent = errMsg(err);
    el.hidden = false;
}
function clearWelcomeError() { $('welcome-error').hidden = true; }

// ============ Dashboard ============

function enterDashboard() {
    requestScopes.connectionChanged();
    clearRenderedView();
    $('welcome').hidden = true;
    $('dashboard').hidden = false;
    return refreshClusterSwitcher()
        .then(() => loadNamespaceOptions())
        .then(() => {
            // Both are fire-and-forget so the first view paints immediately
            // rather than waiting on the sidebar.
            loadSidebarCounts();
            loadCustomSections();
            return selectView('overview');
        });
}

// Populate the connected-clusters dropdown and mark the active one.
function refreshClusterSwitcher() {
    return ConnectedClusters().then((clusters) => {
        const sel = $('cluster-select');
        sel.innerHTML = '';
        for (const c of clusters ?? []) {
            const opt = document.createElement('option');
            opt.value = c.name;
            opt.textContent = c.name;
            if (c.active) opt.selected = true;
            sel.appendChild(opt);
        }
    });
}

$('cluster-select').addEventListener('change', (e) => {
    const name = e.target.value;
    requestScopes.connectionChanged();
    clearRenderedView();
    closeDrawer();
    for (const f of activeForwards) StopPortForward(f.key);
    activeForwards = [];
    clearAccessCache(); // a different cluster grants different things
    SwitchCluster(name)
        .then(() => { currentNamespace = ''; return loadNamespaceOptions(); })
        .then(() => {
            // A different cluster has different counts and different CRDs — both
            // must be rebuilt, or the sidebar keeps describing the old one.
            loadSidebarCounts();
            loadCustomSections();
            return selectView('overview');
        })
        .catch((err) => {
            // The native select changes before SwitchCluster succeeds. Restore
            // the authoritative active option and reload the old scope on error.
            refreshClusterSwitcher().then(() => refreshCurrentView()).finally(() => showDashError(err));
        });
});

// "+ Add cluster" returns to Welcome but keeps existing connections alive;
// connecting there just adds another cluster and re-enters the dashboard.
$('btn-add-cluster').addEventListener('click', () => {
    requestScopes.connectionChanged();
    closeDrawer();
    $('dashboard').hidden = true;
    $('welcome').hidden = false;
    clearWelcomeError();
    populateRecent();
});

$('btn-disconnect').addEventListener('click', () => {
    const active = $('cluster-select').value;
    requestScopes.connectionChanged();
    clearRenderedView();
    closeDrawer();
    for (const f of activeForwards) StopPortForward(f.key);
    activeForwards = [];
    clearAccessCache();
    DisconnectCluster(active).then((newActive) => {
        if (!newActive) {
            // No clusters left → back to Welcome.
            $('dashboard').hidden = true;
            $('welcome').hidden = false;
            return;
        }
        currentNamespace = '';
        return refreshClusterSwitcher()
            .then(() => loadNamespaceOptions())
            .then(() => {
                loadSidebarCounts();
                loadCustomSections();
                return selectView('overview');
            });
    }).catch(showDashError);
});

document.querySelectorAll('.nav-item').forEach((btn) => {
    btn.addEventListener('click', () => selectView(btn.dataset.view));
    // Inject a count badge element into each nav item once.
    const span = document.createElement('span');
    span.className = 'nav-count';
    btn.appendChild(span);
});

document.querySelectorAll('[data-rail-view]').forEach((btn) => {
    btn.addEventListener('click', () => selectView(btn.dataset.railView));
});
document.querySelector('[data-rail-action="theme"]')?.addEventListener('click', () => $('btn-theme').click());
document.querySelector('[data-rail-action="settings"]')?.addEventListener('click', () => $('btn-settings').click());
$('btn-command-palette').addEventListener('click', openPalette);

// ---- Collapsible sidebar groups (accordion) ----
const NAV_COLLAPSE_KEY = 'kubby-nav-collapsed';

function persistNavCollapsed() {
    const collapsed = [...document.querySelectorAll('.nav-section.collapsed')].map((s) => s.dataset.section);
    try { localStorage.setItem(NAV_COLLAPSE_KEY, JSON.stringify(collapsed)); } catch { /* ignore */ }
}

function restoreNavCollapsed() {
    let collapsed = [];
    try { collapsed = JSON.parse(localStorage.getItem(NAV_COLLAPSE_KEY) || '[]'); } catch { /* ignore */ }
    for (const sec of document.querySelectorAll('.nav-section')) {
        const on = collapsed.includes(sec.dataset.section);
        sec.classList.toggle('collapsed', on);
        sec.querySelector('.nav-group')?.setAttribute('aria-expanded', String(!on));
    }
}

document.querySelectorAll('.nav-group').forEach((group) => {
    group.addEventListener('click', () => {
        const section = group.closest('.nav-section');
        const collapsed = section.classList.toggle('collapsed');
        group.setAttribute('aria-expanded', String(!collapsed));
        persistNavCollapsed();
    });
});
restoreNavCollapsed();

// Make sure the group holding a given view is expanded (used when jumping via
// the command palette or programmatic selectView into a collapsed group).
function revealNavSection(view) {
    const item = document.querySelector(`.nav-item[data-view="${view}"]`);
    const section = item?.closest('.nav-section');
    if (section && section.classList.contains('collapsed')) {
        section.classList.remove('collapsed');
        section.querySelector('.nav-group')?.setAttribute('aria-expanded', 'true');
        persistNavCollapsed();
    }
}

// Sidebar counts come from ONE bound call that fans out concurrently in Go
// (`SidebarCounts`). It used to be one call per kind — ~25 round-trips over the
// Wails bridge, each serialising every object of that kind just so JS could take
// `.length` — and switching namespace fired all of them again. That was the stall.
//
// A namespace change passes includeCluster=false: nodes, PVs, StorageClasses,
// ClusterRoles/Bindings and CRDs cannot change count when only the namespace
// does, so their badges are left as they are instead of being refetched.
let navCountsReqId = 0;

function loadSidebarCounts({ includeCluster = true } = {}) {
    const reqId = ++navCountsReqId;
    const connection = requestScopes.connectionToken();
    const namespace = currentNamespace;
    return SidebarCounts(namespace, includeCluster)
        .then((counts) => {
            // Drop a slow reply that a newer request has already superseded —
            // otherwise flicking through namespaces can leave older numbers on top.
            if (reqId !== navCountsReqId || !requestScopes.isCurrentConnection(connection)) return;
            for (const c of counts ?? []) {
                const badge = document.querySelector(`.nav-item[data-view="${c.view}"] .nav-count`);
                if (!badge) continue;
                badge.textContent = String(c.count);
                badge.classList.toggle('nav-count-err', c.errors > 0);
                badge.title = c.errors > 0 ? `${c.errors} with errors` : '';
            }
        })
        .catch(() => {});
}

$('namespace-select').addEventListener('change', (e) => {
    currentNamespace = e.target.value;
    updateNsScope();
    loadSidebarCounts({ includeCluster: false });
    refreshCurrentView();
});

$('btn-refresh').addEventListener('click', () => { loadSidebarCounts(); refreshCurrentView(); });

// ---- Custom-resource sections (built from the cluster's CRDs) ----
//
// Every CRD-defined kind becomes a real section: a sidebar entry, a "Go to …"
// command-palette entry (the palette iterates PAGE_TITLES, so registering there
// is all it takes), and a generic table that drills into the usual drawer.
// A view id is `custom:<Kind.group>`; see viewSectionId() for how one shared
// DOM section serves them all.
//
// Deliberately no count badges here: that would mean one list per kind on every
// refresh, which is the cost the sidebar was just relieved of.
const CUSTOM_KINDS = new Map(); // view id → { refKind, title, namespaced }

function loadCustomSections() {
    const box = $('nav-custom-items');
    const section = $('nav-section-custom');
    const connection = requestScopes.connectionToken();
    return CustomKinds()
        .then((res) => {
            if (!requestScopes.isCurrentConnection(connection)) return;
            const kinds = res?.kinds ?? [];
            CUSTOM_KINDS.clear();
            box.innerHTML = '';
            section.hidden = kinds.length === 0;
            if (kinds.length === 0) return;

            for (const k of kinds) {
                const view = `custom:${k.refKind}`;
                CUSTOM_KINDS.set(view, k);
                PAGE_TITLES[view] = k.title;
                if (k.namespaced) NAMESPACED_VIEWS.add(view);

                const btn = document.createElement('button');
                btn.className = 'nav-item';
                btn.dataset.view = view;
                btn.title = k.refKind;
                btn.innerHTML = `<svg class="ico"><use href="#i-crds"/></svg><span class="nav-label">${esc(k.title)}</span>`;
                btn.addEventListener('click', () => selectView(view));
                box.appendChild(btn);
            }
            // Say so when the cap hid some, rather than looking complete.
            $('nav-custom-count').textContent = res.overflow > 0
                ? `${kinds.length} of ${res.total}` : String(kinds.length);
            $('nav-custom-count').title = res.overflow > 0
                ? `${res.overflow} more kinds exist — open them from CRDs`
                : '';
        })
        .catch(() => { if (requestScopes.isCurrentConnection(connection)) section.hidden = true; });
}

function loadCustom(scope) {
    const meta = CUSTOM_KINDS.get(scope.view);
    if (!meta) return Promise.resolve();
    $('custom-title').textContent = meta.title;
    $('custom-api').textContent = meta.refKind;
    return ListCustom(meta.refKind, meta.namespaced ? scope.namespace : '')
        .then((items) => {
            if (!isCurrentViewRequest(scope)) return;
            const body = $('custom-body');
            const table = body.closest('table');
            body.innerHTML = '';
            $('custom-empty').hidden = (items?.length ?? 0) > 0;
            for (const it of items ?? []) {
                const tr = row(
                    `<td>${esc(it.namespace)}</td><td>${esc(it.name)}</td>`
                    + `<td>${it.status ? badge(it.status, !it.isError) : '<span class="dim">—</span>'}</td>`
                    + `<td>${esc(it.age)}</td>`,
                    { isError: !!it.isError, ref: { kind: meta.refKind, namespace: it.namespace, name: it.name } },
                );
                padRowToHeader(tr, table);
                body.appendChild(tr);
            }
        })
        .catch((err) => viewError(scope, err));
}

// Custom-resource views share one DOM section (#view-custom) because there is no
// per-kind markup to generate — only the kind being listed differs.
function viewSectionId(view) {
    return String(view).startsWith('custom:') ? 'view-custom' : `view-${view}`;
}

function selectView(view) {
    currentView = view;
    revealNavSection(view);
    const sectionId = viewSectionId(view);
    document.querySelectorAll('.nav-item').forEach((b) => b.classList.toggle('active', b.dataset.view === view));
    updateRailActive(view);
    document.querySelectorAll('.view').forEach((v) => (v.hidden = v.id !== sectionId));
    $('page-title').textContent = PAGE_TITLES[view] ?? view;
    $('page-subtitle').textContent = PAGE_SUBTITLES[view]
        ?? (String(view).startsWith('custom:') ? 'Browse this custom API and inspect its live resources.' : 'Browse and manage live cluster resources.');
    const navItem = document.querySelector(`.nav-item[data-view="${view}"]`);
    $('page-eyebrow').textContent = navItem?.closest('.nav-section')?.querySelector('.nav-group span')?.textContent ?? 'Workspace';
    const createKind = VIEW_KIND[view];
    $('btn-create').hidden = !createKind;
    // Warm the permission probe for this view's kind — the row menus opened from
    // it read the answer synchronously — and gate Create on it when it lands.
    // Import stays enabled: its YAML can name any kind, so there is nothing
    // specific to check.
    if (createKind) {
        const scopeNamespace = currentNamespace;
        const ns = NAMESPACED_VIEWS.has(view) ? scopeNamespace : '';
        const connection = requestScopes.connectionToken();
        gate($('btn-create'), allowed(accessPeek(createKind, ns), 'create'), denyReason(createKind, ns, 'create'));
        accessSet(createKind, ns).then((set) => {
            if (currentView !== view || currentNamespace !== scopeNamespace || !requestScopes.isCurrentConnection(connection)) return;
            gate($('btn-create'), allowed(set, 'create'), denyReason(createKind, ns, 'create'));
        });
    }
    // The instant filter applies to table views only (not the dashboard-style views).
    const hasTable = view !== 'overview' && view !== 'traffic' && view !== 'sizing';
    $('view-search').hidden = !hasTable;
    $('view-filter').value = '';
    updateNsScope();
    refreshCurrentView();
}

function updateRailActive(view) {
    const section = document.querySelector(`.nav-item[data-view="${view}"]`)?.closest('.nav-section')?.dataset.section;
    const railView = ({ cluster: 'overview', workloads: 'pods', network: 'traffic', config: 'configmaps', storage: 'pvs', access: 'roles', ecosystem: 'helm', custom: 'helm' })[section]
        ?? 'overview';
    document.querySelectorAll('[data-rail-view]').forEach((button) => {
        button.classList.toggle('active', button.dataset.railView === railView);
    });
}

function updateNsScope() {
    const scopeEl = $('ns-scope');
    scopeEl.textContent = NAMESPACED_VIEWS.has(currentView)
        ? (currentNamespace === '' ? 'All namespaces' : `Namespace: ${currentNamespace}`)
        : '';
}

function refreshCurrentView() {
    clearDashError();
    clearSelection();
    const scope = requestScopes.beginView(currentView, currentNamespace);
    // Rows are actionable. Remove the previous owner's rows immediately so a
    // failed or slow refresh cannot leave clickable data from another scope.
    clearRenderedView(scope.view);
    const p = doRefresh(scope);
    Promise.resolve(p).finally(() => { if (isCurrentViewRequest(scope)) filterCurrentTable(); });
    return p;
}

function clearRenderedView(view = currentView) {
    document.querySelectorAll(`#${viewSectionId(view)} tbody`).forEach((body) => { body.innerHTML = ''; });
}

// Instant client-side filter over the current view's table rows.
$('view-filter').addEventListener('input', filterCurrentTable);

function filterCurrentTable() {
    if (currentView === 'overview') { $('view-count').textContent = ''; return; }
    const body = document.querySelector(`#${viewSectionId(currentView)} tbody`);
    if (!body) { $('view-count').textContent = ''; return; }
    const term = $('view-filter').value.trim().toLowerCase();
    const rows = body.querySelectorAll('tr');
    let shown = 0;
    for (const tr of rows) {
        const match = !term || tr.textContent.toLowerCase().includes(term);
        tr.hidden = !match;
        if (match) shown++;
    }
    $('view-count').textContent = term ? `${shown} / ${rows.length}` : `${rows.length}`;
}

function doRefresh(scope) {
    if (String(scope.view).startsWith('custom:')) return loadCustom(scope);
    switch (scope.view) {
        case 'overview': return loadOverview(scope);
        case 'nodes': return loadNodes(scope);
        case 'namespaces': return loadNamespaces(scope);
        case 'sizing': return loadSizing(scope);
        case 'pods': return loadPods(scope);
        case 'deployments': return loadDeployments(scope);
        case 'services': return loadServices(scope);
        case 'configmaps': return loadConfigMaps(scope);
        case 'secrets': return loadSecrets(scope);
        case 'statefulsets': return loadSimple(scope, ListStatefulSets, 'statefulsets', 'StatefulSet',
            (s) => `<td>${esc(s.namespace)}</td><td>${esc(s.name)}</td><td>${badge(s.ready, !s.isError)}</td><td>${esc(s.age)}</td>`);
        case 'daemonsets': return loadSimple(scope, ListDaemonSets, 'daemonsets', 'DaemonSet',
            (d) => `<td>${esc(d.namespace)}</td><td>${esc(d.name)}</td><td>${d.desired}</td><td>${d.ready}</td><td>${d.available}</td><td>${esc(d.age)}</td>`);
        case 'jobs': return loadSimple(scope, ListJobs, 'jobs', 'Job',
            (j) => `<td>${esc(j.namespace)}</td><td>${esc(j.name)}</td><td>${badge(j.completions, !j.isError)}</td><td>${j.active}</td><td>${esc(j.age)}</td>`);
        case 'cronjobs': return loadSimple(scope, ListCronJobs, 'cronjobs', 'CronJob',
            (c) => `<td>${esc(c.namespace)}</td><td>${esc(c.name)}</td><td class="mono">${esc(c.schedule)}</td><td>${c.suspend}</td><td>${c.active}</td><td>${esc(c.age)}</td>`);
        case 'ingresses': return loadSimple(scope, ListIngresses, 'ingresses', 'Ingress',
            (i) => `<td>${esc(i.namespace)}</td><td>${esc(i.name)}</td><td>${esc(i.class)}</td><td>${esc(i.hosts)}</td><td>${esc(i.age)}</td>`);
        case 'pvcs': return loadSimple(scope, ListPVCs, 'pvcs', 'PersistentVolumeClaim',
            (p) => `<td>${esc(p.namespace)}</td><td>${esc(p.name)}</td><td>${badge(p.status, !p.isError)}</td><td>${esc(p.capacity)}</td><td>${esc(p.storageClass)}</td><td>${esc(p.age)}</td>`);
        case 'serviceaccounts': return loadSimple(scope, ListServiceAccounts, 'serviceaccounts', 'ServiceAccount',
            (s) => `<td>${esc(s.namespace)}</td><td>${esc(s.name)}</td><td>${s.secrets}</td><td>${esc(s.age)}</td>`);
        case 'pvs': return loadSimple(scope, ListPersistentVolumes, 'pvs', 'PersistentVolume',
            (p) => `<td>${esc(p.name)}</td><td>${esc(p.capacity)}</td><td>${esc(p.accessModes)}</td><td>${badge(p.status, !p.isError)}</td><td>${esc(p.claim)}</td><td>${esc(p.storageClass)}</td><td>${esc(p.age)}</td>`);
        case 'storageclasses': return loadSimple(scope, ListStorageClasses, 'storageclasses', 'StorageClass',
            (s) => `<td>${esc(s.name)}${s.isDefault ? ' <span class="chip">default</span>' : ''}</td><td class="mono">${esc(s.provisioner)}</td><td>${esc(s.reclaimPolicy)}</td><td>${s.isDefault}</td><td>${esc(s.age)}</td>`);
        case 'roles': return loadSimple(scope, ListRoles, 'roles', 'Role',
            (r) => `<td>${esc(r.namespace)}</td><td>${esc(r.name)}</td><td>${r.rules}</td><td>${esc(r.age)}</td>`);
        case 'rolebindings': return loadSimple(scope, ListRoleBindings, 'rolebindings', 'RoleBinding',
            (r) => `<td>${esc(r.namespace)}</td><td>${esc(r.name)}</td><td class="mono">${esc(r.roleRef)}</td><td>${esc(r.subjects)}</td><td>${esc(r.age)}</td>`);
        case 'clusterroles': return loadSimple(scope, ListClusterRoles, 'clusterroles', 'ClusterRole',
            (r) => `<td>${esc(r.name)}</td><td>${r.rules}</td><td>${esc(r.age)}</td>`);
        case 'clusterrolebindings': return loadSimple(scope, ListClusterRoleBindings, 'clusterrolebindings', 'ClusterRoleBinding',
            (r) => `<td>${esc(r.name)}</td><td class="mono">${esc(r.roleRef)}</td><td>${esc(r.subjects)}</td><td>${esc(r.age)}</td>`);
        case 'crds': return loadSimple(scope, ListCRDs, 'crds', 'CustomResourceDefinition',
            (c) => `<td>${esc(c.name)}</td><td class="mono">${esc(c.group)}</td><td>${esc(c.kind)}</td><td>${esc(c.scope)}</td><td>${esc(c.versions)}</td><td>${esc(c.age)}</td>`);
        case 'resourcequotas': return loadSimple(scope, ListResourceQuotas, 'resourcequotas', 'ResourceQuota',
            (q) => `<td>${esc(q.namespace)}</td><td>${esc(q.name)}</td><td>${quotaChips(q.summary)}</td><td>${esc(q.age)}</td>`);
        case 'limitranges': return loadSimple(scope, ListLimitRanges, 'limitranges', 'LimitRange',
            (l) => `<td>${esc(l.namespace)}</td><td>${esc(l.name)}</td><td>${l.limits}</td>
                    <td>${(l.types || '').split(',').map((t) => t.trim()).filter(Boolean).map((t) => `<span class="chip">${esc(t)}</span>`).join('') || '<span class="dim">—</span>'}</td>
                    <td>${esc(l.age)}</td>`);
        case 'helm': return loadHelm(scope);
        case 'helmrepos': return loadHelmRepos(scope);
        case 'traffic': return loadTraffic(scope);
    }
}

// Helm releases are backed by Secrets — rows drill into the underlying Secret.
function loadHelm(scope) {
    return ListHelmReleases(scope.namespace)
        .then((rels) => {
            if (!isCurrentViewRequest(scope)) return;
            const body = $('helm-body');
            body.innerHTML = '';
            $('helm-empty').hidden = (rels?.length ?? 0) > 0;
            for (const r of rels ?? []) {
                body.appendChild(row(
                    `<td>${esc(r.namespace)}</td><td>${esc(r.name)}</td><td>${esc(r.revision)}</td><td>${badge(r.status, !r.isError)}</td><td>${esc(r.updated)}</td>`,
                    { isError: r.isError, ref: { kind: 'HelmRelease', namespace: r.namespace, name: r.name, secretName: r.secretName } },
                ));
            }
        })
        .catch((err) => viewError(scope, err));
}

// Kinds whose backend List method takes no namespace argument.
const CLUSTER_SCOPED_KINDS = new Set([
    'PersistentVolume', 'StorageClass', 'ClusterRole', 'ClusterRoleBinding', 'CustomResourceDefinition',
]);

// Generic loader for the simpler resource lists.
function loadSimple(scope, listFn, viewId, kind, cellsFn) {
    const call = CLUSTER_SCOPED_KINDS.has(kind) ? listFn() : listFn(scope.namespace);
    return call
        .then((items) => {
            if (!isCurrentViewRequest(scope)) return;
            const body = $(`${viewId}-body`);
            const table = body.closest('table');
            body.innerHTML = '';
            $(`${viewId}-empty`).hidden = (items?.length ?? 0) > 0;
            for (const it of items ?? []) {
                const tr = row(cellsFn(it), {
                    isError: !!it.isError,
                    ref: { kind, namespace: it.namespace ?? '', name: it.name },
                });
                padRowToHeader(tr, table);
                body.appendChild(tr);
            }
        })
        .catch((err) => viewError(scope, err));
}

// The Age + Actions headers are appended to every list table by JS (below), so a
// cellsFn that emits fewer cells than its header row would silently shift every
// column — the ⋯ button ends up under "Age". Pad short rows just before the
// actions cell so the last two columns always line up with their headers.
function padRowToHeader(tr, table) {
    const want = table?.querySelectorAll('thead th').length ?? 0;
    const actions = tr.querySelector('.col-actions');
    for (let have = tr.children.length; have < want; have++) {
        const td = document.createElement('td');
        td.className = 'col-pad';
        tr.insertBefore(td, actions);
    }
}

// Render a ResourceQuota's "cpu 500m/2, pods 3/10" summary as readable chips,
// highlighting any resource that has reached its hard limit.
function quotaChips(summary) {
    const parts = String(summary || '').split(',').map((s) => s.trim()).filter(Boolean);
    if (parts.length === 0) return '<span class="dim">no hard limits</span>';
    return `<div class="quota-chips">${parts.map((p) => {
        const m = p.match(/^(\S+)\s+(\S+)\/(\S+)$/);
        if (!m) return `<span class="chip">${esc(p)}</span>`;
        const full = m[2] === m[3];
        return `<span class="chip quota-chip${full ? ' quota-full' : ''}">
            <span class="quota-res">${esc(m[1])}</span>
            <span class="quota-val mono">${esc(m[2])}<span class="dim"> / ${esc(m[3])}</span></span>
        </span>`;
    }).join('')}</div>`;
}

function loadNamespaceOptions(connection = requestScopes.connectionToken()) {
    return ListNamespaces()
        .then((namespaces) => {
            if (!requestScopes.isCurrentConnection(connection)) return;
            const select = $('namespace-select');
            select.innerHTML = '<option value="">All namespaces</option>';
            for (const ns of namespaces ?? []) {
                const opt = document.createElement('option');
                opt.value = ns.name;
                opt.textContent = ns.name;
                select.appendChild(opt);
            }
        })
        .catch((err) => { if (requestScopes.isCurrentConnection(connection)) showDashError(err); });
}

// Build a table row; if `ref` is given the row is clickable and opens the drawer.
// Unless opts.actions === false, a leading checkbox cell and trailing ⋯ actions
// cell are added automatically (headers get matching columns via JS).
function row(cellsHtml, opts = {}) {
    const tr = document.createElement('tr');
    tr.innerHTML = cellsHtml;
    const withActions = !!opts.ref && opts.actions !== false;
    if (withActions) {
        tr.insertAdjacentHTML('afterbegin', `<td class="col-check"><input type="checkbox" class="row-check"></td>`);
        tr.insertAdjacentHTML('beforeend', `<td class="col-actions">${actionsBtn()}</td>`);
    }
    if (opts.isError) tr.classList.add('error-row');
    if (opts.ref) {
        tr.classList.add('clickable');
        tr.addEventListener('click', (e) => { if (!e.target.closest('.col-check')) openDrawer(opts.ref); });
        if (withActions) {
            wireRowActions(tr, opts.ref);
            const cb = tr.querySelector('.row-check');
            cb.__ref = opts.ref;
            cb.addEventListener('change', () => toggleRowSelection(opts.ref, cb.checked, tr));
        }
    }
    return tr;
}

// ---- Overview ----
function loadOverview(scope) {
    // These three populate their own cards independently (each handles its own errors).
    loadNodeMetrics(scope);
    loadTopPods(scope);
    loadRecentEvents(scope);

    return Promise.all([ListNodes(), ListNamespaces(), ListPods(''), ListDeployments('')])
        .then(([nodes, namespaces, pods, deployments]) => {
            if (!isCurrentViewRequest(scope)) return;
            const errored = (pods ?? []).filter((p) => p.isError);
            $('stat-nodes').textContent = nodes?.length ?? 0;
            $('stat-namespaces').textContent = namespaces?.length ?? 0;
            $('stat-pods').textContent = pods?.length ?? 0;
            $('stat-deployments').textContent = deployments?.length ?? 0;
            $('stat-errors').textContent = errored.length;
            updateClusterHealth(pods ?? [], errored);

            const body = $('overview-errors-body');
            body.innerHTML = '';
            $('overview-errors-empty').hidden = errored.length > 0;
            for (const p of errored) {
                body.appendChild(row(
                    `<td>${esc(p.namespace)}</td><td>${esc(p.name)}</td><td>${badge(p.status, false)}</td><td>${p.restarts}</td>`,
                    { isError: true, actions: false, ref: { kind: 'Pod', namespace: p.namespace, name: p.name, isPod: true } },
                ));
            }
            const diagnose = $('overview-ai-diagnose');
            diagnose.hidden = errored.length === 0;
            diagnose.onclick = errored.length === 0 ? null : () => {
                const pod = errored[0];
                openDrawer({ kind: 'Pod', namespace: pod.namespace, name: pod.name, isPod: true, tab: 'ai' });
            };
        })
        .catch((err) => {
            if (!isCurrentViewRequest(scope)) return;
            updateClusterHealth(null, []);
            showDashError(err);
        });
}

function updateClusterHealth(pods, errored) {
    const score = $('cluster-health-score');
    const badgeEl = $('cluster-health-badge');
    const fill = $('cluster-health-fill');
    const total = pods?.length ?? 0;
    if (!pods) {
        score.textContent = '–';
        $('cluster-health-summary').textContent = 'Cluster health could not be loaded.';
        $('cluster-health-ratio').textContent = '– / –';
        fill.style.width = '0%';
        badgeEl.textContent = 'Unavailable';
        badgeEl.className = 'health-state health-state-warn';
        return;
    }
    const unhealthy = errored?.length ?? 0;
    const healthy = Math.max(total - unhealthy, 0);
    const percent = total > 0 ? Math.round((healthy / total) * 100) : 100;
    score.textContent = String(percent);
    $('cluster-health-summary').textContent = total > 0
        ? `${healthy} of ${total} pods are healthy`
        : 'No pods are running in the cluster yet';
    $('cluster-health-ratio').textContent = `${healthy} / ${total}`;
    fill.style.width = `${percent}%`;
    $('cluster-health-note').textContent = unhealthy > 0
        ? `${unhealthy} pod${unhealthy === 1 ? '' : 's'} need attention. Open a row below for live evidence.`
        : 'Calculated from live pod status returned by the connected cluster.';
    badgeEl.textContent = unhealthy === 0 ? 'Healthy' : (percent >= 90 ? `${unhealthy} warning${unhealthy === 1 ? '' : 's'}` : 'Needs attention');
    badgeEl.className = `health-state ${unhealthy === 0 ? 'health-state-ok' : (percent >= 90 ? 'health-state-warn' : 'health-state-error')}`;
}

function loadTopPods(scope) {
    TopPods(8)
        .then((pods) => {
            if (!isCurrentViewRequest(scope)) return;
            const body = $('overview-toppods-body');
            body.innerHTML = '';
            $('overview-toppods-empty').hidden = (pods?.length ?? 0) > 0;
            for (const p of pods ?? []) {
                body.appendChild(row(
                    `<td>${esc(p.namespace)}</td><td>${esc(p.name)}</td><td class="mono">${p.cpuMilli}m</td><td class="mono">${p.memMi}Mi</td>`,
                    { actions: false, ref: { kind: 'Pod', namespace: p.namespace, name: p.name, isPod: true } },
                ));
            }
        })
        .catch(() => {
            if (!isCurrentViewRequest(scope)) return;
            $('overview-toppods-body').innerHTML = '';
            $('overview-toppods-empty').hidden = false;
        });
}

function loadRecentEvents(scope) {
    ClusterEvents(15)
        .then((events) => {
            if (!isCurrentViewRequest(scope)) return;
            const body = $('overview-events-body');
            body.innerHTML = '';
            $('overview-events-empty').hidden = (events?.length ?? 0) > 0;
            for (const e of events ?? []) {
                const tr = document.createElement('tr');
                const cls = e.isWarn ? 'ev-type-warn' : 'ev-type-normal';
                const count = e.count > 1 ? ` (x${e.count})` : '';
                tr.innerHTML = `<td class="${cls}">${esc(e.type)}</td><td class="mono">${esc(e.object)}</td><td>${esc(e.reason)}</td><td>${esc(e.age)}${count}</td><td>${esc(e.message)}</td>`;
                body.appendChild(tr);
            }
        })
        .catch(() => {
            if (!isCurrentViewRequest(scope)) return;
            $('overview-events-body').innerHTML = '';
            $('overview-events-empty').hidden = false;
        });
}

function loadNodeMetrics(scope) {
    NodeMetrics()
        .then((metrics) => {
            if (!isCurrentViewRequest(scope)) return;
            const box = $('node-metrics');
            const hint = $('node-metrics-hint');
            const total = $('node-metrics-total');
            if (!metrics || metrics.length === 0) {
                box.innerHTML = '';
                total.textContent = '';
                hint.hidden = false;
                updateCapacitySummary(null);
                return;
            }
            hint.hidden = true;

            // Cluster-wide first: with a dozen nodes the per-node cards answer
            // "which node is hot", but not "how much room is left overall".
            const sum = metrics.reduce((a, m) => ({
                cpu: a.cpu + (m.cpuMilli || 0), cpuCap: a.cpuCap + (m.cpuCapacity || 0),
                mem: a.mem + (m.memMi || 0), memCap: a.memCap + (m.memCapacity || 0),
            }), { cpu: 0, cpuCap: 0, mem: 0, memCap: 0 });
            updateCapacitySummary(metrics, sum);
            total.textContent = `${metrics.length} nodes · CPU ${pct(sum.cpu, sum.cpuCap)}% of ${fmtCores(sum.cpuCap)} cores`
                + ` · Memory ${pct(sum.mem, sum.memCap)}% of ${fmtMem(sum.memCap)}`;

            box.innerHTML = metrics.map(nodeUsageCard).join('');
            box.querySelectorAll('.nm-name').forEach((el) => {
                el.addEventListener('click', () => openDrawer({ kind: 'Node', namespace: '', name: el.dataset.name }));
            });
        })
        .catch(() => {
            if (!isCurrentViewRequest(scope)) return;
            $('node-metrics').innerHTML = '';
            $('node-metrics-total').textContent = '';
            $('node-metrics-hint').hidden = false;
            updateCapacitySummary(null);
        });
}

function updateCapacitySummary(metrics, sum) {
    const hint = $('capacity-hint');
    if (!metrics || !sum) {
        $('capacity-node-count').textContent = 'Metrics unavailable';
        $('capacity-cpu-pct').textContent = '–%';
        $('capacity-memory-pct').textContent = '–%';
        $('capacity-cpu-value').textContent = 'Install Metrics Server for live usage';
        $('capacity-memory-value').textContent = 'Resource capacity is still available per node';
        $('capacity-cpu-ring').style.setProperty('--value', 0);
        $('capacity-memory-ring').style.setProperty('--value', 0);
        hint.hidden = false;
        return;
    }
    const cpuPercent = pct(sum.cpu, sum.cpuCap);
    const memoryPercent = pct(sum.mem, sum.memCap);
    $('capacity-node-count').textContent = `${metrics.length} node${metrics.length === 1 ? '' : 's'}`;
    $('capacity-cpu-pct').textContent = `${cpuPercent}%`;
    $('capacity-memory-pct').textContent = `${memoryPercent}%`;
    $('capacity-cpu-value').textContent = `${fmtCores(sum.cpu)} of ${fmtCores(sum.cpuCap)} cores`;
    $('capacity-memory-value').textContent = `${fmtMem(sum.mem)} of ${fmtMem(sum.memCap)}`;
    $('capacity-cpu-ring').style.setProperty('--value', Math.min(cpuPercent, 100));
    $('capacity-memory-ring').style.setProperty('--value', Math.min(memoryPercent, 100));
    hint.hidden = true;
}

function nodeUsageCard(m) {
    const cpuPct = pct(m.cpuMilli, m.cpuCapacity);
    const memPct = pct(m.memMi, m.memCapacity);
    const hot = Math.max(cpuPct, memPct) >= 90 ? ' nm-node-hot' : '';
    return `<article class="nm-node${hot}">
        <button type="button" class="nm-name" data-name="${esc(m.name)}" title="${esc(m.name)}">${nodeNameHtml(m.name)}</button>
        <div class="nm-meters">
            ${meter('CPU', cpuPct, `${fmtCores(m.cpuMilli)} / ${fmtCores(m.cpuCapacity)} cores`)}
            ${meter('Memory', memPct, `${fmtMem(m.memMi)} / ${fmtMem(m.memCapacity)}`)}
        </div>
    </article>`;
}

// One usage meter: label, percentage, bar, absolute values.
function meter(label, value, valText) {
    // Thresholds, not a gradient: a node at 71% and one at 88% should look
    // different at a glance, and only real pressure should read as red.
    const level = value >= 90 ? 'crit' : (value >= 70 ? 'warn' : 'ok');
    return `<div class="nm-meter nm-lvl-${level}">
        <div class="nm-meter-head">
            <span class="nm-meter-label">${esc(label)}</span>
            <span class="nm-meter-pct">${value}%</span>
        </div>
        <div class="nm-track"><div class="nm-fill" style="width:${Math.min(value, 100)}%"></div></div>
        <div class="nm-meter-val">${esc(valText)}</div>
    </div>`;
}

// Cloud node names share a long generated prefix and differ only in the last
// segment, so that segment gets the emphasis.
function nodeNameHtml(name) {
    const i = String(name).lastIndexOf('-');
    if (i <= 0) return `<strong>${esc(name)}</strong>`;
    return `<span class="nm-name-prefix">${esc(name.slice(0, i + 1))}</span><strong>${esc(name.slice(i + 1))}</strong>`;
}

function pct(used, capacity) {
    return capacity > 0 ? Math.round((used / capacity) * 100) : 0;
}

// Millicores → cores, which is the unit node capacity is actually thought in.
function fmtCores(milli) {
    const cores = (milli || 0) / 1000;
    return cores >= 10 ? cores.toFixed(0) : cores.toFixed(2).replace(/\.?0+$/, '');
}

function fmtMem(mi) {
    return (mi || 0) >= 1024 ? `${((mi || 0) / 1024).toFixed(1)} GiB` : `${mi || 0} MiB`;
}

// ---- Nodes ----
function loadNodes(scope) {
    return ListNodes()
        .then((nodes) => {
            if (!isCurrentViewRequest(scope)) return;
            const body = $('nodes-body');
            body.innerHTML = '';
            for (const n of nodes ?? []) {
                body.appendChild(row(
                    `<td>${esc(n.name)}</td><td>${badge(n.status, n.ready)}</td><td>${esc(n.role)}</td><td class="mono">${esc(n.version)}</td><td>${esc(n.age)}</td>`,
                    { ref: { kind: 'Node', namespace: '', name: n.name } },
                ));
            }
        })
        .catch((err) => viewError(scope, err));
}

// ---- Namespaces ----
function loadNamespaces(scope) {
    return ListNamespaces()
        .then((namespaces) => {
            if (!isCurrentViewRequest(scope)) return;
            const body = $('namespaces-body');
            body.innerHTML = '';
            for (const ns of namespaces ?? []) {
                body.appendChild(row(
                    `<td>${esc(ns.name)}</td><td>${badge(ns.status, ns.status === 'Active')}</td><td>${esc(ns.age)}</td>`,
                    { ref: { kind: 'Namespace', namespace: '', name: ns.name } },
                ));
            }
        })
        .catch((err) => viewError(scope, err));
}

// ---- Pods (enriched: live CPU/mem, IP, node, age, per-row actions) ----
function loadPods(scope) {
    return Promise.all([ListPods(scope.namespace), PodMetricsList(scope.namespace)])
        .then(([pods, metrics]) => {
            if (!isCurrentViewRequest(scope)) return;
            const usage = {};
            for (const m of metrics ?? []) usage[`${m.namespace}/${m.name}`] = m;
            const body = $('pods-body');
            body.innerHTML = '';
            $('pods-empty').hidden = (pods?.length ?? 0) > 0;
            for (const p of pods ?? []) {
                const m = usage[`${p.namespace}/${p.name}`];
                const cpu = m ? `${m.cpuMilli}m` : '–';
                const mem = m ? `${m.memMi}Mi` : '–';
                const ref = { kind: 'Pod', namespace: p.namespace, name: p.name, isPod: true };
                body.appendChild(row(
                    `<td>${esc(p.name)}</td><td>${esc(p.namespace)}</td><td>${esc(p.ready)}</td><td>${badge(p.status, !p.isError)}</td><td>${p.restarts}</td><td class="mono">${cpu}</td><td class="mono">${mem}</td><td class="mono">${esc(p.podIP)}</td><td class="mono">${esc(p.node)}</td><td>${esc(p.age)}</td>`,
                    { isError: p.isError, ref },
                ));
            }
        })
        .catch((err) => viewError(scope, err));
}

// A three-dot actions button + per-row menu, like Lens/Headlamp.
function actionsBtn() {
    return `<button class="row-actions-btn" title="Actions">⋯</button>`;
}

function wireRowActions(tr, ref) {
    const btn = tr.querySelector('.row-actions-btn');
    if (!btn) return;
    btn.addEventListener('click', (e) => {
        e.stopPropagation(); // don't trigger the row's openDrawer
        openRowMenu(btn, ref);
    });
}

// ---- Deployments ----
function loadDeployments(scope) {
    return ListDeployments(scope.namespace)
        .then((deps) => {
            if (!isCurrentViewRequest(scope)) return;
            const body = $('deployments-body');
            body.innerHTML = '';
            $('deployments-empty').hidden = (deps?.length ?? 0) > 0;
            for (const d of deps ?? []) {
                body.appendChild(row(
                    `<td>${esc(d.namespace)}</td><td>${esc(d.name)}</td><td>${badge(d.ready, !d.isError)}</td><td>${d.upToDate}</td><td>${d.available}</td><td>${esc(d.age)}</td>`,
                    { isError: d.isError, ref: { kind: 'Deployment', namespace: d.namespace, name: d.name } },
                ));
            }
        })
        .catch((err) => viewError(scope, err));
}

// ---- Services ----
function loadServices(scope) {
    return ListServices(scope.namespace)
        .then((svcs) => {
            if (!isCurrentViewRequest(scope)) return;
            const body = $('services-body');
            body.innerHTML = '';
            $('services-empty').hidden = (svcs?.length ?? 0) > 0;
            for (const s of svcs ?? []) {
                body.appendChild(row(
                    `<td>${esc(s.namespace)}</td><td>${esc(s.name)}</td><td>${esc(s.type)}</td><td class="mono">${esc(s.clusterIP)}</td><td class="mono">${esc(s.ports)}</td><td>${esc(s.age)}</td>`,
                    { ref: { kind: 'Service', namespace: s.namespace, name: s.name } },
                ));
            }
        })
        .catch((err) => viewError(scope, err));
}

// ---- ConfigMaps ----
function loadConfigMaps(scope) {
    return ListConfigMaps(scope.namespace)
        .then((items) => {
            if (!isCurrentViewRequest(scope)) return;
            const body = $('configmaps-body');
            body.innerHTML = '';
            $('configmaps-empty').hidden = (items?.length ?? 0) > 0;
            for (const c of items ?? []) {
                body.appendChild(row(
                    `<td>${esc(c.namespace)}</td><td>${esc(c.name)}</td><td>${c.keys}</td><td>${esc(c.age)}</td>`,
                    { ref: { kind: 'ConfigMap', namespace: c.namespace, name: c.name } },
                ));
            }
        })
        .catch((err) => viewError(scope, err));
}

// ---- Secrets ----
function loadSecrets(scope) {
    return ListSecrets(scope.namespace)
        .then((items) => {
            if (!isCurrentViewRequest(scope)) return;
            const body = $('secrets-body');
            body.innerHTML = '';
            $('secrets-empty').hidden = (items?.length ?? 0) > 0;
            for (const s of items ?? []) {
                body.appendChild(row(
                    `<td>${esc(s.namespace)}</td><td>${esc(s.name)}</td><td class="mono">${esc(s.type)}</td><td>${s.keys}</td><td>${esc(s.age)}</td>`,
                    { ref: { kind: 'Secret', namespace: s.namespace, name: s.name } },
                ));
            }
        })
        .catch((err) => viewError(scope, err));
}

// ============ Detail drawer ============

let drawerRef = null; // { kind, namespace, name, isPod }
let activeDrawerScope = null;

function isCurrentDrawerRequest(scope) {
    return !!drawerRef
        && !$('drawer').hidden
        && requestScopes.isCurrentDrawer(scope, drawerRef);
}

function openDrawer(ref) {
    if (ref.kind === 'HelmRelease') { openHelmDetailModal(ref); return; }
    stopFollow();
    stopExec();
    drawerRef = ref;
    activeDrawerScope = requestScopes.openDrawer(ref);
    const scope = activeDrawerScope;
    const isDeployment = ref.kind === 'Deployment';
    const canForward = !!ref.isPod || ref.kind === 'Service';
    // A custom resource is referenced kubectl-style as "Kind.group" so the
    // backend can resolve it unambiguously; the header shows just the kind.
    $('drawer-kind').textContent = String(ref.kind).split('.')[0];
    $('drawer-name').textContent = ref.name;
    $('drawer-ns').textContent = ref.namespace ? `namespace: ${ref.namespace}` : 'cluster-scoped';
    $('drawer-tab-logs').hidden = !ref.isPod;
    $('drawer-tab-terminal').hidden = !ref.isPod;
    $('drawer-tab-forward').hidden = !canForward;
    $('btn-scale').hidden = !isDeployment;
    $('btn-restart').hidden = !isDeployment;

    $('drawer-backdrop').hidden = false;
    $('drawer').hidden = false;

    // Gate the write buttons and the subresource tabs. Applied twice: once from
    // whatever answer is already cached, then again when the probe returns — but
    // only if this drawer is still the one open.
    applyDrawerAccess(ref);
    accessSet(ref.kind, ref.namespace).then(() => {
        if (isCurrentDrawerRequest(scope)) applyDrawerAccess(ref);
    });

    resetAIPanel(ref);
    setDrawerTab(ref.tab ?? 'details');
    loadDetails(scope);
    loadYAML(scope);
    if (ref.isPod) { prepareLogs(scope); prepareTerminal(scope); }
    if (canForward) prepareForward();
}

// Disable what this token cannot do, rather than hiding it — a greyed-out
// Terminal tab with "your token cannot exec" says something true; a missing tab
// would suggest Kubby has no terminal.
function applyDrawerAccess(ref) {
    const set = accessPeek(ref.kind, ref.namespace);
    const reason = (verb) => denyReason(ref.kind, ref.namespace, verb);

    // Saving edited YAML goes through a server-side apply, which the apiserver
    // accepts on either update or patch.
    const ownsYAML = yamlReady
        && yamlOwnerKey === requestScopes.drawerOwnerKey(activeDrawerScope)
        && isCurrentDrawerRequest(activeDrawerScope);
    gate($('btn-yaml-save'), ownsYAML && (allowed(set, 'update') || allowed(set, 'patch')),
        ownsYAML ? reason('update') : 'Wait for this resource YAML to finish loading.');
    gate($('btn-delete'), allowed(set, 'delete'), reason('delete'));
    gate($('btn-scale'), allowed(set, 'update'), reason('update'));
    gate($('btn-restart'), allowed(set, 'patch'), reason('patch'));

    if (ref.isPod) {
        gate($('drawer-tab-logs'), allowed(set, 'logs'), reason('read logs of'));
        gate($('drawer-tab-terminal'), allowed(set, 'exec'), reason('exec into'));
    }
    if (ref.isPod || ref.kind === 'Service') {
        // A Service forward resolves to a pod and forwards to that, so the
        // permission that matters is the Pod's.
        const podSet = ref.isPod ? set : accessPeek('Pod', ref.namespace);
        if (!ref.isPod) accessSet('Pod', ref.namespace);
        gate($('drawer-tab-forward'), allowed(podSet, 'portforward'),
            denyReason('Pod', ref.namespace, 'port-forward'));
    }
}

function closeDrawer() {
    stopFollow();
    stopExec();
    requestScopes.closeDrawer();
    activeDrawerScope = null;
    yamlReady = false;
    yamlOwnerKey = '';
    yamlRequestId++;
    drawerRef = null;
    $('drawer').hidden = true;
    $('drawer-backdrop').hidden = true;
}

// ---- Scale / Restart (Deployment) ----
$('btn-scale').addEventListener('click', () => { if (drawerRef) scaleRef(drawerRef); });

// Open the Scale modal for a deployment, prefilling its current replica count.
function scaleRef(ref) {
    GetDetail(ref.kind, ref.namespace, ref.name)
        .then((d) => {
            const field = (d.info ?? []).find((f) => f.label === 'Replicas');
            const current = field ? parseInt(field.value, 10) : 1;
            openScaleModal(ref, Number.isNaN(current) ? 1 : current);
        })
        .catch(() => openScaleModal(ref, 1));
}

function restartRef(ref) {
    showConfirm(`Rolling-restart deployment “${ref.name}”?`, { title: 'Restart deployment', icon: '🔄', okText: 'Restart' }).then((ok) => {
        if (!ok) return;
        RestartDeployment(ref.namespace, ref.name)
            .then(() => { if (drawerRef && drawerRef.name === ref.name) loadDetails(); refreshCurrentView(); })
            .catch((err) => showError(errMsg(err)));
    });
}

// Rolling-restart for StatefulSet / DaemonSet (fn = RestartStatefulSet|RestartDaemonSet).
function restartWorkload(ref, fn) {
    showConfirm(`Rolling-restart ${ref.kind} “${ref.name}”?`, { title: `Restart ${ref.kind}`, icon: '🔄', okText: 'Restart' }).then((ok) => {
        if (!ok) return;
        fn(ref.namespace, ref.name)
            .then(() => { if (drawerRef && drawerRef.name === ref.name) loadDetails(); refreshCurrentView(); })
            .catch((err) => showError(errMsg(err)));
    });
}

function openScaleModal(ref, current) {
    openModal({
        title: `Scale "${ref.name}"`,
        okText: 'Scale',
        bodyHtml: `<div class="modal-field">
            <label for="modal-input">Replicas</label>
            <input type="number" id="modal-input" class="modal-number" min="0" max="1000" value="${current}">
            <p class="modal-hint">Use the arrows or type a number, then Scale.</p>
        </div>`,
        onOpen: () => { const el = $('modal-input'); el.focus(); el.select(); },
        onOk: () => {
            const n = parseInt($('modal-input').value, 10);
            if (Number.isNaN(n) || n < 0) return Promise.reject('Enter a non-negative number.');
            return ScaleDeployment(ref.namespace, ref.name, n).then(() => {
                if (drawerRef && drawerRef.name === ref.name && !$('drawer').hidden) loadDetails();
                loadSidebarCounts();
                refreshCurrentView();
            });
        },
    });
}

$('btn-restart').addEventListener('click', () => {
    const ref = drawerRef;
    if (!ref) return;
    showConfirm(`Rolling-restart deployment “${ref.name}”?`, { title: 'Restart deployment', icon: '🔄', okText: 'Restart' }).then((ok) => {
        if (!ok) return;
        RestartDeployment(ref.namespace, ref.name)
            .then(() => { loadDetails(); refreshCurrentView(); })
            .catch((err) => showError(errMsg(err)));
    });
});

$('drawer-close').addEventListener('click', closeDrawer);
$('drawer-backdrop').addEventListener('click', closeDrawer);

$('btn-delete').addEventListener('click', () => {
    const ref = drawerRef;
    if (!ref) return;
    showConfirm(`Delete ${ref.kind} “${ref.name}”${ref.namespace ? ` in ${ref.namespace}` : ''}?\nThis cannot be undone.`, { title: `Delete ${ref.kind}`, icon: '🗑', okText: 'Delete', danger: true }).then((ok) => {
        if (!ok) return;
        DeleteResource(ref.kind, ref.namespace, ref.name)
            .then(() => { closeDrawer(); refreshCurrentView(); })
            .catch((err) => showError(errMsg(err)));
    });
});

document.querySelectorAll('.drawer-tab').forEach((tab) => {
    tab.addEventListener('click', () => setDrawerTab(tab.dataset.dtab));
});

function setDrawerTab(name) {
    document.querySelectorAll('.drawer-tab').forEach((t) => t.classList.toggle('active', t.dataset.dtab === name));
    $('dpanel-details').hidden = name !== 'details';
    $('dpanel-yaml').hidden = name !== 'yaml';
    $('dpanel-logs').hidden = name !== 'logs';
    $('dpanel-terminal').hidden = name !== 'terminal';
    $('dpanel-forward').hidden = name !== 'forward';
    $('dpanel-ai').hidden = name !== 'ai';
    if (name === 'ai') { prepareAIPanel(); $('ai-input').focus(); }
}

// ---- Details + Events ----
function loadDetails(scope = activeDrawerScope) {
    if (!scope || !isCurrentDrawerRequest(scope)) return;
    const ref = scope.ref;
    $('detail-meta').innerHTML = '<p class="empty-inline">Loading…</p>';
    $('detail-relations').innerHTML = '';
    $('detail-events-body').innerHTML = '';
    $('detail-events-empty').hidden = true;

    GetDetail(ref.kind, ref.namespace, ref.name)
        .then((d) => { if (isCurrentDrawerRequest(scope)) renderDetailMeta(d); })
        .catch((err) => {
            if (isCurrentDrawerRequest(scope)) $('detail-meta').innerHTML = `<p class="error">${esc(errMsg(err))}</p>`;
        });

    loadRelations(scope);
    if (ref.kind === 'Secret') loadSecretData(scope);

    ListEvents(ref.kind, ref.namespace, ref.name)
        .then((events) => {
            if (!isCurrentDrawerRequest(scope)) return;
            const body = $('detail-events-body');
            body.innerHTML = '';
            $('detail-events-empty').hidden = (events?.length ?? 0) > 0;
            for (const e of events ?? []) {
                const tr = document.createElement('tr');
                const cls = e.isWarn ? 'ev-type-warn' : 'ev-type-normal';
                const count = e.count > 1 ? ` (x${e.count})` : '';
                tr.innerHTML = `<td class="${cls}">${esc(e.type)}</td><td>${esc(e.reason)}</td><td>${esc(e.age)}${count}</td><td>${esc(e.message)}</td>`;
                body.appendChild(tr);
            }
        })
        .catch(() => { /* events are best-effort */ });
}

function renderDetailMeta(d) {
    const rows = [];
    rows.push(detailRow('Name', d.name));
    if (d.namespace) rows.push(detailRow('Namespace', d.namespace));
    if (d.age) rows.push(detailRow('Age', d.age));
    if (d.created) rows.push(detailRow('Created', d.created));
    for (const f of d.info ?? []) {
        if (f.value) rows.push(detailRow(f.label, f.value));
    }

    let html = `<button class="btn btn-secondary btn-sm ai-explain-btn">
        <svg class="ico"><use href="#i-sparkle"/></svg> Ask AI about this ${esc(d.kind ?? drawerRef?.kind ?? 'resource')}
    </button>`;
    html += `<div class="detail-group"><div class="detail-group-title">Overview</div>${rows.join('')}</div>`;
    html += chipsGroup('Labels', d.labels);
    html += chipsGroup('Annotations', d.annotations);
    $('detail-meta').innerHTML = html;
    $('detail-meta').querySelector('.ai-explain-btn')?.addEventListener('click', () => setDrawerTab('ai'));
}

function detailRow(k, v) {
    return `<div class="detail-row"><span class="k">${esc(k)}</span><span class="v">${esc(v)}</span></div>`;
}

// Relations tree: Deployment→RS→Pod, Service→Pod, Ingress→Service→Pod.
function loadRelations(scope) {
    const ref = scope.ref;
    $('detail-relations').innerHTML = '';
    if (ref.kind === 'Node') { loadNodePods(scope); return; }
    if (ref.kind === 'Namespace') { loadNamespaceSummary(scope); return; }
    const treeFn = ref.kind === 'Deployment' ? DeploymentTree
        : ref.kind === 'Service' ? ServiceTree
        : ref.kind === 'Ingress' ? IngressTree
        : null;
    if (!treeFn) return;
    treeFn(ref.namespace, ref.name)
        .then((tree) => {
            if (!isCurrentDrawerRequest(scope)) return;
            if (!tree) return;
            const box = $('detail-relations');
            box.innerHTML =
                `<h4 class="detail-section-title">Relations</h4><div class="rel-tree">${relNodeHtml(tree, ref.namespace)}</div>`;
            // Delegate clicks: navigate into the clicked child resource's drawer.
            box.querySelectorAll('.rel-name[data-kind]').forEach((el) => {
                el.addEventListener('click', () => {
                    const kind = el.dataset.kind;
                    if (kind === drawerRef?.kind && el.dataset.name === drawerRef?.name) return;
                    openDrawer({
                        kind,
                        namespace: el.dataset.namespace || '',
                        name: el.dataset.name,
                        isPod: kind === 'Pod',
                    });
                });
            });
        })
        .catch(() => { /* best-effort */ });
}

function relNodeHtml(node, namespace) {
    const errCls = node.isError ? ' err' : '';
    const status = node.status ? `<span class="rel-status">${esc(node.status)}</span>` : '';
    // Child nodes are navigable; the root resource is the current view.
    const clickable = node.kind === 'Pod' || node.kind === 'ReplicaSet' || node.kind === 'Service';
    const ns = node.namespace || namespace;
    const attrs = clickable
        ? ` data-kind="${esc(node.kind)}" data-name="${esc(node.name)}" data-namespace="${esc(ns)}"`
        : '';
    let html = `<div class="rel-node"><span class="rel-kind">${esc(node.kind)}</span><span class="rel-name${errCls}${clickable ? ' rel-link' : ''}"${attrs}>${esc(node.name)}</span>${status}`;
    if (node.children && node.children.length) {
        html += `<div class="rel-children">${node.children.map((c) => relNodeHtml(c, namespace)).join('')}</div>`;
    }
    html += `</div>`;
    return html;
}

// Node → Pods: what's scheduled on this node (grouped by namespace).
function loadNodePods(scope) {
    const ref = scope.ref;
    const box = $('detail-relations');
    box.innerHTML = '<h4 class="detail-section-title">Pods on this node</h4><p class="empty-inline">Loading…</p>';
    PodsOnNode(ref.name)
        .then((pods) => {
            if (!isCurrentDrawerRequest(scope)) return;
            if (!pods || pods.length === 0) { box.innerHTML = '<h4 class="detail-section-title">Pods on this node</h4><p class="empty-inline">No pods scheduled here.</p>'; return; }
            const rows = pods.map((p) =>
                `<div class="node-pod-row${p.isError ? ' err' : ''}" data-ns="${esc(p.namespace)}" data-name="${esc(p.name)}">
                    <span class="node-pod-name">${esc(p.name)}</span>
                    <span class="chart-repo">${esc(p.namespace)}</span>
                    <span class="node-pod-status">${badge(p.status, !p.isError)}</span>
                </div>`).join('');
            box.innerHTML = `<h4 class="detail-section-title">Pods on this node <span class="section-count">${pods.length}</span></h4><div class="node-pod-list">${rows}</div>`;
            box.querySelectorAll('.node-pod-row').forEach((el) => {
                el.addEventListener('click', () => openDrawer({ kind: 'Pod', namespace: el.dataset.ns, name: el.dataset.name, isPod: true }));
            });
        })
        .catch((err) => { if (isCurrentDrawerRequest(scope)) box.innerHTML = `<p class="error">${esc(errMsg(err))}</p>`; });
}

// Namespace → summary: per-kind counts, click a card to jump into that view scoped here.
function loadNamespaceSummary(scope) {
    const ref = scope.ref;
    const box = $('detail-relations');
    box.innerHTML = '<h4 class="detail-section-title">Contents</h4><p class="empty-inline">Loading…</p>';
    NamespaceSummary(ref.name)
        .then((counts) => {
            if (!isCurrentDrawerRequest(scope)) return;
            if (!counts || counts.length === 0) { box.innerHTML = ''; return; }
            const cards = counts.map((c) =>
                `<button class="ns-sum-card${c.errors > 0 ? ' has-err' : ''}" data-view="${esc(c.view)}">
                    <span class="ns-sum-num">${c.count}</span>
                    <span class="ns-sum-kind">${esc(c.kind)}</span>
                    ${c.errors > 0 ? `<span class="ns-sum-err">${c.errors} failing</span>` : ''}
                </button>`).join('');
            box.innerHTML = `<h4 class="detail-section-title">Contents</h4><div class="ns-sum-grid">${cards}</div>`;
            box.querySelectorAll('.ns-sum-card').forEach((el) => {
                el.addEventListener('click', () => {
                    setNamespaceScope(ref.name);
                    closeDrawer();
                    selectView(el.dataset.view);
                });
            });
        })
        .catch((err) => { if (isCurrentDrawerRequest(scope)) box.innerHTML = `<p class="error">${esc(errMsg(err))}</p>`; });
}

// Point the namespace selector at a specific namespace and refresh scope/counts.
function setNamespaceScope(ns) {
    const sel = $('namespace-select');
    if ([...sel.options].some((o) => o.value === ns)) {
        sel.value = ns;
        currentNamespace = ns;
        updateNsScope();
        loadSidebarCounts({ includeCluster: false });
    }
}

// ============ Traffic view (Ingress → Service → Pod) ============
//
// The view answers "where does a request actually end up?", so each hop gets its
// own lane and every broken hop is called out where it breaks. Nodes are real
// buttons that open the resource drawer, which is also how you delete an Ingress
// from here.

// Responses are dropped if another load started meanwhile: a slow reply landing
// after a newer one would re-render resources that no longer exist (e.g. an
// Ingress you just deleted reappearing).
let trafficReqId = 0;

function loadTraffic(scope) {
    const reqId = ++trafficReqId;
    const ingBox = $('traffic-ingress');
    const svcBox = $('traffic-services');
    return NetworkFlows(scope.namespace || '')
        .then((flows) => {
            if (reqId !== trafficReqId || !isCurrentViewRequest(scope)) return;
            const ings = flows?.ingresses ?? [];
            const svcs = flows?.services ?? [];

            renderFlowSummary(flows, ings, svcs);

            $('flow-ing-count').textContent = ings.length ? String(ings.length) : '';
            $('traffic-ingress-empty').hidden = ings.length > 0;
            ingBox.innerHTML = ings.map(flowIngressCard).join('');

            $('traffic-svc-count').textContent = svcs.length ? String(svcs.length) : '';
            $('traffic-services-empty').hidden = svcs.length > 0;
            svcBox.innerHTML = svcs.map((s) => flowHopRow(s, { standalone: true })).join('');

            wireFlowNodes(ingBox);
            wireFlowNodes(svcBox);
            $('flow-updated').textContent = `Updated ${new Date().toLocaleTimeString()}`;
            applyFlowFilter();
        })
        .catch((err) => {
            if (reqId !== trafficReqId || !isCurrentViewRequest(scope)) return;
            // Never leave the previous topology on screen behind an error — it
            // reads as current state when it isn't.
            ingBox.innerHTML = '';
            svcBox.innerHTML = '';
            $('flow-summary').innerHTML = '';
            $('flow-updated').textContent = '';
            showDashError(err);
        });
}

function renderFlowSummary(flows, ings, svcs) {
    // Ingress and Istio Gateway are both entry points, but which one a cluster
    // uses changes how you debug it — so the tile counts them separately.
    const gateways = ings.filter((i) => i.kind === 'Gateway').length;
    const ingresses = ings.length - gateways;
    const entryHint = gateways && ingresses ? `${ingresses} Ingress · ${gateways} Istio Gateway`
        : gateways ? 'Istio Gateways'
        : 'Kubernetes Ingresses';
    const tiles = [
        { n: ings.length, label: ings.length === 1 ? 'Entry point' : 'Entry points', hint: entryHint },
        { n: flows?.routedCount ?? 0, label: 'Routed services', hint: 'reachable from outside' },
        { n: svcs.length, label: 'Internal services', hint: 'in-cluster only' },
        { n: flows?.endpointCount ?? 0, label: 'Endpoint pods', hint: 'pods behind a Service' },
        { n: flows?.brokenCount ?? 0, label: 'Broken paths', hint: 'traffic that goes nowhere', bad: true },
    ];
    $('flow-summary').innerHTML = tiles.map((t) => `
        <div class="flow-stat${t.bad && t.n > 0 ? ' flow-stat-bad' : ''}">
            <span class="flow-stat-num">${t.n}</span>
            <span class="flow-stat-label">${esc(t.label)}</span>
            <span class="flow-stat-hint">${esc(t.hint)}</span>
        </div>`).join('');
}

function flowIngressCard(ing) {
    // An Istio Gateway carries a pod selector where an Ingress carries a class,
    // so the chip is labelled by what it actually is.
    const isGateway = ing.kind === 'Gateway';
    const classLabel = isGateway ? 'selector' : 'class';
    const chips = [
        ing.namespace && `<span class="chip">${esc(ing.namespace)}</span>`,
        ing.class && `<span class="chip">${classLabel}: ${esc(ing.class)}</span>`,
        ...(ing.ports ?? []).map((p) => `<span class="chip mono">${esc(p)}</span>`),
        ing.tls && `<span class="chip chip-accent">TLS</span>`,
        ing.address && `<span class="chip mono">${esc(ing.address)}</span>`,
    ].filter(Boolean).join('');
    const hosts = (ing.hosts ?? []).length
        ? `<div class="flow-hosts">${ing.hosts.map((h) => `<span class="flow-host">${esc(h)}</span>`).join('')}</div>`
        : `<div class="flow-hosts"><span class="flow-host flow-host-any">any host</span></div>`;
    const rows = (ing.services ?? []).map((s) => flowHopRow(s, { standalone: false })).join('');

    return `<article class="flow-card${ing.warning ? ' flow-card-warn' : ''}"
                     data-search="${esc(flowSearchText(ing))}"
                     data-problem="${flowHasProblem(ing) ? '1' : '0'}">
        <header class="flow-card-head">
            <span class="flow-kind flow-kind-${isGateway ? 'gw' : 'ing'}">${esc(ing.kind || 'Ingress')}</span>
            ${flowNodeBtn(ing.refKind || ing.kind || 'Ingress', ing.name, ing.namespace, 'ing')}
            <div class="flow-card-meta">${chips}</div>
        </header>
        ${hosts}
        ${ing.warning ? `<p class="flow-warn">${esc(ing.warning)}</p>` : ''}
        <div class="flow-rows">${rows}</div>
    </article>`;
}

// One hop row: [routes] → [service] → [pods]. Standalone rows (no Ingress in
// front) drop the routes lane and render as their own card.
function flowHopRow(svc, { standalone }) {
    const routes = (svc.routes ?? []).map((r) => {
        const [left, port] = String(r).split(' → ');
        return `<span class="flow-route">
            <span class="flow-route-path">${esc(left)}</span>
            ${port ? `<span class="flow-route-port mono">${esc(port)}</span>` : ''}
        </span>`;
    }).join('');

    const ports = (svc.ports ?? []).map((p) => `<code class="flow-port">${esc(p)}</code>`).join('');
    const podCount = (svc.pods ?? []).length;
    const pods = podCount
        ? svc.pods.map(flowPodPill).join('')
        : `<div class="flow-void">${svc.warning ? esc(svc.warning) : 'No pods'}</div>`;

    const state = svc.warning ? (podCount === 0 ? 'err' : 'warn') : 'ok';
    const podsHead = podCount
        ? `${svc.readyPods}/${podCount} ready`
        : 'no endpoints';

    return `<div class="flow-row flow-row-${state}${standalone ? ' flow-row-standalone' : ''}"
                 data-search="${esc(flowSearchText(svc))}"
                 data-problem="${svc.warning ? '1' : '0'}">
        ${standalone ? '' : `<div class="flow-lane flow-lane-routes">
            <span class="flow-lane-label">Route</span>
            ${routes || '<span class="dim">—</span>'}
            ${svc.via ? `<button type="button" class="flow-via"
                    data-kind="${esc(svc.viaKind)}" data-namespace="${esc(svc.viaNamespace ?? '')}" data-name="${esc(svc.viaName)}"
                    title="These routes come from ${esc(svc.via)}">via ${esc(svc.via)}</button>` : ''}
        </div>
        <div class="flow-arrow" aria-hidden="true"></div>`}
        <div class="flow-lane flow-lane-service">
            <span class="flow-lane-label">Service</span>
            ${flowNodeBtn('Service', svc.name, svc.namespace, 'svc')}
            <div class="flow-node-meta">
                ${svc.type ? `<span>${esc(svc.type)}</span>` : ''}
                ${svc.clusterIP ? `<span class="mono">${esc(svc.clusterIP)}</span>` : ''}
                ${standalone ? `<span>${esc(svc.namespace)}</span>` : ''}
            </div>
            ${ports ? `<div class="flow-ports">${ports}</div>` : ''}
            ${svc.warning && podCount ? `<p class="flow-warn">${esc(svc.warning)}</p>` : ''}
        </div>
        <div class="flow-arrow" aria-hidden="true"></div>
        <div class="flow-lane flow-lane-pods">
            <span class="flow-lane-label">Pods <span class="flow-lane-count">${esc(podsHead)}</span></span>
            <div class="flow-pods">${pods}</div>
        </div>
    </div>`;
}

function flowPodPill(p) {
    const state = p.isError ? 'err' : (p.isReady ? 'ok' : 'warn');
    return `<button type="button" class="flow-pod flow-pod-${state}"
                    data-kind="Pod" data-namespace="${esc(p.namespace)}" data-name="${esc(p.name)}"
                    title="${esc(`${p.status} · ${p.ready}${p.node ? ` · node ${p.node}` : ''}${p.ip ? ` · ${p.ip}` : ''}`)}">
        <span class="flow-dot"></span>
        <span class="flow-pod-name">${esc(p.name)}</span>
        <span class="flow-pod-meta">${esc(p.status)} · ${esc(p.ready)}</span>
    </button>`;
}

function flowNodeBtn(kind, name, namespace, cls) {
    return `<button type="button" class="flow-node flow-node-${cls}"
                    data-kind="${esc(kind)}" data-namespace="${esc(namespace ?? '')}" data-name="${esc(name)}">
        <span class="flow-node-name">${esc(name)}</span>
    </button>`;
}

// Everything a user might type to find this card, flattened once at render time.
function flowSearchText(obj) {
    const bits = [obj.name, obj.namespace, obj.kind, obj.class, obj.type, obj.clusterIP, obj.warning, obj.via];
    for (const h of obj.hosts ?? []) bits.push(h);
    for (const p of obj.ports ?? []) bits.push(p);
    for (const r of obj.routes ?? []) bits.push(r);
    for (const s of obj.services ?? []) bits.push(flowSearchText(s));
    for (const p of obj.pods ?? []) bits.push(p.name, p.node, p.ip, p.status);
    return bits.filter(Boolean).join(' ').toLowerCase();
}

function flowHasProblem(ing) {
    return !!ing.warning || (ing.services ?? []).some((s) => !!s.warning);
}

function wireFlowNodes(box) {
    box.querySelectorAll('[data-kind][data-name]').forEach((el) => {
        el.addEventListener('click', (e) => {
            e.stopPropagation();
            openDrawer({
                kind: el.dataset.kind,
                namespace: el.dataset.namespace || '',
                name: el.dataset.name,
                isPod: el.dataset.kind === 'Pod',
            });
        });
    });
}

// Client-side filter over the rendered flow: matches ingress cards as a whole,
// and individual hop rows inside them.
function applyFlowFilter() {
    const term = ($('flow-filter').value || '').trim().toLowerCase();
    const onlyProblems = $('flow-only-problems').checked;
    const keep = (el) => (!term || (el.dataset.search || '').includes(term))
        && (!onlyProblems || el.dataset.problem === '1');

    for (const card of document.querySelectorAll('#traffic-ingress .flow-card')) {
        const rows = [...card.querySelectorAll('.flow-row')];
        const cardMatches = keep(card);
        let anyRow = false;
        for (const r of rows) {
            // A card that matches by its own name/host shows all of its rows.
            const show = (cardMatches && !onlyProblems) || keep(r);
            r.hidden = !show;
            anyRow = anyRow || show;
        }
        card.hidden = !(anyRow || (cardMatches && rows.length === 0));
    }
    for (const r of document.querySelectorAll('#traffic-services .flow-row')) {
        r.hidden = !keep(r);
    }

    const ingShown = [...document.querySelectorAll('#traffic-ingress .flow-card')].filter((c) => !c.hidden).length;
    const svcShown = [...document.querySelectorAll('#traffic-services .flow-row')].filter((r) => !r.hidden).length;
    const filtering = !!term || onlyProblems;

    $('flow-ing-count').textContent = ingShown ? String(ingShown) : '';
    $('traffic-svc-count').textContent = svcShown ? String(svcShown) : '';
    // "Nothing is exposed" is only true when nothing is filtered out — otherwise
    // say so, so an empty screen never reads as an empty cluster.
    $('traffic-ingress-empty').hidden = filtering || ingShown > 0;
    $('traffic-services-empty').hidden = filtering || svcShown > 0;
    const noMatch = $('flow-nomatch');
    noMatch.hidden = !(filtering && ingShown === 0 && svcShown === 0);
    if (!noMatch.hidden) {
        noMatch.innerHTML = onlyProblems && !term
            ? '<strong>No broken paths.</strong> Every route in this scope reaches a ready pod.'
            : `<strong>No match for “${esc(term)}”.</strong> Try an ingress, host, service or pod name.`;
    }
}

$('flow-filter').addEventListener('input', applyFlowFilter);
$('flow-only-problems').addEventListener('change', applyFlowFilter);

// Secret data with per-key reveal/hide (values are decoded server-side).
function loadSecretData(scope) {
    const ref = scope.ref;
    SecretData(ref.namespace, ref.name)
        .then((entries) => {
            if (!isCurrentDrawerRequest(scope)) return;
            const box = $('detail-relations');
            if (!entries || entries.length === 0) { box.innerHTML = ''; return; }
            box.innerHTML = `<h4 class="detail-section-title">Data</h4>` + entries.map((e, i) =>
                `<div class="secret-row">
                    <div class="secret-head">
                        <span class="secret-key mono">${esc(e.key)}</span>
                        <button class="btn btn-secondary btn-sm secret-reveal" data-i="${i}">Reveal</button>
                    </div>
                    <pre class="secret-val mono" id="secret-val-${i}" hidden></pre>
                </div>`).join('');
            box.querySelectorAll('.secret-reveal').forEach((btn) => {
                btn.addEventListener('click', () => {
                    const i = btn.dataset.i;
                    const pre = $(`secret-val-${i}`);
                    if (pre.hidden) { pre.textContent = entries[i].value; pre.hidden = false; btn.textContent = 'Hide'; }
                    else { pre.hidden = true; btn.textContent = 'Reveal'; }
                });
            });
        })
        .catch(() => { if (isCurrentDrawerRequest(scope)) $('detail-relations').innerHTML = ''; });
}

function chipsGroup(title, obj) {
    const entries = Object.entries(obj ?? {});
    if (entries.length === 0) return '';
    const chips = entries.map(([k, v]) => `<span class="chip">${esc(k)}=${esc(v)}</span>`).join('');
    return `<div class="detail-group"><div class="detail-group-title">${esc(title)}</div><div class="chips">${chips}</div></div>`;
}

// ---- YAML (+ diff against the originally-loaded version) ----
let yamlOriginal = '';
let yamlDiffMode = false;
let yamlEditor = null;
let yamlReady = false;
let yamlOwnerKey = '';
let yamlRequestId = 0;

// The drawer's editor is created once and reused for every resource — the host
// element is static in index.html, and rebuilding CodeMirror on each open would
// throw away the undo history for no reason.
function drawerEditor() {
    if (!yamlEditor) yamlEditor = createYamlEditor($('yaml-editor'));
    return yamlEditor;
}

function loadYAML(scope = activeDrawerScope) {
    if (!scope || !isCurrentDrawerRequest(scope)) return;
    const ref = scope.ref;
    const reqId = ++yamlRequestId;
    yamlReady = false;
    yamlOwnerKey = '';
    applyDrawerAccess(ref);
    setYamlStatus('Loading…', '');
    drawerEditor().setValue('');
    exitDiffMode();
    GetYAML(ref.kind, ref.namespace, ref.name)
        .then((text) => {
            if (reqId !== yamlRequestId || !isCurrentDrawerRequest(scope)) return;
            yamlOriginal = text;
            drawerEditor().setValue(text);
            yamlReady = true;
            yamlOwnerKey = requestScopes.drawerOwnerKey(scope);
            applyDrawerAccess(ref);
            setYamlStatus('', '');
        })
        .catch((err) => {
            if (reqId === yamlRequestId && isCurrentDrawerRequest(scope)) setYamlStatus(errMsg(err), 'err');
        });
}

$('btn-yaml-reload').addEventListener('click', loadYAML);

$('btn-yaml-diff').addEventListener('click', () => {
    if (yamlDiffMode) { exitDiffMode(); return; }
    const diff = lineDiff(yamlOriginal, drawerEditor().getValue());
    if (diff.every((d) => d.t === ' ')) { setYamlStatus('No changes', ''); return; }
    // Joined with '', not '\n' — see renderDiffInto for why a newline here would
    // double-space every diff.
    $('yaml-diff').innerHTML = diff.map((d) => {
        const cls = d.t === '+' ? 'diff-add' : d.t === '-' ? 'diff-del' : 'diff-ctx';
        return `<span class="${cls}">${esc(d.t === ' ' ? '  ' : d.t + ' ')}${esc(d.l)}</span>`;
    }).join('');
    $('yaml-editor').hidden = true;
    $('yaml-diff').hidden = false;
    $('btn-yaml-diff').textContent = 'Edit';
    yamlDiffMode = true;
});

function exitDiffMode() {
    yamlDiffMode = false;
    $('yaml-diff').hidden = true;
    $('yaml-editor').hidden = false;
    $('btn-yaml-diff').textContent = 'Diff';
}

// Minimal LCS-based line diff (files here are small, O(n*m) is fine).
function lineDiff(aText, bText) {
    const a = aText.split('\n'), b = bText.split('\n');
    const n = a.length, m = b.length;
    const dp = Array.from({ length: n + 1 }, () => new Int32Array(m + 1));
    for (let i = n - 1; i >= 0; i--) {
        for (let j = m - 1; j >= 0; j--) {
            dp[i][j] = a[i] === b[j] ? dp[i + 1][j + 1] + 1 : Math.max(dp[i + 1][j], dp[i][j + 1]);
        }
    }
    const out = [];
    let i = 0, j = 0;
    while (i < n && j < m) {
        if (a[i] === b[j]) { out.push({ t: ' ', l: a[i] }); i++; j++; }
        else if (dp[i + 1][j] >= dp[i][j + 1]) { out.push({ t: '-', l: a[i] }); i++; }
        else { out.push({ t: '+', l: b[j] }); j++; }
    }
    while (i < n) out.push({ t: '-', l: a[i++] });
    while (j < m) out.push({ t: '+', l: b[j++] });
    return out;
}

$('btn-yaml-save').addEventListener('click', () => {
    const scope = activeDrawerScope;
    const ref = scope?.ref;
    if (!scope || !ref || !isCurrentDrawerRequest(scope)
        || !yamlReady || yamlOwnerKey !== requestScopes.drawerOwnerKey(scope)) {
        setYamlStatus('This YAML no longer belongs to the open resource. Reload it before saving.', 'err');
        return;
    }
    if (yamlDiffMode) exitDiffMode();
    const text = drawerEditor().getValue();
    const ownerKey = yamlOwnerKey;
    const expectedCluster = $('cluster-select').value;
    setYamlStatus('Saving…', '');
    UpdateYAML(expectedCluster, ref.kind, ref.namespace, ref.name, text)
        .then(() => {
            if (!isCurrentDrawerRequest(scope) || yamlOwnerKey !== ownerKey) return;
            setYamlStatus('Saved ✓', 'ok');
            yamlOriginal = text;
            refreshCurrentView();
        })
        .catch((err) => {
            if (isCurrentDrawerRequest(scope) && yamlOwnerKey === ownerKey) setYamlStatus(errMsg(err), 'err');
        });
});

function setYamlStatus(text, cls) {
    const el = $('yaml-status');
    el.textContent = text;
    el.className = 'save-status' + (cls ? ' ' + cls : '');
}

// ---- Logs (static fetch + live follow + search + download) ----
let logLines = [];
let following = false;

// A single global listener appends streamed lines (only one stream at a time).
EventsOn('logline', (line) => {
    if (!following) return;
    logLines.push(line);
    if (logLines.length > 5000) logLines = logLines.slice(-5000);
    renderLogs();
});

function prepareLogs(scope = activeDrawerScope) {
    if (!scope || !isCurrentDrawerRequest(scope)) return;
    const ref = scope.ref;
    const select = $('logs-container');
    select.innerHTML = '';
    $('logs-follow').checked = false;
    $('logs-search').value = '';
    PodContainers(ref.namespace, ref.name)
        .then((containers) => {
            if (!isCurrentDrawerRequest(scope)) return;
            for (const c of containers ?? []) {
                const opt = document.createElement('option');
                opt.value = c;
                opt.textContent = c;
                select.appendChild(opt);
            }
            loadStaticLogs(scope);
        })
        .catch((err) => {
            if (!isCurrentDrawerRequest(scope)) return;
            logLines = [errMsg(err)];
            renderLogs();
        });
}

function loadStaticLogs(scope = activeDrawerScope) {
    if (!scope || !isCurrentDrawerRequest(scope)) return;
    const ref = scope.ref;
    logLines = ['Loading logs…'];
    renderLogs();
    PodLogs(ref.namespace, ref.name, $('logs-container').value, LOG_TAIL_LINES)
        .then((text) => {
            if (!isCurrentDrawerRequest(scope)) return;
            logLines = (text || '').split('\n');
            renderLogs();
        })
        .catch((err) => {
            if (!isCurrentDrawerRequest(scope)) return;
            logLines = [errMsg(err)];
            renderLogs();
        });
}

function renderLogs() {
    const term = $('logs-search').value.trim().toLowerCase();
    const lines = term ? logLines.filter((l) => l.toLowerCase().includes(term)) : logLines;
    const view = $('logs-view');
    const atBottom = view.scrollHeight - view.scrollTop - view.clientHeight < 40;
    view.textContent = lines.join('\n') || '(no logs)';
    if (following || atBottom) view.scrollTop = view.scrollHeight;
}

function startFollow() {
    const ref = drawerRef;
    const scope = activeDrawerScope;
    if (!ref || !ref.isPod) return;
    following = true;
    logLines = [];
    renderLogs();
    StartLogStream(ref.namespace, ref.name, $('logs-container').value).catch((err) => {
        if (!isCurrentDrawerRequest(scope)) return;
        following = false;
        $('logs-follow').checked = false;
        logLines = [errMsg(err)];
        renderLogs();
    });
}

function stopFollow() {
    if (following) {
        following = false;
        StopLogStream();
    }
    const cb = $('logs-follow');
    if (cb) cb.checked = false;
}

$('logs-follow').addEventListener('change', (e) => {
    if (e.target.checked) startFollow();
    else { stopFollow(); loadStaticLogs(); }
});

$('logs-search').addEventListener('input', renderLogs);

$('btn-logs-reload').addEventListener('click', () => {
    if (following) { StopLogStream(); startFollow(); }
    else loadStaticLogs();
});

$('logs-container').addEventListener('change', () => {
    if (following) { StopLogStream(); startFollow(); }
    else loadStaticLogs();
});

$('btn-logs-download').addEventListener('click', () => {
    const ref = drawerRef;
    if (!ref) return;
    const name = `${ref.name}${$('logs-container').value ? '-' + $('logs-container').value : ''}.log`;
    SaveTextToFile(name, logLines.join('\n'))
        .catch((err) => showError(errMsg(err)));
});

// ---- Terminal (interactive exec, line mode) ----
let execConnected = false;
const ANSI_RE = /\x1b\[[0-9;?]*[ -/]*[@-~]/g; // strip CSI escape sequences

EventsOn('exec-output', (chunk) => {
    if (!execConnected) return;
    appendTerm(String(chunk).replace(ANSI_RE, ''));
});
EventsOn('exec-closed', (msg) => {
    if (msg) appendTerm(`\n[session ended: ${msg}]\n`);
    else appendTerm(`\n[session ended]\n`);
    resetTerminalUI();
});

function prepareTerminal(scope = activeDrawerScope) {
    if (!scope || !isCurrentDrawerRequest(scope)) return;
    const ref = scope.ref;
    const select = $('term-container');
    select.innerHTML = '';
    $('term-view').textContent = '';
    $('term-status').textContent = '';
    resetTerminalUI();
    PodContainers(ref.namespace, ref.name)
        .then((containers) => {
            if (!isCurrentDrawerRequest(scope)) return;
            for (const c of containers ?? []) {
                const opt = document.createElement('option');
                opt.value = c;
                opt.textContent = c;
                select.appendChild(opt);
            }
        })
        .catch((err) => { if (isCurrentDrawerRequest(scope)) $('term-status').textContent = errMsg(err); });
}

function appendTerm(text) {
    const view = $('term-view');
    view.textContent += text;
    if (view.textContent.length > 200000) view.textContent = view.textContent.slice(-200000);
    view.scrollTop = view.scrollHeight;
}

function resetTerminalUI() {
    execConnected = false;
    const input = $('term-input');
    if (input) { input.disabled = true; input.value = ''; }
    $('btn-term-start').hidden = false;
    $('btn-term-stop').hidden = true;
}

$('btn-term-start').addEventListener('click', () => {
    const ref = drawerRef;
    const scope = activeDrawerScope;
    if (!ref || !ref.isPod) return;
    const container = $('term-container').value;
    const shell = $('term-shell').value;
    $('term-status').textContent = 'Connecting…';
    $('term-view').textContent = '';
    StartExec(ref.namespace, ref.name, container, shell)
        .then(() => {
            if (!isCurrentDrawerRequest(scope)) { StopExec(); return; }
            execConnected = true;
            $('term-status').textContent = `connected (${shell})`;
            $('btn-term-start').hidden = true;
            $('btn-term-stop').hidden = false;
            const input = $('term-input');
            input.disabled = false;
            input.focus();
            appendTerm(`Connected to ${ref.name} · ${container} via ${shell}\nType commands below. Full-screen apps (vi, top) are not supported.\n\n`);
        })
        .catch((err) => { if (isCurrentDrawerRequest(scope)) $('term-status').textContent = errMsg(err); });
});

$('btn-term-stop').addEventListener('click', stopExec);

$('term-input').addEventListener('keydown', (e) => {
    if (e.key !== 'Enter' || !execConnected) return;
    const cmd = e.target.value;
    appendTerm(`$ ${cmd}\n`);
    ExecWrite(cmd + '\n').catch((err) => appendTerm(`[write error: ${errMsg(err)}]\n`));
    e.target.value = '';
});

function stopExec() {
    if (execConnected) {
        execConnected = false;
        StopExec();
    }
    resetTerminalUI();
}

// ---- Port forward ----
let activeForwards = [];

EventsOn('portforward-closed', (key) => {
    activeForwards = activeForwards.filter((f) => f.key !== key);
    renderForwards();
});

function prepareForward() {
    $('pf-error').hidden = true;
    $('pf-local').value = '0';
    $('pf-remote').value = '';
    renderForwards();
}

$('btn-pf-start').addEventListener('click', () => {
    const ref = drawerRef;
    if (!ref) return;
    const remote = parseInt($('pf-remote').value, 10);
    const local = parseInt($('pf-local').value, 10) || 0;
    if (Number.isNaN(remote) || remote < 1 || remote > 65535) {
        showPfError('Enter a valid remote port (1–65535).');
        return;
    }
    $('pf-error').hidden = true;
    const btn = $('btn-pf-start');
    btn.disabled = true;
    btn.textContent = 'Starting…';
    StartPortForward(ref.kind, ref.namespace, ref.name, local, remote)
        .then((info) => {
            activeForwards.push(info);
            renderForwards();
        })
        .catch((err) => showPfError(errMsg(err)))
        .finally(() => { btn.disabled = false; btn.textContent = 'Start forward'; });
});

function showPfError(msg) {
    const el = $('pf-error');
    el.textContent = msg;
    el.hidden = false;
}

function renderForwards() {
    const ref = drawerRef;
    const list = $('pf-list');
    const mine = ref ? activeForwards.filter((f) => f.namespace === ref.namespace && f.name === ref.name) : [];
    $('pf-empty').hidden = mine.length > 0;
    list.innerHTML = '';
    for (const f of mine) {
        const div = document.createElement('div');
        div.className = 'pf-item';
        div.innerHTML =
            `<div class="pf-item-main">
                <span class="pf-addr mono">localhost:${f.localPort}</span>
                <span class="pf-arrow">→</span>
                <span class="mono">${esc(f.podName)}:${f.remotePort}</span>
            </div>`;
        const stop = document.createElement('button');
        stop.className = 'btn btn-danger btn-sm';
        stop.textContent = 'Stop';
        stop.addEventListener('click', () => {
            StopPortForward(f.key);
            activeForwards = activeForwards.filter((x) => x.key !== f.key);
            renderForwards();
        });
        div.appendChild(stop);
        list.appendChild(div);
    }
}

// ============ Generic modal (scale / create / import) ============

let modalOnOk = null;
let modalOnExtra = null;

// YAML editors living inside the current modal, keyed by host element id. They
// must be destroyed when the modal closes — closeModal wipes the body's HTML,
// and a CodeMirror instance left pointing at removed nodes leaks its listeners.
let modalEditors = {};

function mountModalEditor(id, { value = '', placeholder = '' } = {}) {
    const host = $(id);
    if (!host) return null;
    const ed = createYamlEditor(host, { value, placeholder });
    modalEditors[id] = ed;
    return ed;
}

function modalEditor(id) { return modalEditors[id] || null; }
function modalYaml(id) { return modalEditors[id] ? modalEditors[id].getValue() : ''; }
function setModalYaml(id, text) { if (modalEditors[id]) modalEditors[id].setValue(text); }

function openModal({ title, bodyHtml, okText = 'OK', onOk, onOpen, extraText = '', onExtra = null }) {
    $('modal-title').textContent = title;
    $('modal-body').innerHTML = bodyHtml;
    $('modal-ok').textContent = okText;
    $('modal-error').hidden = true;
    modalOnOk = onOk;
    // An optional secondary action (Import YAML's Preview). Hidden unless given.
    modalOnExtra = onExtra;
    const extra = $('modal-extra');
    extra.hidden = !onExtra;
    extra.disabled = false;
    if (onExtra) extra.textContent = extraText || 'More';
    $('modal-backdrop').hidden = false;
    $('modal').hidden = false;
    if (onOpen) onOpen();
}

function closeModal() {
    $('modal').hidden = true;
    $('modal-backdrop').hidden = true;
    for (const id of Object.keys(modalEditors)) modalEditors[id].destroy();
    modalEditors = {};
    $('modal-body').innerHTML = '';
    $('modal-extra').hidden = true;
    modalOnOk = null;
    modalOnExtra = null;
}

function modalError(msg) {
    const el = $('modal-error');
    el.textContent = msg;
    el.hidden = false;
}

function submitModal() {
    if (!modalOnOk) { closeModal(); return; }
    const btn = $('modal-ok');
    btn.disabled = true;
    $('modal-error').hidden = true;
    Promise.resolve()
        .then(() => modalOnOk())
        .then(() => closeModal())
        .catch((err) => modalError(errMsg(err)))
        .finally(() => { btn.disabled = false; });
}

// The secondary action runs in place — unlike OK it does not close the modal,
// because its whole purpose is to show you something before you commit.
$('modal-extra').addEventListener('click', () => {
    if (!modalOnExtra) return;
    const btn = $('modal-extra');
    btn.disabled = true;
    $('modal-error').hidden = true;
    Promise.resolve()
        .then(() => modalOnExtra())
        .catch((err) => modalError(errMsg(err)))
        .finally(() => { btn.disabled = false; });
});

$('modal-ok').addEventListener('click', submitModal);
$('modal-cancel').addEventListener('click', closeModal);
$('modal-close').addEventListener('click', closeModal);
$('modal-backdrop').addEventListener('click', closeModal);

// ---- Stackable alert/confirm dialog (replaces the native browser popups) ----
// Its own layer with a higher z-index so it can appear on top of an open modal
// (e.g. confirming a rollback from inside the Helm history modal).
let dialogResolve = null;

function openDialog({ title, message, icon, okText, cancelText, danger }) {
    return new Promise((resolve) => {
        $('dialog-title').textContent = title;
        $('dialog-icon').textContent = icon;
        $('dialog-message').textContent = message; // textContent → newlines preserved via CSS, no HTML injection
        $('dialog-ok').textContent = okText;
        const cancelBtn = $('dialog-cancel');
        cancelBtn.hidden = cancelText === null;
        if (cancelText !== null) cancelBtn.textContent = cancelText;
        $('dialog-ok').classList.toggle('btn-danger', !!danger);
        $('dialog-backdrop').hidden = false;
        $('dialog').hidden = false;
        dialogResolve = resolve;
        setTimeout(() => $('dialog-ok').focus(), 0);
    });
}

function closeDialog(result) {
    if ($('dialog').hidden) return;
    $('dialog').hidden = true;
    $('dialog-backdrop').hidden = true;
    $('dialog-ok').classList.remove('btn-danger');
    const resolve = dialogResolve;
    dialogResolve = null;
    if (resolve) resolve(result);
}

// A simple notice (one OK button). Returns a promise that resolves when dismissed.
function showAlert(message, { title = 'Notice', icon = 'ℹ️' } = {}) {
    return openDialog({ title, message, icon, okText: 'OK', cancelText: null });
}
// An error notice with warning styling.
function showError(message, title = 'Error') {
    recordError(message);
    return openDialog({ title, message, icon: '⚠️', okText: 'OK', cancelText: null });
}
// A confirm dialog. Resolves true (OK) or false (Cancel/Esc/backdrop).
function showConfirm(message, { title = 'Confirm', icon = '❓', okText = 'Confirm', danger = false } = {}) {
    return openDialog({ title, message, icon, okText, cancelText: 'Cancel', danger });
}

$('dialog-ok').addEventListener('click', () => closeDialog(true));
$('dialog-cancel').addEventListener('click', () => closeDialog(false));
$('dialog-backdrop').addEventListener('click', () => closeDialog(false));

// ---- Create / Import YAML ----

$('btn-create').addEventListener('click', () => openCreateModal(currentView));
$('btn-import').addEventListener('click', openImportModal);

function openCreateModal(view) {
    const kind = VIEW_KIND[view];
    if (!kind) { openImportModal(); return; }
    const ns = NAMESPACED_VIEWS.has(view) ? (currentNamespace || 'default') : '';
    openYamlApplyModal({
        title: `Create ${kind}`,
        okText: 'Create',
        value: templateFor(kind, ns),
        emptyMessage: 'The YAML is empty.',
    });
}

function openImportModal() {
    openYamlApplyModal({
        title: 'Import YAML',
        okText: 'Apply',
        placeholder: 'Paste any Kubernetes YAML here — built-in kinds and custom resources alike.\nSeparate multiple documents with a line containing only ---\nExisting resources are updated (create-or-update).',
        emptyMessage: 'Paste some YAML first.',
    });
}

// One modal serves both Create and Import YAML: they end in the same server-side
// apply, so they get the same editor and the same Preview. That matters most for
// Create — creating a name that already exists is an update, and the preview is
// what makes that visible beforehand instead of afterwards.
function openYamlApplyModal({ title, okText, value = '', placeholder = '', emptyMessage }) {
    openModal({
        title,
        okText,
        bodyHtml: `<div id="modal-yaml" class="yaml-host yaml-host-modal"></div>
                   <div id="modal-preview" class="pv" hidden></div>`,
        onOpen: () => {
            previewOpen = false;
            mountModalEditor('modal-yaml', { value, placeholder });
            modalEditor('modal-yaml').focus();
        },
        extraText: 'Preview',
        onExtra: toggleApplyPreview,
        onOk: () => {
            const v = modalYaml('modal-yaml').trim();
            if (!v) return Promise.reject(emptyMessage);
            return ApplyYAML(v).then(applyReport, (err) => {
                markApplyFailures(err);
                throw err;
            });
        },
    });
}

// A manifest can create several objects across several namespaces; the backend
// reports what it did to each one, so show that rather than closing silently.
function applyReport(report) {
    loadNamespaceOptions();
    refreshCurrentView();
    if (report) showAlert(report, { title: 'Applied', icon: '✅' });
}

// ---- Dry-run preview: what would this YAML change? ----

let previewOpen = false;

function toggleApplyPreview() {
    if (previewOpen) { showApplyEditor(); return Promise.resolve(); }

    const text = modalYaml('modal-yaml').trim();
    if (!text) return Promise.reject('There is no YAML to preview.');

    const panel = $('modal-preview');
    panel.innerHTML = '<p class="pv-summary">Asking the cluster what would change…</p>';
    $('modal-yaml').hidden = true;
    panel.hidden = false;
    $('modal-extra').textContent = 'Back to edit';
    previewOpen = true;

    return ApplyPreview(text)
        .then((d) => renderApplyPreview(panel, d))
        .catch((err) => { showApplyEditor(); throw err; });
}

function showApplyEditor() {
    previewOpen = false;
    const panel = $('modal-preview');
    if (panel) panel.hidden = true;
    const host = $('modal-yaml');
    if (host) host.hidden = false;
    $('modal-extra').textContent = 'Preview';
}

function renderApplyPreview(panel, d) {
    const docs = d?.docs ?? [];
    const counts = [];
    if (d.create) counts.push(`${d.create} create`);
    if (d.update) counts.push(`${d.update} update`);
    if (d.unchanged) counts.push(`${d.unchanged} unchanged`);
    if (d.failed) counts.push(`${d.failed} cannot be previewed`);

    const parts = [
        `<p class="pv-summary">${esc(counts.join(' · ') || 'Nothing to apply.')}</p>`,
        `<p class="pv-note">Nothing has been changed yet. This is the cluster's own answer, so it includes
         the defaults it fills in and anything admission would rewrite.</p>`,
    ];
    docs.forEach((doc, i) => {
        const badge = { create: 'pv-create', update: 'pv-update', unchanged: 'pv-unchanged' }[doc.action] || 'pv-error';
        const label = doc.action === 'error' ? 'cannot preview' : doc.action;
        let body = '';
        if (doc.error) body = `<p class="pv-msg">${esc(doc.error)}</p>`;
        else if (doc.action !== 'unchanged') body = `<pre class="pv-diff" data-pv="${i}"></pre>`;
        parts.push(`<div class="pv-doc">
            <div class="pv-head">
                <span class="pv-badge ${badge}">${esc(label)}</span>
                <span class="pv-ref">${esc(doc.ref)}</span>
            </div>${body}</div>`);
    });
    panel.innerHTML = parts.join('');

    docs.forEach((doc, i) => {
        const pre = panel.querySelector(`[data-pv="${i}"]`);
        if (pre) renderDiffInto(pre, doc.current, doc.proposed, { collapse: 3 });
    });
}

// After a failed apply, put each failure back on the line it came from: the
// report names "document 3", and the editor can say which lines those are.
function markApplyFailures(err) {
    const ed = modalEditor('modal-yaml');
    if (!ed) return;
    const failures = parseApplyFailures(errMsg(err));
    if (!failures.length) return;
    showApplyEditor();
    ed.markDocumentErrors(failures);
}

// Minimal starter templates per kind. `__NS__` lines are dropped for cluster-scoped kinds.
function templateFor(kind, ns) {
    const nsLine = ns ? `\n  namespace: ${ns}` : '';
    switch (kind) {
        case 'Namespace':
            return `apiVersion: v1\nkind: Namespace\nmetadata:\n  name: my-namespace\n`;
        case 'Pod':
            return `apiVersion: v1\nkind: Pod\nmetadata:\n  name: my-pod${nsLine}\nspec:\n  containers:\n    - name: main\n      image: nginx:alpine\n      ports:\n        - containerPort: 80\n`;
        case 'Deployment':
            return `apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: my-deployment${nsLine}\nspec:\n  replicas: 1\n  selector:\n    matchLabels:\n      app: my-deployment\n  template:\n    metadata:\n      labels:\n        app: my-deployment\n    spec:\n      containers:\n        - name: main\n          image: nginx:alpine\n          ports:\n            - containerPort: 80\n`;
        case 'Service':
            return `apiVersion: v1\nkind: Service\nmetadata:\n  name: my-service${nsLine}\nspec:\n  selector:\n    app: my-app\n  ports:\n    - port: 80\n      targetPort: 80\n`;
        case 'ConfigMap':
            return `apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: my-config${nsLine}\ndata:\n  key: value\n`;
        case 'Secret':
            return `apiVersion: v1\nkind: Secret\nmetadata:\n  name: my-secret${nsLine}\ntype: Opaque\nstringData:\n  key: value\n`;
        case 'ServiceAccount':
            return `apiVersion: v1\nkind: ServiceAccount\nmetadata:\n  name: my-serviceaccount${nsLine}\n`;
        case 'PersistentVolumeClaim':
            return `apiVersion: v1\nkind: PersistentVolumeClaim\nmetadata:\n  name: my-pvc${nsLine}\nspec:\n  accessModes:\n    - ReadWriteOnce\n  resources:\n    requests:\n      storage: 1Gi\n`;
        case 'StatefulSet':
            return `apiVersion: apps/v1\nkind: StatefulSet\nmetadata:\n  name: my-statefulset${nsLine}\nspec:\n  serviceName: my-statefulset\n  replicas: 1\n  selector:\n    matchLabels:\n      app: my-statefulset\n  template:\n    metadata:\n      labels:\n        app: my-statefulset\n    spec:\n      containers:\n        - name: main\n          image: nginx:alpine\n`;
        case 'DaemonSet':
            return `apiVersion: apps/v1\nkind: DaemonSet\nmetadata:\n  name: my-daemonset${nsLine}\nspec:\n  selector:\n    matchLabels:\n      app: my-daemonset\n  template:\n    metadata:\n      labels:\n        app: my-daemonset\n    spec:\n      containers:\n        - name: main\n          image: nginx:alpine\n`;
        case 'Job':
            return `apiVersion: batch/v1\nkind: Job\nmetadata:\n  name: my-job${nsLine}\nspec:\n  template:\n    spec:\n      restartPolicy: Never\n      containers:\n        - name: main\n          image: busybox\n          command: ["sh", "-c", "echo hello && sleep 5"]\n`;
        case 'CronJob':
            return `apiVersion: batch/v1\nkind: CronJob\nmetadata:\n  name: my-cronjob${nsLine}\nspec:\n  schedule: "*/5 * * * *"\n  jobTemplate:\n    spec:\n      template:\n        spec:\n          restartPolicy: OnFailure\n          containers:\n            - name: main\n              image: busybox\n              command: ["sh", "-c", "date"]\n`;
        case 'Ingress':
            return `apiVersion: networking.k8s.io/v1\nkind: Ingress\nmetadata:\n  name: my-ingress${nsLine}\nspec:\n  rules:\n    - host: example.local\n      http:\n        paths:\n          - path: /\n            pathType: Prefix\n            backend:\n              service:\n                name: my-service\n                port:\n                  number: 80\n`;
        case 'PersistentVolume':
            return `apiVersion: v1\nkind: PersistentVolume\nmetadata:\n  name: my-pv\nspec:\n  capacity:\n    storage: 1Gi\n  accessModes:\n    - ReadWriteOnce\n  hostPath:\n    path: /mnt/data\n`;
        case 'StorageClass':
            return `apiVersion: storage.k8s.io/v1\nkind: StorageClass\nmetadata:\n  name: my-storageclass\nprovisioner: kubernetes.io/no-provisioner\nreclaimPolicy: Delete\nvolumeBindingMode: WaitForFirstConsumer\n`;
        case 'Role':
            return `apiVersion: rbac.authorization.k8s.io/v1\nkind: Role\nmetadata:\n  name: my-role${nsLine}\nrules:\n  - apiGroups: [""]\n    resources: ["pods"]\n    verbs: ["get", "list", "watch"]\n`;
        case 'RoleBinding':
            return `apiVersion: rbac.authorization.k8s.io/v1\nkind: RoleBinding\nmetadata:\n  name: my-rolebinding${nsLine}\nroleRef:\n  apiGroup: rbac.authorization.k8s.io\n  kind: Role\n  name: my-role\nsubjects:\n  - kind: ServiceAccount\n    name: default\n    namespace: ${ns || 'default'}\n`;
        case 'ClusterRole':
            return `apiVersion: rbac.authorization.k8s.io/v1\nkind: ClusterRole\nmetadata:\n  name: my-clusterrole\nrules:\n  - apiGroups: [""]\n    resources: ["nodes"]\n    verbs: ["get", "list", "watch"]\n`;
        case 'ClusterRoleBinding':
            return `apiVersion: rbac.authorization.k8s.io/v1\nkind: ClusterRoleBinding\nmetadata:\n  name: my-clusterrolebinding\nroleRef:\n  apiGroup: rbac.authorization.k8s.io\n  kind: ClusterRole\n  name: my-clusterrole\nsubjects:\n  - kind: ServiceAccount\n    name: default\n    namespace: default\n`;
        case 'ResourceQuota':
            return `apiVersion: v1\nkind: ResourceQuota\nmetadata:\n  name: my-quota${nsLine}\nspec:\n  hard:\n    pods: "10"\n    requests.cpu: "2"\n    requests.memory: 2Gi\n    limits.cpu: "4"\n    limits.memory: 4Gi\n`;
        case 'LimitRange':
            return `apiVersion: v1\nkind: LimitRange\nmetadata:\n  name: my-limitrange${nsLine}\nspec:\n  limits:\n    - type: Container\n      default:\n        cpu: 500m\n        memory: 512Mi\n      defaultRequest:\n        cpu: 100m\n        memory: 128Mi\n`;
        default:
            return `apiVersion: v1\nkind: ${kind}\nmetadata:\n  name: my-${kind.toLowerCase()}${nsLine}\n`;
    }
}

// Esc closes the drawer or the modal; Enter submits a simple modal.
document.addEventListener('keydown', (e) => {
    // The stackable alert/confirm dialog takes precedence over everything.
    if (!$('dialog').hidden) {
        if (e.key === 'Escape') { e.preventDefault(); closeDialog(false); }
        else if (e.key === 'Enter') { e.preventDefault(); closeDialog(true); }
        return;
    }
    // A CodeMirror editor handles some of these keys itself — Escape closes its
    // search panel and releases Tab back to focus navigation. Its listener sits on
    // an element inside document, so it has already run; if it acted on the key it
    // called preventDefault, and this handler must not act on the same press.
    if (e.defaultPrevented) return;

    if (e.key === 'Escape') {
        if (!$('modal').hidden) { closeModal(); return; }
        if (!$('drawer').hidden) closeDrawer();
        return;
    }
    // Enter submits the modal only when focus is NOT in something that owns Enter:
    // a textarea, an input that opts out (.no-enter-submit), or the YAML editor —
    // whose editable element is a contenteditable div, not a textarea, so checking
    // the tag name alone would let Enter apply a half-typed manifest.
    if (e.key === 'Enter' && !$('modal').hidden
        && e.target.tagName !== 'TEXTAREA'
        && !(e.target.closest && e.target.closest('.cm-editor'))
        && !e.target.classList.contains('no-enter-submit')) {
        e.preventDefault();
        submitModal();
    }
});

// ============ Right-sizing: requested vs actually used ============
//
// The backend (rightsizing.go) does the joining and the judging; this only
// renders. Two things it must not do: show a missing value as 0 (the backend
// sends -1, which means "not declared" and is the whole point of several
// findings), and imply usage exists when metrics-server is absent.

const SZ_UNSET = -1;
let sizingReport = null;

function loadSizing(scope) {
    const ns = scope.namespace;
    return Sizing(ns)
        .then((r) => {
            if (!isCurrentViewRequest(scope)) return;
            sizingReport = r;
            renderSizing(r);
        })
        .catch((err) => {
            if (!isCurrentViewRequest(scope)) return;
            sizingReport = null;
            $('sz-bars').innerHTML = `<p class="error">${esc(errMsg(err))}</p>`;
            $('sz-ns-body').innerHTML = '';
            $('sz-cont-body').innerHTML = '';
        });
}

function renderSizing(r) {
    const t = r.totals ?? {};

    $('sz-scope').textContent = [
        `${t.pods ?? 0} pods · ${t.containers ?? 0} containers`,
        r.nodes ? `${r.nodes} ready node${r.nodes === 1 ? '' : 's'}` : '',
    ].filter(Boolean).join(' · ');

    $('sz-bars').innerHTML =
        szTile('CPU', t.cpuRequest, t.cpuUsage, r.allocCpu, r.cpuReservedPct, szCPU, 'cores', r.metricsAvailable)
        + szTile('Memory', t.memRequest, t.memUsage, r.allocMem, r.memReservedPct, szMem, '', r.metricsAvailable);

    const note = $('sz-note');
    note.textContent = r.note || '';
    note.hidden = !r.note;

    const advice = r.advice ?? [];
    $('sz-advice-card').hidden = advice.length === 0;
    $('sz-advice').innerHTML = advice.map((a) => `<li>${esc(a)}</li>`).join('');

    const namespaces = r.namespaces ?? [];
    $('sz-ns-total').textContent = `${namespaces.length} namespace${namespaces.length === 1 ? '' : 's'} in scope`;
    $('sz-ns-body').innerHTML = namespaces.map((ns) => {
        const quotaBits = [];
        if ((ns.quota ?? []).length) quotaBits.push(`${ns.quota.length} quota`);
        if (ns.limitRanges) quotaBits.push(`${ns.limitRanges} LimitRange`);
        const quota = quotaBits.length
            ? quotaBits.map((b) => `<span class="chip">${esc(b)}</span>`).join(' ')
            : '<span class="sz-ok">none</span>';
        const undeclared = ns.undeclared
            ? `<span class="sz-undeclared" title="containers missing a CPU request, a memory request or a memory limit">${ns.undeclared} of ${ns.containers}</span>`
            : '<span class="sz-ok">—</span>';
        return `<tr>
            <td>${esc(ns.namespace)}</td>
            <td class="sz-num">${ns.pods ?? 0}</td>
            <td>${szCell(ns.cpuRequest, ns.cpuUsage, szCPU, r.metricsAvailable)}</td>
            <td>${szCell(ns.memRequest, ns.memUsage, szMem, r.metricsAvailable)}</td>
            <td class="sz-num">${undeclared}</td>
            <td>${quota}</td>
        </tr>`;
    }).join('');
    $('sz-ns-empty').hidden = namespaces.length > 0;

    renderSizingContainers();
}

// One stat tile per resource — because the story here IS one number.
//
// Two bar layouts were tried and both failed. The first scaled the bars to the
// cluster's allocatable capacity, which pushed both quantities into the leftmost 3%
// of a very wide bar. The second gave requested and used a bar each on a shared
// scale — but a muted fill for "requested" reads as an *empty track*, so the tile
// looked like a bar that had not loaded.
//
// The form was wrong both times. "A ratio against a limit" is a **meter**: the
// reservation is the TRACK, the usage is the FILL. One bar, and the track's meaning
// is unambiguous because it is the limit. The headline is the consequence in the
// resource's own unit — "924m reserved and idle" — since that is the number a reader
// would otherwise have to compute by subtraction.
function szTile(label, requested, used, alloc, reservedPct, fmt, unit, haveMetrics) {
    const req = requested > 0 ? requested : 0;
    const act = haveMetrics && used > 0 ? used : 0;

    let hero = '—';
    let heroNote = 'no usage data';
    let state = '';
    if (!haveMetrics) {
        heroNote = 'metrics-server unavailable, so usage is unknown';
    } else if (req === 0 && act === 0) {
        hero = 'nothing';
        heroNote = 'nothing requested, nothing used';
    } else if (req === 0) {
        hero = fmt(act);
        heroNote = 'used with nothing requested — running on unreserved capacity';
        state = 'sz-state-warn';
    } else if (act > req) {
        hero = fmt(act - req);
        heroNote = 'more than it reserved';
        state = 'sz-state-warn';
    } else {
        hero = fmt(req - act);
        heroNote = 'reserved and sitting idle';
        // Deliberately NOT the warning colour. Idle capacity is waste, not a hazard;
        // consuming beyond a reservation is. Two amber display numbers side by side
        // made both mean nothing — the status colour is reserved for the one that can
        // actually break something. The mostly-empty meter carries the waste instead.
        state = '';
    }

    // Meter: track = the reservation. A fill past 100% is clamped and recoloured —
    // the exact overage is the headline above, so the bar does not need to lie about
    // its own scale to carry it.
    let meter = '';
    if (haveMetrics && req > 0) {
        const pct = Math.min(100, (act * 100) / req);
        meter = `<span class="sz-meter" role="img"
                       aria-label="${esc(fmt(act))} used of ${esc(fmt(req))} reserved">
            ${act > 0 ? `<span class="sz-meter-fill${act > req ? ' sz-meter-over' : ''}"
                               style="width:${pct.toFixed(1)}%"></span>` : ''}
        </span>`;
    }

    const figures = haveMetrics
        ? `<b>${esc(fmt(act))}</b> used of <b>${esc(fmt(req))}</b> reserved`
        : `<b>${esc(fmt(req))}</b> reserved`;

    return `<div class="sz-tile ${state}">
        <div class="sz-tile-head">
            <span class="sz-tile-label">${esc(label)}</span>
            ${alloc > 0 ? `<span class="sz-tile-share">${reservedPct}% of the cluster's
                ${esc(fmt(alloc))}${unit ? ' ' + unit : ''}</span>` : ''}
        </div>
        <p class="sz-tile-hero">${esc(hero)}</p>
        <p class="sz-tile-note">${esc(heroNote)}</p>
        ${meter}
        <p class="sz-tile-figures">${figures}</p>
    </div>`;
}

// The same paired bars shrunk into a table cell.
//
// Note what this deliberately does NOT say: when nothing was requested it says so,
// rather than printing "9Mi of 0Mi" — a ratio against zero is not a fact, and the
// earlier version rendered it as one.
function szCell(requested, used, fmt, haveMetrics) {
    const req = requested > 0 ? requested : 0;
    const act = haveMetrics && used > 0 ? used : 0;

    if (!haveMetrics) {
        return `<span class="sz-cell-nums">${req ? `<b>${esc(fmt(req))}</b> requested` : '<span class="sz-ok">none requested</span>'}</span>`;
    }
    if (req === 0 && act === 0) return '<span class="sz-cell-nums sz-ok">—</span>';

    const nums = req === 0
        ? `<b>${esc(fmt(act))}</b> used · <span class="sz-undeclared">no request</span>`
        : `<b>${esc(fmt(act))}</b> used · ${esc(fmt(req))} req`;

    // The same meter as the tiles, one bar: track = reserved, fill = used. No bar at
    // all when nothing was reserved, because there is no limit to be a ratio of.
    let meter = '';
    if (req > 0 && act > 0) {
        const pct = Math.min(100, (act * 100) / req);
        meter = `<span class="sz-cell-meter"><span class="sz-cell-fill${act > req ? ' sz-meter-over' : ''}"
                 style="width:${pct.toFixed(1)}%"></span></span>`;
    }
    return `<span class="sz-cell-nums">${nums}</span>${meter}`;
}

// Split out so the "only serious" toggle can re-render without refetching.
function renderSizingContainers() {
    const r = sizingReport;
    if (!r) return;
    const seriousOnly = $('sz-only-serious').checked;
    // Severity ≥ 4 is "OOMKilled" or "memory near its limit" — see severity() in
    // rightsizing.go. Anything below that is a declaration problem, which the
    // advice list already states once instead of once per row.
    const rows = (r.containers ?? []).filter((c) => !seriousOnly || (c.severity ?? 0) >= 4);

    $('sz-cont-count').textContent = seriousOnly
        ? `${rows.length} of ${(r.containers ?? []).length}`
        : `${rows.length}`;
    const haveMetrics = r.metricsAvailable;
    // Namespace/pod/container in one cell: three separate columns cost width the
    // findings column needs, and they are read as one identity anyway.
    $('sz-cont-body').innerHTML = rows.map((c) => `<tr class="${(c.severity ?? 0) >= 4 ? 'sz-row-bad' : ''}">
        <td><span class="sz-ref">
            <span class="sz-ref-name">${esc(c.container)}</span>
            <span class="sz-ref-sub">${esc(c.namespace)} / ${esc(c.pod)}</span>
        </span></td>
        <td><span class="chip">${esc(c.qos || '—')}</span></td>
        <td>${szCell(c.cpuRequest, c.cpuUsage, szCPU, haveMetrics)}</td>
        <td>${szCell(c.memRequest, c.memUsage, szMem, haveMetrics)}</td>
        <td>${(c.findings ?? []).map((f) => `<div class="sz-finding">${esc(f)}</div>`).join('')}</td>
    </tr>`).join('');
    $('sz-cont-empty').hidden = rows.length > 0;
}

$('sz-only-serious').addEventListener('change', renderSizingContainers);

// A dash, never a zero: the backend sends -1 for "the container never said".
function szCPU(milli) {
    if (milli === undefined || milli === null || milli === SZ_UNSET) return '—';
    return milli < 1000 ? `${milli}m` : (milli / 1000).toFixed(2);
}

function szMem(bytes) {
    if (bytes === undefined || bytes === null || bytes === SZ_UNSET) return '—';
    const mi = 1024 * 1024;
    return bytes < 1024 * mi ? `${Math.round(bytes / mi)}Mi` : `${(bytes / (1024 * mi)).toFixed(1)}Gi`;
}


// ============ What this token may do (RBAC gating) ============
//
// The cluster is asked once per kind+namespace via SelfSubjectAccessReview
// (access.go) and the answer is cached on both sides. An action the token cannot
// perform is disabled with the reason in its tooltip, instead of being offered
// and failing at a 403 after a confirmation dialog.
//
// The rule when the answer has not arrived yet, or the cluster would not answer:
// **allow**. A button greyed out for someone who does have permission is a worse
// bug than a click that fails — see AccessSet.Checked in access.go.

const accessResolved = new Map(); // key -> AccessSet | null
const accessPending = new Map();  // key -> Promise, so a burst asks once

function accessKey(kind, namespace) { return `${kind}|${namespace ?? ''}`; }

function accessSet(kind, namespace) {
    const key = accessKey(kind, namespace);
    if (accessResolved.has(key)) return Promise.resolve(accessResolved.get(key));
    if (!accessPending.has(key)) {
        accessPending.set(key, CanI(kind, namespace ?? '')
            .catch(() => null)
            .then((set) => { accessResolved.set(key, set); accessPending.delete(key); return set; }));
    }
    return accessPending.get(key);
}

// Synchronous read for code that must not wait (building a menu on click).
function accessPeek(kind, namespace) {
    return accessResolved.get(accessKey(kind, namespace)) ?? null;
}

// Mirrors AccessSet.Allowed in access.go: no answer, or no entry, means allowed.
function allowed(set, verb) {
    if (!verb || !set || !set.verbs) return true;
    return set.verbs[verb] !== false;
}

function denyReason(kind, namespace, verb) {
    const where = namespace ? ` in ${namespace}` : '';
    return `Your token cannot ${verb} ${String(kind).split('.')[0]}${where}.`;
}

// A different cluster means different permissions; the Go-side cache is per
// connection, so only this one needs clearing.
function clearAccessCache() {
    accessResolved.clear();
    accessPending.clear();
}

// gate disables a control and explains why, restoring its own tooltip when the
// block lifts.
function gate(el, ok, reason) {
    if (!el) return;
    if (el.dataset.title0 === undefined) el.dataset.title0 = el.title || '';
    el.disabled = !ok;
    el.title = ok ? el.dataset.title0 : reason;
}

// ============ Row actions menu ============

function openRowMenu(btn, ref) {
    const menu = $('row-menu');
    let actions;
    if (ref.kind === 'HelmRelease') {
        actions = [
            { label: 'Values & manifest', run: () => openHelmDetailModal(ref) },
            { label: 'History & rollback…', run: () => openHelmHistoryModal(ref) },
            { label: 'Upgrade values…', run: () => openHelmUpgradeModal(ref) },
            { label: 'Uninstall', danger: true, run: () => uninstallHelm(ref) },
        ];
        renderRowMenu(menu, btn, actions);
        return;
    }
    // `need` is the verb the action requires — see the RBAC gating block above.
    // It is the verb the *API* wants, not the one the label suggests: a rolling
    // restart is a patch, and scaling is an update of the scale subresource.
    actions = [
        { label: 'Open details', need: 'get', run: () => { openDrawer(ref); setDrawerTab('details'); } },
        { label: 'Edit YAML', need: 'get', run: () => { openDrawer(ref); setDrawerTab('yaml'); } },
    ];
    if (ref.isPod) {
        actions.push({ label: 'View logs', need: 'logs', run: () => { openDrawer(ref); setDrawerTab('logs'); } });
        actions.push({ label: 'Terminal', need: 'exec', run: () => { openDrawer(ref); setDrawerTab('terminal'); } });
    }
    if (ref.kind === 'Deployment') {
        actions.push({ label: 'Scale…', need: 'update', run: () => scaleRef(ref) });
        actions.push({ label: 'Restart', need: 'patch', run: () => restartRef(ref) });
        actions.push({ label: 'Rollout history…', need: 'get', run: () => openRolloutModal(ref) });
        actions.push({ label: 'Pause rollout', need: 'patch', run: () => pauseRef(ref, true) });
        actions.push({ label: 'Resume rollout', need: 'patch', run: () => pauseRef(ref, false) });
    }
    if (ref.kind === 'StatefulSet') {
        actions.push({ label: 'Restart', need: 'patch', run: () => restartWorkload(ref, RestartStatefulSet) });
    }
    if (ref.kind === 'DaemonSet') {
        actions.push({ label: 'Restart', need: 'patch', run: () => restartWorkload(ref, RestartDaemonSet) });
    }
    if (ref.kind === 'Node') {
        actions.push({ label: 'Cordon', need: 'patch', run: () => nodeSchedule(ref, false) });
        actions.push({ label: 'Uncordon', need: 'patch', run: () => nodeSchedule(ref, true) });
        // Drain also needs create on pods/eviction in every namespace it touches,
        // which cannot be known from here; `patch` on the Node is the part that is
        // checkable, and it is the step drain does first.
        actions.push({ label: 'Drain', danger: true, need: 'patch', run: () => drainRef(ref) });
    }
    if (ref.kind === 'CronJob') {
        // Triggering creates a *Job*, so the CronJob's own verbs say nothing here.
        actions.push({ label: 'Trigger now (run)', need: 'create', needKind: 'Job', run: () => runCronNow(ref) });
    }
    actions.push({ label: '✨ Ask AI', need: 'get', run: () => openDrawer({ ...ref, tab: 'ai' }) });
    actions.push({ label: 'Delete', danger: true, need: 'delete', run: () => deleteRef(ref) });
    applyMenuAccess(actions, ref);
    renderRowMenu(menu, btn, actions);
}

// Mark the actions this token cannot perform. Synchronous by design — the menu
// must open on click — so it reads whatever answer has already arrived (warmed
// when the view opened) and leaves the rest enabled.
function applyMenuAccess(actions, ref) {
    for (const a of actions) {
        if (!a.need) continue;
        const kind = a.needKind || ref.kind;
        if (a.needKind) accessSet(kind, ref.namespace); // warm it for next time
        if (!allowed(accessPeek(kind, ref.namespace), a.need)) {
            a.disabled = denyReason(kind, ref.namespace, a.need);
        }
    }
}

// Populate and position the floating row menu near its button.
function renderRowMenu(menu, btn, actions) {
    menu.innerHTML = actions.map((a, i) =>
        `<button class="row-menu-item${a.danger ? ' danger' : ''}" data-i="${i}"${
            a.disabled ? ` disabled title="${esc(a.disabled)}"` : ''}>${esc(a.label)}</button>`).join('');
    menu.querySelectorAll('.row-menu-item').forEach((el) => {
        el.addEventListener('click', () => { closeRowMenu(); actions[parseInt(el.dataset.i, 10)].run(); });
    });
    const r = btn.getBoundingClientRect();
    menu.hidden = false;
    const mw = menu.offsetWidth || 180;
    let left = r.right - mw;
    if (left < 8) left = 8;
    let top = r.bottom + 4;
    if (top + menu.offsetHeight > window.innerHeight - 8) top = r.top - menu.offsetHeight - 4;
    menu.style.left = `${left}px`;
    menu.style.top = `${top}px`;
}

function closeRowMenu() { $('row-menu').hidden = true; }

function deleteRef(ref) {
    showConfirm(`Delete ${ref.kind} “${ref.name}”${ref.namespace ? ` in ${ref.namespace}` : ''}?\nThis cannot be undone.`, { title: `Delete ${ref.kind}`, icon: '🗑', okText: 'Delete', danger: true }).then((ok) => {
        if (!ok) return;
        DeleteResource(ref.kind, ref.namespace, ref.name)
            .then(() => { loadSidebarCounts(); refreshCurrentView(); })
            .catch((err) => showError(errMsg(err)));
    });
}

function pauseRef(ref, paused) {
    SetDeploymentPaused(ref.namespace, ref.name, paused)
        .then(() => refreshCurrentView())
        .catch((err) => showError(errMsg(err)));
}

function nodeSchedule(ref, schedulable) {
    SetNodeSchedulable(ref.name, schedulable)
        .then(() => refreshCurrentView())
        .catch((err) => showError(errMsg(err)));
}

function drainRef(ref) {
    showConfirm(`Drain node “${ref.name}”?\nThis cordons it and evicts its pods (DaemonSet pods are kept).`, { title: 'Drain node', icon: '🚰', okText: 'Drain', danger: true }).then((ok) => {
        if (!ok) return;
        DrainNode(ref.name)
            .then(() => refreshCurrentView())
            .catch((err) => showError(errMsg(err)));
    });
}

function runCronNow(ref) {
    showConfirm(`Trigger CronJob “${ref.name}” now (create a Job)?`, { title: 'Trigger CronJob', icon: '⏱', okText: 'Trigger' }).then((ok) => {
        if (!ok) return;
        RunCronJobNow(ref.namespace, ref.name)
            .then(() => { loadSidebarCounts(); showAlert('Job created.', { title: 'Triggered', icon: '✅' }); })
            .catch((err) => showError(errMsg(err)));
    });
}

// Rollout history modal with per-revision Rollback.
function openRolloutModal(ref) {
    openModal({
        title: `Rollout history — ${ref.name}`,
        okText: 'Close',
        bodyHtml: `<div id="rollout-list" class="rollout-list"><p class="empty-inline">Loading…</p></div>`,
        onOk: () => Promise.resolve(),
    });
    RolloutHistory(ref.namespace, ref.name)
        .then((revs) => {
            const box = $('rollout-list');
            if (!revs || revs.length === 0) { box.innerHTML = '<p class="empty-inline">No revisions.</p>'; return; }
            box.innerHTML = revs.map((r) =>
                `<div class="rollout-row">
                    <div>
                        <span class="rollout-rev">Revision ${r.revision}${r.current ? ' <span class="chip">current</span>' : ''}</span>
                        <div class="rollout-images mono">${esc(r.images)}</div>
                        <div class="rollout-age">${esc(r.name)} · ${esc(r.age)}</div>
                    </div>
                    ${r.current ? '' : `<button class="btn btn-secondary btn-sm rollback-btn" data-rev="${r.revision}">Rollback</button>`}
                </div>`).join('');
            box.querySelectorAll('.rollback-btn').forEach((btn) => {
                btn.addEventListener('click', () => {
                    const rev = parseInt(btn.dataset.rev, 10);
                    showConfirm(`Rollback “${ref.name}” to revision ${rev}?`, { title: 'Rollback deployment', icon: '↩', okText: 'Rollback' }).then((ok) => {
                        if (!ok) return;
                        btn.disabled = true;
                        RollbackDeployment(ref.namespace, ref.name, rev)
                            .then(() => { closeModal(); refreshCurrentView(); })
                            .catch((err) => { showError(errMsg(err)); btn.disabled = false; });
                    });
                });
            });
        })
        .catch((err) => { $('rollout-list').innerHTML = `<p class="error">${esc(errMsg(err))}</p>`; });
}

// Close the row menu on any outside click / scroll / Esc.
document.addEventListener('click', (e) => {
    if (!$('row-menu').hidden && !e.target.classList.contains('row-actions-btn')) closeRowMenu();
});
document.addEventListener('scroll', closeRowMenu, true);

// ============ Bulk select + sortable columns ============

// These two selectors decorate *resource-list* tables — bulk-select, Age, Actions.
//
// **A table that is not a resource list must carry `class="plain"`.** This used to
// be an id-based exclusion list, which meant every new non-list view had to
// remember to add itself to two selectors — and a view that forgot got phantom
// columns in its header, shifting every row one cell left and pushing the page
// into a horizontal scroll. That happened to ResourceQuotas once and to
// Right-sizing again. Opting out by class puts the decision next to the markup.
document.querySelectorAll('#content .view:not(#view-overview):not(#view-helmrepos) table:not(.plain) thead tr').forEach((tr) => {
    tr.insertAdjacentHTML('afterbegin', '<th class="no-sort col-check"><input type="checkbox" class="select-all" title="Select all"></th>');
});
// Overview cards, Pods, and Helm Repos define their own columns.
document.querySelectorAll('#content .view:not(#view-overview):not(#view-pods):not(#view-helmrepos) table:not(.plain) thead tr').forEach((tr) => {
    tr.insertAdjacentHTML('beforeend', '<th>Age</th><th class="no-sort col-actions">Actions</th>');
});

// Make every resource-view table header clickable to sort its rows.
document.querySelectorAll('#content .view thead th').forEach((th) => {
    if (th.classList.contains('no-sort')) return;
    th.classList.add('sortable');
    th.addEventListener('click', () => sortTable(th));
});

// ---- Selection state + bulk actions ----
const selectedRows = new Map(); // key -> ref

function selKey(ref) { return `${ref.kind}/${ref.namespace}/${ref.name}`; }

function toggleRowSelection(ref, checked, tr) {
    if (checked) { selectedRows.set(selKey(ref), ref); tr.classList.add('selected'); }
    else { selectedRows.delete(selKey(ref)); tr.classList.remove('selected'); }
    updateBulkBar();
}

function clearSelection() {
    selectedRows.clear();
    document.querySelectorAll('.row-check:checked').forEach((cb) => { cb.checked = false; });
    document.querySelectorAll('.select-all:checked').forEach((cb) => { cb.checked = false; });
    document.querySelectorAll('tr.selected').forEach((tr) => tr.classList.remove('selected'));
    updateBulkBar();
}

function updateBulkBar() {
    const bar = $('bulk-bar');
    const n = selectedRows.size;
    $('bulk-count').textContent = `${n} selected`;
    bar.hidden = n === 0;
}

// Select-all toggles every visible (non-filtered) row checkbox in that table.
document.addEventListener('change', (e) => {
    if (!e.target.classList.contains('select-all')) return;
    const checked = e.target.checked;
    e.target.closest('table').querySelectorAll('tbody .row-check').forEach((cb) => {
        const tr = cb.closest('tr');
        if (tr.hidden) return;
        cb.checked = checked;
        toggleRowSelection(cb.__ref, checked, tr);
    });
});

$('bulk-clear').addEventListener('click', clearSelection);
$('bulk-delete').addEventListener('click', () => {
    const refs = [...selectedRows.values()];
    if (refs.length === 0) return;
    showConfirm(`Delete ${refs.length} selected resource(s)?\nThis cannot be undone.`, { title: 'Delete selected', icon: '🗑', okText: `Delete ${refs.length}`, danger: true }).then((ok) => {
        if (!ok) return;
        Promise.allSettled(refs.map((r) => r.kind === 'HelmRelease'
            ? HelmUninstall(r.namespace, r.name)
            : DeleteResource(r.kind, r.namespace, r.name)))
            .then((results) => {
                const failed = results.filter((r) => r.status === 'rejected').length;
                clearSelection();
                loadSidebarCounts();
                refreshCurrentView();
                if (failed) showError(`${failed} of ${refs.length} could not be deleted.`);
            });
    });
});

function sortTable(th) {
    const headRow = th.parentNode;
    const idx = [...headRow.children].indexOf(th);
    const table = th.closest('table');
    const tbody = table.querySelector('tbody');
    const rows = [...tbody.querySelectorAll('tr')];
    const asc = th.getAttribute('data-sort') !== 'asc';

    // Reset indicators on sibling headers.
    for (const h of headRow.children) {
        h.removeAttribute('data-sort');
        const ind = h.querySelector('.sort-ind');
        if (ind) ind.remove();
    }
    th.setAttribute('data-sort', asc ? 'asc' : 'desc');
    const ind = document.createElement('span');
    ind.className = 'sort-ind';
    ind.textContent = asc ? ' ↑' : ' ↓';
    th.appendChild(ind);

    rows.sort((a, b) => {
        const av = cellSortValue(a, idx), bv = cellSortValue(b, idx);
        if (typeof av === 'number' && typeof bv === 'number') return asc ? av - bv : bv - av;
        return asc ? String(av).localeCompare(String(bv)) : String(bv).localeCompare(String(av));
    });
    for (const r of rows) tbody.appendChild(r);
}

function cellSortValue(tr, idx) {
    const td = tr.children[idx];
    if (!td) return '';
    const t = td.textContent.trim();
    // Numeric-ish (5.89m, 287Mi, ages like 12h) → sort by leading number.
    const num = parseFloat(t.replace(/[^0-9.\-].*$/, ''));
    if (/^[0-9]/.test(t) && !Number.isNaN(num)) return num;
    return t.toLowerCase();
}

// ============ Command palette (Ctrl+K) ============

let paletteFiltered = [];
let paletteSel = 0;

function buildPaletteCommands() {
    const cmds = [];
    for (const [view, title] of Object.entries(PAGE_TITLES)) {
        cmds.push({ kind: 'Go to', label: title, run: () => selectView(view) });
    }
    for (const opt of $('namespace-select').options) {
        const label = opt.value === '' ? 'All namespaces' : opt.value;
        cmds.push({ kind: 'Namespace', label, run: () => {
            $('namespace-select').value = opt.value;
            currentNamespace = opt.value;
            updateNsScope();
            loadSidebarCounts({ includeCluster: false });
            refreshCurrentView();
        } });
    }
    for (const opt of $('cluster-select').options) {
        cmds.push({ kind: 'Cluster', label: `Switch to ${opt.value}`, run: () => {
            $('cluster-select').value = opt.value;
            $('cluster-select').dispatchEvent(new Event('change'));
        } });
    }
    cmds.push({ kind: 'Action', label: 'Create resource', run: () => openCreateModal(currentView) });
    cmds.push({ kind: 'Action', label: 'Import YAML', run: openImportModal });
    cmds.push({ kind: 'Action', label: 'Refresh', run: () => { loadSidebarCounts(); refreshCurrentView(); } });
    cmds.push({ kind: 'Action', label: 'Add cluster', run: () => $('btn-add-cluster').click() });
    cmds.push({ kind: 'Action', label: 'Disconnect cluster', run: () => $('btn-disconnect').click() });
    return cmds;
}

function openPalette() {
    if ($('dashboard').hidden) return; // palette only makes sense inside the dashboard
    paletteAll = buildPaletteCommands();
    paletteHits = [];
    paletteSel = 0;
    $('palette-input').value = '';
    $('palette-backdrop').hidden = false;
    $('palette').hidden = false;
    renderPalette();
    $('palette-input').focus();
}
let paletteAll = [];
let paletteHits = [];          // live resource-search results (async, from SearchResources)
let paletteSearchTimer = null;

function closePalette() {
    $('palette').hidden = true;
    $('palette-backdrop').hidden = true;
    clearTimeout(paletteSearchTimer);
}

// Navigate to a searched resource: scope its namespace, open its view + drawer.
function gotoHit(h) {
    if (h.namespace) setNamespaceScope(h.namespace);
    selectView(h.view);
    // A custom-resource view id carries the group-qualified kind, which is what
    // the drawer needs to address the object unambiguously.
    const kind = String(h.view).startsWith('custom:') ? h.view.slice('custom:'.length) : h.kind;
    openDrawer({ kind, namespace: h.namespace, name: h.name, isPod: h.kind === 'Pod' });
}

// Debounced global resource search feeding extra palette rows.
function schedulePaletteSearch(term) {
    clearTimeout(paletteSearchTimer);
    if (term.length < 2) { paletteHits = []; return; }
    paletteSearchTimer = setTimeout(() => {
        SearchResources(term)
            .then((hits) => {
                paletteHits = (hits || []).map((h) => ({
                    kind: `${h.kind}${h.namespace ? ' · ' + h.namespace : ''}`,
                    label: h.name,
                    resource: true,
                    run: () => gotoHit(h),
                }));
                renderPalette();
            })
            .catch(() => { paletteHits = []; });
    }, 250);
}

function renderPalette() {
    const term = $('palette-input').value.trim().toLowerCase();
    const staticMatches = term ? paletteAll.filter((c) => `${c.label} ${c.kind}`.toLowerCase().includes(term)) : paletteAll;
    paletteFiltered = staticMatches.concat(paletteHits);
    if (paletteSel >= paletteFiltered.length) paletteSel = 0;
    $('palette-empty').hidden = paletteFiltered.length > 0;
    const list = $('palette-list');
    list.innerHTML = paletteFiltered.map((c, i) =>
        `<div class="palette-item${i === paletteSel ? ' sel' : ''}${c.resource ? ' res' : ''}" data-i="${i}"><span>${esc(c.label)}</span><span class="p-kind">${esc(c.kind)}</span></div>`).join('');
    list.querySelectorAll('.palette-item').forEach((el) => {
        el.addEventListener('click', () => runPalette(parseInt(el.dataset.i, 10)));
        el.addEventListener('mousemove', () => { paletteSel = parseInt(el.dataset.i, 10); highlightPalette(); });
    });
}

function highlightPalette() {
    $('palette-list').querySelectorAll('.palette-item').forEach((el, i) => el.classList.toggle('sel', i === paletteSel));
    const sel = $('palette-list').querySelector('.palette-item.sel');
    if (sel) sel.scrollIntoView({ block: 'nearest' });
}

function runPalette(i) {
    const c = paletteFiltered[i];
    if (!c) return;
    closePalette();
    c.run();
}

$('palette-input').addEventListener('input', () => {
    paletteSel = 0;
    schedulePaletteSearch($('palette-input').value.trim());
    renderPalette();
});
$('palette-backdrop').addEventListener('click', closePalette);

document.addEventListener('keydown', (e) => {
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'k') { e.preventDefault(); openPalette(); return; }
    if ($('palette').hidden) return;
    if (e.key === 'Escape') { e.preventDefault(); closePalette(); }
    else if (e.key === 'ArrowDown') { e.preventDefault(); paletteSel = Math.min(paletteSel + 1, paletteFiltered.length - 1); highlightPalette(); }
    else if (e.key === 'ArrowUp') { e.preventDefault(); paletteSel = Math.max(paletteSel - 1, 0); highlightPalette(); }
    else if (e.key === 'Enter') { e.preventDefault(); runPalette(paletteSel); }
});

// ============ Helm (release detail / history / upgrade / search / install) ============

// ---- External-link helper (opens the system browser via Wails runtime) ----

function extLink(url, text) {
    if (!url) return '';
    return `<span class="ext-link mono" data-url="${esc(url)}" title="Open in browser">${esc(text || url)} ↗</span>`;
}
document.addEventListener('click', (e) => {
    const el = e.target.closest('.ext-link');
    if (el && el.dataset.url) { e.preventDefault(); BrowserOpenURL(el.dataset.url); }
});

// Render a line diff (reusing lineDiff) into an element.
//
// `collapse: n` keeps only n lines of context around each change. Use it when
// diffing against a live object — a Deployment the server has defaulted runs to
// hundreds of lines, and a one-line change should read as one line. Leave it off
// when the whole text is the point (the drawer's edit-vs-loaded diff).
function renderDiffInto(el, aText, bText, { collapse = 0 } = {}) {
    const diff = lineDiff(aText || '', bText || '');
    if (diff.every((d) => d.t === ' ')) { el.innerHTML = '<span class="diff-ctx">No changes.</span>'; return; }
    const shown = collapse > 0 ? collapseDiff(diff, collapse) : diff;
    // Joined with '' — NOT '\n'.
    //
    // Each line is a `display: block` span, so it already breaks. The containers
    // (.yaml-diff, .pv-diff, .diff-view) are `white-space: pre`, which means a
    // literal newline between two blocks is preserved and renders as its own empty
    // line — every diff came out double-spaced, twice as tall, twice as much
    // scrolling. Keep the two facts together: block spans, no separator.
    el.innerHTML = shown.map((d) => {
        if (d.t === '…') return '<span class="diff-ctx">  …</span>';
        const cls = d.t === '+' ? 'diff-add' : d.t === '-' ? 'diff-del' : 'diff-ctx';
        return `<span class="${cls}">${esc(d.t === ' ' ? '  ' : d.t + ' ')}${esc(d.l)}</span>`;
    }).join('');
}

// Replace each run of unchanged lines further than `context` from a change with a
// single ellipsis marker.
function collapseDiff(diff, context) {
    const keep = new Array(diff.length).fill(false);
    diff.forEach((d, i) => {
        if (d.t === ' ') return;
        for (let j = i - context; j <= i + context; j++) {
            if (j >= 0 && j < diff.length) keep[j] = true;
        }
    });
    const out = [];
    let skipping = false;
    diff.forEach((d, i) => {
        if (!keep[i]) {
            if (!skipping) { out.push({ t: '…', l: '' }); skipping = true; }
            return;
        }
        skipping = false;
        out.push(d);
    });
    return out;
}

function openHelmDetailModal(ref) {
    openModal({
        title: `Helm — ${ref.name}`,
        okText: 'Close',
        bodyHtml: `<div class="helm-tabs">
                <button class="helm-tab active" data-htab="resources">Resources</button>
                <button class="helm-tab" data-htab="values">Values</button>
                <button class="helm-tab" data-htab="manifest">Manifest</button>
                <button class="helm-tab" data-htab="notes">Notes</button>
                <button id="helm-run-tests" class="btn btn-secondary btn-sm" style="margin-left:auto">Run tests</button>
            </div>
            <div id="helm-meta" class="helm-meta"></div>
            <div id="helm-resources" class="helm-resources">Loading…</div>
            <pre id="helm-content" class="helm-content" hidden>Loading…</pre>`,
        onOk: () => Promise.resolve(),
    });
    HelmGet(ref.namespace, ref.name)
        .then((d) => {
            $('helm-meta').innerHTML = `<span class="chip">chart: ${esc(d.chart)}</span> <span class="chip">app: ${esc(d.appVersion || '-')}</span> <span class="chip">rev ${d.revision}</span> <span class="chip">${esc(d.status)}</span>`;
            const panes = { values: d.values || '(no user-supplied values)', manifest: d.manifest || '', notes: d.notes || '(no notes)' };
            const show = (tab) => {
                const isRes = tab === 'resources';
                $('helm-resources').hidden = !isRes;
                $('helm-content').hidden = isRes;
                if (!isRes) $('helm-content').textContent = panes[tab];
            };
            show('resources');
            document.querySelectorAll('.helm-tab').forEach((t) => {
                t.addEventListener('click', () => {
                    document.querySelectorAll('.helm-tab').forEach((x) => x.classList.toggle('active', x === t));
                    show(t.dataset.htab);
                });
            });
        })
        .catch((err) => { $('helm-resources').textContent = errMsg(err); });

    // Resources tab: live health of every object the release owns.
    loadHelmReleaseResources(ref);

    $('helm-run-tests').addEventListener('click', () => {
        const btn = $('helm-run-tests');
        btn.disabled = true; btn.textContent = 'Testing…';
        HelmTest(ref.namespace, ref.name)
            .then((out) => showAlert(out, { title: `Test results — ${ref.name}`, icon: '🧪' }))
            .catch((err) => showError(errMsg(err)))
            .finally(() => { btn.disabled = false; btn.textContent = 'Run tests'; });
    });
}

function loadHelmReleaseResources(ref) {
    HelmReleaseResources(ref.namespace, ref.name)
        .then((list) => {
            const box = $('helm-resources');
            if (!list || list.length === 0) { box.innerHTML = '<p class="empty-inline">No resources found in the manifest.</p>'; return; }
            box.innerHTML = '';
            for (const r of list) {
                const div = document.createElement('div');
                div.className = 'helm-res-row';
                const openable = r.kind && r.name;
                div.innerHTML = `<div class="helm-res-main">
                        <span class="helm-res-kind">${esc(r.kind)}</span>
                        <span class="helm-res-name${openable ? ' link' : ''}">${esc(r.name)}</span>
                        <span class="chart-repo">${esc(r.namespace)}</span>
                    </div>
                    <div>${badge(r.status, r.ready)}</div>`;
                if (openable) {
                    const nameEl = div.querySelector('.helm-res-name.link');
                    nameEl.addEventListener('click', () => {
                        closeModal();
                        openDrawer({ kind: r.kind, namespace: r.namespace, name: r.name });
                    });
                }
                box.appendChild(div);
            }
        })
        .catch((err) => { $('helm-resources').innerHTML = `<p class="error">${esc(errMsg(err))}</p>`; });
}

function openHelmHistoryModal(ref) {
    openModal({
        title: `Helm history — ${ref.name}`,
        okText: 'Close',
        bodyHtml: `<div id="helm-history" class="rollout-list"><p class="empty-inline">Loading…</p></div>`,
        onOk: () => Promise.resolve(),
    });
    HelmHistory(ref.namespace, ref.name)
        .then((revs) => {
            const box = $('helm-history');
            if (!revs || revs.length === 0) { box.innerHTML = '<p class="empty-inline">No history.</p>'; return; }
            const currentRev = revs[0].revision;
            box.innerHTML = revs.map((r, i) =>
                `<div class="rollout-row">
                    <div>
                        <span class="rollout-rev">Revision ${r.revision}${i === 0 ? ' <span class="chip">current</span>' : ''}</span>
                        <div class="rollout-images mono">${esc(r.chart)} · ${esc(r.status)}</div>
                        <div class="rollout-age">${esc(r.updated)} · ${esc(r.description)}</div>
                    </div>
                    <div class="rollout-actions">
                        ${i === 0 ? '' : `<button class="btn btn-secondary btn-sm helm-diff-btn" data-rev="${r.revision}">Diff vs current</button>
                        <button class="btn btn-secondary btn-sm helm-rollback-btn" data-rev="${r.revision}">Rollback</button>`}
                    </div>
                </div>`).join('') + '<pre id="helm-hist-diff" class="helm-content diff-view" hidden></pre>';
            box.querySelectorAll('.helm-diff-btn').forEach((btn) => {
                btn.addEventListener('click', () => {
                    const rev = parseInt(btn.dataset.rev, 10);
                    const pre = $('helm-hist-diff');
                    // Toggle: clicking the same revision's Diff again hides the panel.
                    if (!pre.hidden && pre.dataset.rev === String(rev)) {
                        pre.hidden = true;
                        box.querySelectorAll('.helm-diff-btn').forEach((b) => (b.textContent = 'Diff vs current'));
                        return;
                    }
                    pre.dataset.rev = String(rev);
                    pre.hidden = false;
                    pre.textContent = 'Loading diff…';
                    box.querySelectorAll('.helm-diff-btn').forEach((b) =>
                        (b.textContent = b === btn ? '✕ Hide diff' : 'Diff vs current'));
                    Promise.all([
                        HelmGetRevision(ref.namespace, ref.name, rev),
                        HelmGetRevision(ref.namespace, ref.name, currentRev),
                    ])
                        .then(([older, current]) => {
                            renderDiffInto(pre, older.manifest, current.manifest);
                            pre.scrollIntoView({ block: 'nearest' });
                        })
                        .catch((err) => { pre.textContent = errMsg(err); });
                });
            });
            box.querySelectorAll('.helm-rollback-btn').forEach((btn) => {
                btn.addEventListener('click', () => {
                    const rev = parseInt(btn.dataset.rev, 10);
                    showConfirm(`Rollback release “${ref.name}” to revision ${rev}?`, { title: 'Rollback release', icon: '↩', okText: 'Rollback' }).then((ok) => {
                        if (!ok) return;
                        btn.disabled = true;
                        HelmRollback(ref.namespace, ref.name, rev)
                            .then(() => { closeModal(); refreshCurrentView(); })
                            .catch((err) => { showError(errMsg(err)); btn.disabled = false; });
                    });
                });
            });
        })
        .catch((err) => { $('helm-history').innerHTML = `<p class="error">${esc(errMsg(err))}</p>`; });
}

function openHelmUpgradeModal(ref) {
    openModal({
        title: `Upgrade values — ${ref.name}`,
        okText: 'Upgrade',
        bodyHtml: `<p class="modal-hint">Edit the release values, then <strong>Preview</strong> the rendered diff before you Upgrade (reuses the current chart).</p>
            <div id="helm-values" class="yaml-host yaml-host-modal"></div>
            <div class="install-actions">
                <button id="helm-preview-btn" class="btn btn-secondary btn-sm">👁 Preview diff</button>
                <span id="helm-preview-status" class="modal-hint"></span>
            </div>
            <pre id="helm-upg-diff" class="helm-content diff-view" hidden></pre>`,
        onOpen: () => mountModalEditor('helm-values', { value: '# loading…' }),
        onOk: () => {
            const vals = modalYaml('helm-values');
            return HelmUpgradeValues(ref.namespace, ref.name, vals).then(() => refreshCurrentView());
        },
    });
    HelmGet(ref.namespace, ref.name)
        .then((d) => setModalYaml('helm-values', d.values || ''))
        .catch((err) => setModalYaml('helm-values', `# could not load values: ${errMsg(err)}`));

    $('helm-preview-btn').addEventListener('click', () => {
        const vals = modalYaml('helm-values');
        const pre = $('helm-upg-diff');
        const btn = $('helm-preview-btn');
        const status = $('helm-preview-status');
        if (!pre.hidden) { pre.hidden = true; status.textContent = ''; btn.textContent = '👁 Preview diff'; return; }
        pre.hidden = false; pre.textContent = 'Rendering (dry-run)…'; status.textContent = ''; btn.textContent = '✕ Hide preview';
        HelmUpgradePreview(ref.namespace, ref.name, vals)
            .then((diff) => {
                renderDiffInto(pre, diff.current, diff.proposed);
                status.textContent = 'Diff = current manifest → what Upgrade would apply.';
            })
            .catch((err) => { pre.textContent = errMsg(err); });
    });
}

function uninstallHelm(ref) {
    showConfirm(`Uninstall Helm release “${ref.name}” in ${ref.namespace}?\nThis removes all its resources.`, { title: 'Uninstall release', icon: '🗑', okText: 'Uninstall', danger: true }).then((ok) => {
        if (!ok) return;
        HelmUninstall(ref.namespace, ref.name)
            .then(() => { loadSidebarCounts(); refreshCurrentView(); })
            .catch((err) => showError(errMsg(err)));
    });
}

// ---- Search charts (Artifact Hub) + install ----

$('btn-helm-search').addEventListener('click', openChartSearchModal);

function openChartSearchModal() {
    openModal({
        title: 'Search & install charts',
        okText: 'Close',
        bodyHtml: `<div class="chart-search-row">
                <input id="chart-query" class="pf-input no-enter-submit" style="flex:1" type="text" placeholder="Search Artifact Hub (e.g. nginx, redis, prometheus)…" autocomplete="off">
                <button id="chart-search-go" class="btn btn-primary btn-sm">Search</button>
            </div>
            <p class="modal-hint">Requires internet access to artifacthub.io.</p>
            <div id="chart-results" class="chart-results"></div>`,
        onOk: () => Promise.resolve(),
    });
    const run = () => {
        const q = $('chart-query').value.trim();
        if (!q) return;
        $('chart-results').innerHTML = '<p class="empty-inline">Searching…</p>';
        SearchCharts(q)
            .then((results) => renderChartResults(results, $('chart-results')))
            .catch((err) => { $('chart-results').innerHTML = `<p class="error">${esc(errMsg(err))}</p>`; });
    };
    $('chart-search-go').addEventListener('click', run);
    $('chart-query').addEventListener('keydown', (e) => { if (e.key === 'Enter') { e.preventDefault(); run(); } });
    $('chart-query').focus();
}

function renderChartResults(results, box) {
    if (!results || results.length === 0) { box.innerHTML = '<p class="empty-inline">No charts found.</p>'; return; }
    box.innerHTML = '';
    for (const c of results) {
        const div = document.createElement('div');
        div.className = 'chart-item';
        const oci = c.repoURL.startsWith('oci://');
        div.innerHTML = `<div class="chart-main">
                <div class="chart-name">${esc(c.name)} <span class="chart-repo">${esc(c.repo)}</span> <span class="chart-ver mono">${esc(c.version)}</span>${oci ? ' <span class="chip">OCI</span>' : ''}</div>
                <div class="chart-desc">${esc(c.description || '')}</div>
                <div class="chart-url">${extLink(c.repoURL)}</div>
            </div>`;
        const btn = document.createElement('button');
        btn.className = 'btn btn-primary btn-sm';
        btn.textContent = 'Install →';
        btn.addEventListener('click', () => openChartInstallModal(c));
        div.appendChild(btn);
        box.appendChild(div);
    }
}

function openChartInstallModal(chart) {
    const ns = currentNamespace || 'default';
    const chartName = chart.normName || chart.name;
    openModal({
        title: `Install ${chart.name}`,
        okText: 'Install',
        bodyHtml: `<div class="install-form">
                <label>Release name<input type="text" id="inst-name" class="pf-input" value="${esc(chartName)}"></label>
                <label>Namespace<input type="text" id="inst-ns" class="pf-input" value="${esc(ns)}"></label>
                <label>Version<select id="inst-ver" class="pf-input"><option value="${esc(chart.version)}">${esc(chart.version)}</option></select></label>
            </div>
            <div id="inst-links" class="install-links"><span class="modal-hint">Repo: </span>${extLink(chart.repoURL)}</div>
            <div class="install-tabs">
                <button class="install-tab active" data-itab="values">Values</button>
                <button class="install-tab" data-itab="readme">README</button>
            </div>
            <div id="inst-pane-values">
                <div class="install-actions">
                    <button id="inst-load-defaults" class="btn btn-secondary btn-sm">↓ Load chart defaults</button>
                    <button id="inst-preview" class="btn btn-secondary btn-sm">👁 Preview (dry-run)</button>
                    <span id="inst-status" class="modal-hint"></span>
                </div>
                <div id="inst-values" class="yaml-host yaml-host-short"></div>
                <pre id="inst-diff" class="helm-content diff-view" hidden></pre>
            </div>
            <pre id="inst-readme" class="helm-content" hidden>Loading README…</pre>`,
        onOpen: () => mountModalEditor('inst-values', {
            placeholder: '# leave empty for chart defaults, or click “Load chart defaults”',
        }),
        onOk: () => {
            const name = $('inst-name').value.trim();
            const nsv = $('inst-ns').value.trim() || 'default';
            const ver = $('inst-ver').value.trim();
            const vals = modalYaml('inst-values');
            if (!name) return Promise.reject('Enter a release name.');
            return HelmInstall(nsv, name, chart.repoURL, chartName, ver, vals)
                .then(() => { selectView('helm'); loadSidebarCounts(); });
        },
    });

    // Tabs: Values / README.
    document.querySelectorAll('.install-tab').forEach((t) => {
        t.addEventListener('click', () => {
            document.querySelectorAll('.install-tab').forEach((x) => x.classList.toggle('active', x === t));
            const readme = t.dataset.itab === 'readme';
            $('inst-pane-values').hidden = readme;
            $('inst-readme').hidden = !readme;
        });
    });

    // Load chart defaults into the values editor (authoritative, from the repo).
    $('inst-load-defaults').addEventListener('click', () => {
        const btn = $('inst-load-defaults');
        btn.disabled = true; btn.textContent = 'Loading…';
        ChartDefaultValues(chart.repoURL, chartName, $('inst-ver').value.trim())
            .then((vals) => { setModalYaml('inst-values', vals); $('inst-status').textContent = 'Loaded chart defaults.'; })
            .catch((err) => { $('inst-status').textContent = errMsg(err); })
            .finally(() => { btn.disabled = false; btn.textContent = '↓ Load chart defaults'; });
    });

    // Dry-run preview of the manifest this install would create (toggle on/off).
    $('inst-preview').addEventListener('click', () => {
        const pre = $('inst-diff');
        const btn = $('inst-preview');
        if (!pre.hidden) { pre.hidden = true; btn.textContent = '👁 Preview (dry-run)'; return; }
        pre.hidden = false; pre.textContent = 'Rendering (dry-run)…'; btn.textContent = '✕ Hide preview';
        HelmInstallPreview($('inst-ns').value.trim() || 'default', $('inst-name').value.trim() || 'preview',
            chart.repoURL, chartName, $('inst-ver').value.trim(), modalYaml('inst-values'))
            .then((diff) => { renderDiffInto(pre, diff.current, diff.proposed); })
            .catch((err) => { pre.textContent = errMsg(err); });
    });

    // Enrich from Artifact Hub (best-effort): version list, README, home/links.
    ChartDetails(chart.repo, chartName)
        .then((d) => {
            if (d.versions && d.versions.length > 0) {
                const sel = $('inst-ver');
                sel.innerHTML = d.versions.map((v) => `<option value="${esc(v)}"${v === chart.version ? ' selected' : ''}>${esc(v)}</option>`).join('');
            }
            const links = [];
            if (d.homeURL) links.push(extLink(d.homeURL, 'home'));
            for (const l of d.links || []) links.push(extLink(l.url, l.name || 'link'));
            const linkBox = $('inst-links');
            linkBox.innerHTML = `<span class="modal-hint">Repo: </span>${extLink(chart.repoURL)}` +
                (links.length ? ` &nbsp;·&nbsp; <span class="modal-hint">Links: </span>${links.join(' ')}` : '') +
                (d.maintainers && d.maintainers.length ? ` &nbsp;·&nbsp; <span class="modal-hint">By: </span>${esc(d.maintainers.join(', '))}` : '');
            $('inst-readme').textContent = d.readme || '(no README published)';
        })
        .catch(() => { $('inst-readme').textContent = '(chart details unavailable — not on Artifact Hub or offline)'; });
}

// ============ Helm repositories ============

function loadHelmRepos(scope) {
    return ListHelmRepos()
        .then((repos) => {
            if (!isCurrentViewRequest(scope)) return;
            const body = $('helmrepos-body');
            body.innerHTML = '';
            $('helmrepos-empty').hidden = (repos?.length ?? 0) > 0;
            for (const r of repos ?? []) {
                const tr = document.createElement('tr');
                tr.innerHTML = `<td>${esc(r.name)}</td>
                    <td>${extLink(r.url)}</td>
                    <td class="col-actions">
                        <button class="btn btn-secondary btn-sm repo-browse" data-name="${esc(r.name)}">Browse</button>
                        <button class="btn btn-secondary btn-sm repo-remove" data-name="${esc(r.name)}">Remove</button>
                    </td>`;
                body.appendChild(tr);
            }
            body.querySelectorAll('.repo-browse').forEach((btn) =>
                btn.addEventListener('click', () => openRepoBrowseModal(btn.dataset.name)));
            body.querySelectorAll('.repo-remove').forEach((btn) =>
                btn.addEventListener('click', () => {
                    showConfirm(`Remove repo “${btn.dataset.name}”?`, { title: 'Remove repository', icon: '🗑', okText: 'Remove', danger: true }).then((ok) => {
                        if (!ok) return;
                        RemoveHelmRepo(btn.dataset.name).then(loadHelmRepos).catch((err) => showError(errMsg(err)));
                    });
                }));
        })
        .catch((err) => viewError(scope, err));
}

function openRepoAddModal() {
    openModal({
        title: 'Add Helm repository',
        okText: 'Add',
        bodyHtml: `<div class="install-form">
                <label>Name<input type="text" id="repo-name" class="pf-input" placeholder="bitnami"></label>
                <label>URL<input type="text" id="repo-url" class="pf-input" placeholder="https://charts.bitnami.com/bitnami"></label>
                <label>Username <span class="modal-hint">(private repos only)</span><input type="text" id="repo-user" class="pf-input" autocomplete="off"></label>
                <label>Password<input type="password" id="repo-pass" class="pf-input" autocomplete="off"></label>
            </div>
            <p class="modal-hint">The index is downloaded to validate the URL — this needs internet access.</p>`,
        onOk: () => {
            const name = $('repo-name').value.trim();
            const url = $('repo-url').value.trim();
            if (!name || !url) return Promise.reject('Name and URL are required.');
            return AddHelmRepo(name, url, $('repo-user').value, $('repo-pass').value).then(loadHelmRepos);
        },
    });
}

function openRepoBrowseModal(name) {
    openModal({
        title: `Browse ${name}`,
        okText: 'Close',
        bodyHtml: `<div class="chart-search-row">
                <input id="repo-filter" class="pf-input no-enter-submit" style="flex:1" type="text" placeholder="Filter charts…" autocomplete="off">
            </div>
            <div id="repo-browse-results" class="chart-results"><p class="empty-inline">Loading…</p></div>`,
        onOk: () => Promise.resolve(),
    });
    let all = [];
    const render = () => {
        const q = $('repo-filter').value.trim().toLowerCase();
        const filtered = q ? all.filter((c) => c.name.toLowerCase().includes(q)) : all;
        renderChartResults(filtered, $('repo-browse-results'));
    };
    BrowseHelmRepo(name)
        .then((charts) => { all = charts || []; render(); })
        .catch((err) => { $('repo-browse-results').innerHTML = `<p class="error">${esc(errMsg(err))}</p>`; });
    $('repo-filter').addEventListener('input', render);
}

$('btn-repo-add').addEventListener('click', openRepoAddModal);
$('btn-repo-update').addEventListener('click', () => {
    const btn = $('btn-repo-update');
    btn.disabled = true; btn.textContent = 'Updating…';
    UpdateHelmRepos()
        .then(() => { btn.textContent = '↻ Updated ✓'; })
        .catch((err) => showError(errMsg(err)))
        .finally(() => { setTimeout(() => { btn.disabled = false; btn.textContent = '↻ Update all'; }, 1500); });
});

// ============ Live mode (auto-refresh) ============

let liveTimer = null;

$('btn-live').addEventListener('click', () => {
    if (liveTimer) {
        clearInterval(liveTimer);
        liveTimer = null;
        $('btn-live').classList.remove('live-on');
    } else {
        liveTimer = setInterval(() => {
            if ($('dashboard').hidden) return;
            // Don't disrupt an active selection / open drawer / modal / palette.
            if (selectedRows.size > 0) return;
            if (!$('drawer').hidden || !$('modal').hidden || !$('palette').hidden) return;
            refreshCurrentView();
            loadSidebarCounts();
        }, 5000);
        $('btn-live').classList.add('live-on');
    }
});

// ============ Theme (dark mode) ============
// The moon/sun icons are swapped by CSS off the root data-theme attribute.

function applyTheme(theme) {
    document.documentElement.setAttribute('data-theme', theme);
}
$('btn-theme').addEventListener('click', () => {
    const next = document.documentElement.getAttribute('data-theme') === 'dark' ? 'light' : 'dark';
    try { localStorage.setItem('kubby-theme', next); } catch { /* ignore */ }
    applyTheme(next);
});
(function initTheme() {
    let saved = 'light';
    try { saved = localStorage.getItem('kubby-theme') || 'light'; } catch { /* ignore */ }
    applyTheme(saved);
})();

// ============ FR-7: AI assistant (drawer tab) ============
//
// The assistant is a conversation about the resource that is currently open, not
// a one-shot popup. Everything it knows is collected live when you ask, shown as
// chips in the header, and inspectable in full before you send anything.

let aiThread = [];    // [{ role, content }] — the visible conversation
let aiThreadKey = ''; // kind/ns/name the thread belongs to
let aiBusy = false;
let aiContext = null; // last-fetched AIContext for the chips / "what gets sent"
let aiStatus = null;  // cached GetAIStatus() — keeps rendering synchronous

// Starter questions, tuned per kind — a blank prompt box is the main reason
// people never use an assistant like this.
function aiSuggestions(ref) {
    const common = [
        'Explain this resource in plain language',
        'Is anything misconfigured here?',
    ];
    switch (ref.kind) {
        case 'Pod':
            return ['What is wrong with this Pod?', 'What do the recent logs mean?',
                'Why has it restarted?', ...common];
        case 'Deployment':
        case 'StatefulSet':
        case 'DaemonSet':
            return [`Why are not all replicas ready?`, 'Is this rollout healthy?',
                'Are the resource requests and limits sensible?', ...common];
        case 'Service':
            return ['Why is this Service not reaching any Pod?', 'Is the selector correct?', ...common];
        case 'Ingress':
            return ['Why does this Ingress return 404?', 'Is the routing set up correctly?', ...common];
        case 'PersistentVolumeClaim':
            return ['Why is this PVC still Pending?', ...common];
        case 'Node':
            return ['Is this Node healthy?', 'Why are Pods not scheduling here?', ...common];
        case 'Job':
        case 'CronJob':
            return ['Why did this fail?', 'When did it last run successfully?', ...common];
        default:
            return [...common, 'What should I check next?'];
    }
}

function aiKeyFor(ref) { return `${ref.kind}/${ref.namespace}/${ref.name}`; }

// Called when the drawer opens on a new resource: the old thread belongs to a
// different resource, so it is dropped rather than silently carried over.
function resetAIPanel(ref) {
    const key = aiKeyFor(ref);
    if (key === aiThreadKey) return;
    aiThreadKey = key;
    aiThread = [];
    aiContext = null;
    aiBusy = false;
    $('ai-input').value = '';
    $('ai-input').style.height = '';
    $('btn-ai-send').disabled = false;
    $('ai-context-chips').innerHTML = '';
    renderAIThread();
}

// Lazily loaded when the tab is first opened — collecting logs + YAML is a few
// API calls, not something to do for every drawer open.
function prepareAIPanel() {
    refreshAIProviderBadge().then(renderAIThread);
    renderAIThread();
    if (!aiContext && drawerRef) {
        const ref = drawerRef;
        $('ai-context-chips').innerHTML = '<span class="ai-chip ai-chip-loading">Collecting evidence…</span>';
        AIResourceContext(ref.kind, ref.namespace, ref.name)
            .then((c) => {
                if (aiThreadKey !== aiKeyFor(ref)) return;
                aiContext = c;
                renderAIContextChips();
            })
            .catch(() => { $('ai-context-chips').innerHTML = ''; });
    }
}

function renderAIContextChips() {
    const c = aiContext;
    if (!c) { $('ai-context-chips').innerHTML = ''; return; }
    const chips = [];
    if (c.events) chips.push(`${c.events} event${c.events === 1 ? '' : 's'}`);
    if (c.logContainers) chips.push(`${c.logLines} log lines from ${c.logContainers} container${c.logContainers === 1 ? '' : 's'}`);
    if (c.hasYAML) chips.push('manifest YAML');
    if (chips.length === 0) chips.push('metadata only');
    $('ai-context-chips').innerHTML =
        chips.map((t) => `<span class="ai-chip">${esc(t)}</span>`).join('') +
        `<button type="button" class="ai-chip ai-chip-link" id="ai-view-context">See exactly what is sent</button>`;
    $('ai-view-context').addEventListener('click', showAIContext);
}

function showAIContext() {
    openModal({
        title: 'What Kubby sends to the AI',
        okText: 'Close',
        bodyHtml: `<p class="modal-hint" style="margin-top:0">
                This text is sent to <strong>${esc($('ai-provider-badge').textContent || 'your provider')}</strong>
                with every question in this thread, together with the questions themselves. Nothing else leaves this machine.
            </p>
            <pre class="ai-context-dump">${esc(aiContext?.text ?? '')}</pre>`,
        onOk: () => Promise.resolve(),
    });
}

function refreshAIProviderBadge() {
    return GetAIStatus().then((s) => {
        aiStatus = s;
        const badge = $('ai-provider-badge');
        if (!s.configured) { badge.textContent = 'Not set up'; badge.className = 'ai-provider ai-provider-off'; return s; }
        badge.textContent = `${s.label} · ${s.model}`;
        badge.className = 'ai-provider' + (s.local ? ' ai-provider-local' : '');
        badge.title = s.local
            ? 'Runs on your machine — nothing leaves it'
            : 'Resource evidence is sent to this provider';
        return s;
    }).catch(() => null);
}

function renderAIThread() {
    const box = $('ai-thread');
    const ref = drawerRef;
    if (!ref) { box.innerHTML = ''; return; }
    // Status not fetched yet — say nothing rather than flash the setup card at
    // someone who already configured a provider.
    if (aiStatus === null) { box.innerHTML = '<p class="empty-inline">Loading…</p>'; return; }

    if (!aiStatus.configured) { box.innerHTML = aiSetupCardHtml(); wireAISetupCard(); return; }

    if (aiThread.length === 0) {
        box.innerHTML = `<div class="ai-welcome">
            <p class="ai-welcome-lead">Ask anything about this ${esc(ref.kind)}.
               Kubby attaches its events, logs and manifest to your question.</p>
            <div class="ai-suggestions">
                ${aiSuggestions(ref).map((q) =>
                    `<button type="button" class="ai-suggestion">${esc(q)}</button>`).join('')}
            </div>
        </div>`;
        box.querySelectorAll('.ai-suggestion').forEach((b) => {
            b.addEventListener('click', () => sendAIQuestion(b.textContent.trim()));
        });
        $('btn-ai-reset').hidden = true;
        return;
    }

    box.innerHTML = aiThread.map((m, i) => m.role === 'user'
        ? `<div class="ai-msg ai-msg-user"><div class="ai-bubble">${esc(m.content)}</div></div>`
        : `<div class="ai-msg ai-msg-bot">
               <div class="ai-answer">${renderMarkdownish(m.content)}</div>
               <button type="button" class="ai-copy" data-i="${i}">Copy</button>
           </div>`).join('')
        + (aiBusy ? `<div class="ai-msg ai-msg-bot"><div class="ai-thinking">
                <span class="ai-spin"></span> Reading events, logs and YAML…</div></div>` : '');

    box.querySelectorAll('.ai-copy').forEach((b) => {
        b.addEventListener('click', () => {
            navigator.clipboard?.writeText(aiThread[+b.dataset.i].content);
            b.textContent = 'Copied';
            setTimeout(() => { b.textContent = 'Copy'; }, 1200);
        });
    });
    $('btn-ai-reset').hidden = false;
    box.scrollTop = box.scrollHeight;
}

function aiSetupCardHtml() {
    return `<div class="ai-setup">
        <h4>Set up the AI assistant</h4>
        <p>Kubby can read a resource's events, logs and manifest and explain — in plain language —
           what is wrong and how to fix it. Pick where that runs; it takes about a minute.</p>
        <ul class="ai-setup-list">
            <li><strong>Ollama</strong> — runs on this machine. No API key, nothing leaves your laptop.</li>
            <li><strong>Anthropic (Claude)</strong> or an <strong>OpenAI-compatible</strong> endpoint — needs an API key.</li>
        </ul>
        <button type="button" class="btn btn-primary btn-sm" id="ai-setup-open">Choose a provider</button>
        <p class="ai-setup-note">Your key is stored only on this machine, in <code>%AppData%/kubby/ai.json</code>.</p>
    </div>`;
}

function wireAISetupCard() {
    $('ai-setup-open')?.addEventListener('click', openSettingsModal);
    $('btn-ai-reset').hidden = true;
}

function sendAIQuestion(text) {
    const q = String(text || '').trim();
    if (!q || aiBusy || !drawerRef) return;
    const ref = drawerRef;
    const key = aiKeyFor(ref);

    aiThread.push({ role: 'user', content: q });
    aiBusy = true;
    $('ai-input').value = '';
    autoGrowAIInput();
    $('btn-ai-send').disabled = true;
    renderAIThread();

    AskAboutResource(ref.kind, ref.namespace, ref.name, aiThread)
        .then((answer) => {
            if (aiThreadKey !== key) return; // user moved to another resource
            aiThread.push({ role: 'assistant', content: answer });
        })
        .catch((err) => {
            if (aiThreadKey !== key) return;
            aiThread.push({ role: 'assistant', content: `⚠️ **${errMsg(err)}**\n\nOpen the ⚙ button above to check the provider settings.` });
        })
        .finally(() => {
            // The button is re-enabled even if the user moved on, otherwise the
            // composer would stay dead on the resource they switched to.
            $('btn-ai-send').disabled = false;
            if (aiThreadKey !== key) return;
            aiBusy = false;
            renderAIThread();
        });
}

$('ai-composer').addEventListener('submit', (e) => {
    e.preventDefault();
    sendAIQuestion($('ai-input').value);
});

// Enter sends, Shift+Enter makes a new line. The textarea carries
// `.no-enter-submit` so the global handler leaves it alone.
$('ai-input').addEventListener('keydown', (e) => {
    if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault();
        sendAIQuestion($('ai-input').value);
    }
});
$('ai-input').addEventListener('input', autoGrowAIInput);

function autoGrowAIInput() {
    const el = $('ai-input');
    el.style.height = 'auto';
    el.style.height = `${Math.min(el.scrollHeight, 140)}px`;
}

$('btn-ai-reset').addEventListener('click', () => {
    aiThread = [];
    renderAIThread();
    $('ai-input').focus();
});

$('btn-ai-settings').addEventListener('click', openSettingsModal);

// Minimal, safe markdown → HTML (escape first, then a few inline rules).
function renderMarkdownish(text) {
    const lines = String(text || '').split('\n');
    let html = '';
    let inList = false;
    const inline = (s) => esc(s)
        .replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
        .replace(/`([^`]+?)`/g, '<code>$1</code>');
    for (let raw of lines) {
        const line = raw.trimEnd();
        const bullet = line.match(/^\s*[-*]\s+(.*)$/);
        const heading = line.match(/^#{1,4}\s+(.*)$/);
        if (bullet) {
            if (!inList) { html += '<ul>'; inList = true; }
            html += `<li>${inline(bullet[1])}</li>`;
            continue;
        }
        if (inList) { html += '</ul>'; inList = false; }
        if (heading) html += `<h4>${inline(heading[1])}</h4>`;
        else if (line === '') html += '';
        else html += `<p>${inline(line)}</p>`;
    }
    if (inList) html += '</ul>';
    return html || '<p class="empty-inline">(không có nội dung)</p>';
}

// ---- Settings (AI provider config) ----
$('btn-settings').addEventListener('click', openSettingsModal);

function openSettingsModal() {
    openModal({
        title: 'Settings',
        okText: 'Save',
        bodyHtml: `<div class="about-row" style="margin-bottom:0.4rem"><span class="about-label">AI assistant</span></div>
            <p class="modal-hint" style="margin-top:0">
                When you ask a question, Kubby sends that resource's <strong>events, recent logs and manifest</strong>
                to the provider you pick here — and nothing else. Your key is stored only on this machine, in
                <code>%AppData%/kubby/ai.json</code>.
            </p>
            <div class="settings-ai-form" style="margin-top:0.7rem">
                <label>Provider
                    <select id="ai-provider" class="pf-input">
                        <option value="">— Choose one —</option>
                        <option value="ollama">Ollama — local, no key, nothing leaves this machine</option>
                        <option value="anthropic">Anthropic (Claude)</option>
                        <option value="openai">OpenAI-compatible (OpenAI, Azure, internal gateway)</option>
                    </select>
                </label>
                <label>Model <input type="text" id="ai-model" class="pf-input no-enter-submit" placeholder="leave blank for the default"></label>
                <label id="ai-endpoint-wrap">Endpoint <input type="text" id="ai-endpoint" class="pf-input no-enter-submit" placeholder="leave blank for the default"></label>
                <label id="ai-key-wrap">API key <input type="password" id="ai-key" class="pf-input no-enter-submit" autocomplete="off" placeholder="stored locally, never shown again"></label>
                <label class="settings-field-full">Answer language
                    <select id="ai-language" class="pf-input">
                        <option value="auto">Match the question</option>
                        <option value="en">Always English</option>
                        <option value="vi">Luôn tiếng Việt</option>
                    </select>
                </label>
            </div>
            <p class="modal-hint" id="ai-hint"></p>
            <div class="about-box">
                <div class="about-row">
                    <span class="about-label">About</span>
                    <span class="about-version mono" id="about-version">…</span>
                </div>
                <p class="modal-hint" style="margin:0.35rem 0 0.5rem">
                    Reporting a problem? <strong>Copy diagnostics</strong> gathers the version, this machine's
                    platform, the connected cluster's Kubernetes version and capabilities, and the last error
                    shown — no API key, no kubeconfig content, no resource data. Review it before sharing.
                </p>
                <button type="button" class="btn btn-secondary btn-sm" id="btn-copy-diagnostics">Copy diagnostics</button>
                <span class="about-copied" id="diag-copied" hidden>Copied</span>
            </div>`,
        onOk: () => {
            const provider = $('ai-provider').value;
            if (!provider) return Promise.reject('Pick a provider first.');
            if (provider !== 'ollama' && !$('ai-key').value.trim()) {
                return Promise.reject('This provider needs an API key.');
            }
            return SaveAIConfig(
                provider,
                $('ai-endpoint').value.trim(),
                $('ai-key').value,
                $('ai-model').value.trim(),
                $('ai-language').value,
            ).then(() => refreshAIProviderBadge()).then(renderAIThread);
        },
    });

    const hints = {
        anthropic: 'Default model claude-haiku-4-5-20251001. Leave the endpoint blank. A paid API key is required.',
        openai: 'Default model gpt-4o-mini. Endpoint is the base URL, e.g. https://api.openai.com or an internal gateway.',
        ollama: 'Install Ollama and pull a model (e.g. `ollama pull llama3.1`). Default endpoint http://localhost:11434. No key, no data leaves this machine.',
    };
    const applyProviderUI = () => {
        const p = $('ai-provider').value;
        $('ai-key-wrap').style.display = p === 'ollama' ? 'none' : '';
        $('ai-hint').textContent = hints[p] || '';
    };
    $('ai-provider').addEventListener('change', applyProviderUI);

    GetAIConfig().then((cfg) => {
        $('ai-provider').value = cfg.provider || '';
        $('ai-endpoint').value = cfg.endpoint || '';
        $('ai-key').value = cfg.apiKey || '';
        $('ai-model').value = cfg.model || '';
        $('ai-language').value = cfg.language || 'auto';
        applyProviderUI();
    }).catch(() => applyProviderUI());

    AppVersion().then((v) => { $('about-version').textContent = v; })
        .catch(() => { $('about-version').textContent = 'unknown'; });

    $('btn-copy-diagnostics').addEventListener('click', () => {
        const btn = $('btn-copy-diagnostics');
        btn.disabled = true;
        // Collected in Go, and copied through the Wails runtime rather than
        // navigator.clipboard — the WebView does not always grant the page
        // clipboard permission, and a silent failure here is worse than useless.
        Diagnostics(lastErrorSeen)
            .then((report) => CopyToClipboard(report))
            .then(() => {
                const flag = $('diag-copied');
                flag.hidden = false;
                setTimeout(() => { flag.hidden = true; }, 2500);
            })
            .catch((err) => showError(errMsg(err), 'Could not copy the diagnostics'))
            .finally(() => { btn.disabled = false; });
    });
}

// ============ Recent connections (Welcome) ============

function populateRecent() {
    RecentConnections()
        .then((list) => {
            const box = $('recent-box');
            const listEl = $('recent-list');
            if (!list || list.length === 0) { box.hidden = true; return; }
            box.hidden = false;
            listEl.innerHTML = '';
            for (const r of list) {
                const item = document.createElement('div');
                item.className = 'recent-item';
                item.innerHTML = `<div class="recent-main"><span class="recent-name">${esc(r.name || r.context)}</span><span class="recent-path mono">${esc(r.path)}</span></div>`;
                const forget = document.createElement('button');
                forget.className = 'recent-forget';
                forget.textContent = '✕';
                forget.title = 'Forget';
                forget.addEventListener('click', (e) => {
                    e.stopPropagation();
                    ForgetConnection(r.path, r.context).then(populateRecent);
                });
                item.appendChild(forget);
                item.addEventListener('click', () => connectRecent(r));
                listEl.appendChild(item);
            }
        })
        .catch(() => { $('recent-box').hidden = true; });
}

function connectRecent(r) {
    clearWelcomeError();
    source = { mode: 'path', path: r.path, content: '' };
    $('connecting-overlay').hidden = false;
    ConnectWithPath(r.path, r.context)
        .then(() => enterDashboard(r.context))
        .catch(showWelcomeError)
        .finally(() => { $('connecting-overlay').hidden = true; });
}

populateRecent();

// ============ helpers ============

function badge(text, isOk) {
    return `<span class="status-badge ${isOk ? 'status-ok' : 'status-error'}">${esc(text)}</span>`;
}

function esc(s) {
    return String(s ?? '').replace(/[&<>"']/g, (c) => ({
        '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;',
    }[c]));
}

function errMsg(err) { return typeof err === 'string' ? err : (err?.message ?? String(err)); }

// The app has no central error sink — every failure is surfaced where it happened
// and then forgotten when the user dismisses it. Remembering the last one is what
// lets "Copy diagnostics" report the error the user actually saw, which is the
// difference between a reproducible bug report and "nó lỗi".
let lastErrorSeen = '';
function recordError(err) {
    const msg = errMsg(err);
    lastErrorSeen = `${new Date().toLocaleTimeString()} — ${msg}`;
    return msg;
}

function showDashError(err) {
    const el = $('dash-error');
    el.textContent = recordError(err);
    el.hidden = false;
}
function clearDashError() { $('dash-error').hidden = true; }
