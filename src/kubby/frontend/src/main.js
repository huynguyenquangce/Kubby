import './style.css';
import './app.css';
import './option-b.css';
import './responsive.css';
import '@fontsource-variable/inter/wght.css';
import '@xterm/xterm/css/xterm.css';

import { FitAddon } from '@xterm/addon-fit';
import { Terminal } from '@xterm/xterm';

import { createYamlEditor, parseApplyFailures } from './editor.js';
import {
    canApplyForwardHydration,
    forwardsToStopOnDrawerClose,
    isCurrentForwardEvent,
    removeForward,
    shouldCancelPendingForward,
    shouldRetainStartedForward,
    upsertForward,
} from './port-forward-state.js';
import { createRequestScopes } from './request-scope.js';
import { createSerialWriter, validTerminalSize } from './terminal-io.js';
import { createFrameScheduler, LineRingBuffer } from './log-buffer.js';
import { lineDiff } from './line-diff.js';
import { confirmedAction, summarizeLineChanges } from './confirmed-action.js';

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
    ClusterStructure,
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
    OverviewSnapshot,
    PodMetricsList,
    PodContainers,
    PodLogs,
    StartLogStream,
    StopLogStream,
    SaveTextToFile,
    StartPortForward,
    CancelPortForwardStart,
    StopPortForward,
    ListPortForwards,
    StartExec,
    ExecWrite,
    ExecResize,
    StopExec,
} from '../wailsjs/go/main/App';
import { EventsOn, BrowserOpenURL } from '../wailsjs/runtime/runtime';

const PAGE_TITLES = {
    overview: 'Overview',
    structure: 'Cluster structure',
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
    structure: 'Debug how entry points, services, workloads, and pods connect across the cluster.',
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
    'structure',
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

function connectionOwnershipChanged() {
    if (!$('modal').hidden) closeModal();
    requestScopes.connectionChanged();
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
    connectionOwnershipChanged();
    clearRenderedView();
    $('welcome').hidden = true;
    $('dashboard').hidden = false;
    hydratePortForwards();
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
            opt.value = c.id;
            opt.textContent = c.name;
            if (c.active) opt.selected = true;
            sel.appendChild(opt);
        }
    });
}

$('cluster-select').addEventListener('change', (e) => {
    const connectionID = e.target.value;
    connectionOwnershipChanged();
    clearRenderedView();
    closeDrawer();
    stopKnownPortForwards();
    clearAccessCache(); // a different cluster grants different things
    SwitchCluster(connectionID)
        .then(() => { currentNamespace = ''; hydratePortForwards(); return loadNamespaceOptions(); })
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
    connectionOwnershipChanged();
    closeDrawer();
    $('dashboard').hidden = true;
    $('welcome').hidden = false;
    clearWelcomeError();
    populateRecent();
});

$('btn-disconnect').addEventListener('click', () => {
    const active = $('cluster-select').value;
    connectionOwnershipChanged();
    clearRenderedView();
    closeDrawer();
    stopKnownPortForwards();
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

// Below the desktop-shell breakpoint the existing sidebar becomes an off-canvas
// navigation drawer. Reusing it keeps dynamic CRD sections, counts, namespace and
// cluster controls in one DOM tree instead of maintaining a second mobile menu.
const mobileNavMedia = window.matchMedia('(max-width: 820px)');

function setMobileNavOpen(open) {
    const wasOpen = $('dashboard').classList.contains('mobile-nav-open');
    const shouldOpen = Boolean(open && mobileNavMedia.matches && !$('dashboard').hidden);
    $('dashboard').classList.toggle('mobile-nav-open', shouldOpen);
    $('btn-mobile-nav').setAttribute('aria-expanded', String(shouldOpen));
    $('btn-mobile-nav').setAttribute('aria-label', shouldOpen ? 'Close navigation' : 'Open navigation');
    $('mobile-nav-backdrop').hidden = !shouldOpen;
    document.querySelector('.main').inert = shouldOpen;
    if (shouldOpen) {
        requestAnimationFrame(() => $('sidebar').querySelector('.nav-item.active, select, button')?.focus());
    } else if (wasOpen && mobileNavMedia.matches) {
        $('btn-mobile-nav').focus();
    }
}

$('btn-mobile-nav').addEventListener('click', () => {
    setMobileNavOpen(!$('dashboard').classList.contains('mobile-nav-open'));
});
$('mobile-nav-backdrop').addEventListener('click', () => setMobileNavOpen(false));
mobileNavMedia.addEventListener('change', () => setMobileNavOpen(false));
document.addEventListener('keydown', (event) => {
    if (event.key === 'Escape' && $('dashboard').classList.contains('mobile-nav-open')) {
        event.preventDefault();
        event.stopImmediatePropagation();
        setMobileNavOpen(false);
    }
});

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
let navCountsRefreshedAt = 0;

function loadSidebarCounts({ includeCluster = true } = {}) {
    const reqId = ++navCountsReqId;
    const connection = requestScopes.connectionToken();
    const namespace = currentNamespace;
    return SidebarCounts(namespace, includeCluster)
        .then((counts) => {
            // Drop a slow reply that a newer request has already superseded —
            // otherwise flicking through namespaces can leave older numbers on top.
            if (reqId !== navCountsReqId || !requestScopes.isCurrentConnection(connection)) return;
            navCountsRefreshedAt = Date.now();
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
$('btn-cluster-structure').addEventListener('click', () => selectView(currentView === 'structure' ? 'overview' : 'structure'));
$('structure-filter').addEventListener('input', applyStructureFilter);
$('structure-only-unhealthy').addEventListener('change', applyStructureFilter);

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
    setMobileNavOpen(false);
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
    $('page-eyebrow').textContent = view === 'structure'
        ? 'Cluster'
        : (navItem?.closest('.nav-section')?.querySelector('.nav-group span')?.textContent ?? 'Workspace');
    const structureButton = $('btn-cluster-structure');
    structureButton.hidden = view !== 'overview' && view !== 'structure';
    structureButton.querySelector('span:last-child').textContent = view === 'structure' ? 'Back to overview' : 'Cluster structure';
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
    const hasTable = view !== 'overview' && view !== 'structure' && view !== 'traffic' && view !== 'sizing';
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
    // These dashboard canvases contain actionable buttons rather than table
    // rows. Remove the previous owner immediately so a slow cluster/namespace
    // response cannot leave a clickable topology from the old scope.
    if (view === 'overview') {
        $('node-status').innerHTML = '';
        $('node-status-total').textContent = '';
    }
    if (view === 'structure') {
        for (const id of ['structure-summary', 'structure-entries', 'structure-internal', 'structure-unexposed']) $(id).innerHTML = '';
        $('structure-warnings').hidden = true;
        $('structure-updated').textContent = '';
        resetStructureInspector();
    }
}

// Instant client-side filter over the current view's table rows.
$('view-filter').addEventListener('input', filterCurrentTable);

function filterCurrentTable() {
    if (currentView === 'overview' || currentView === 'structure') { $('view-count').textContent = ''; return; }
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
        case 'structure': return loadClusterStructure(scope);
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
    return OverviewSnapshot()
        .then((snapshot) => {
            if (!isCurrentViewRequest(scope)) return;
            const stats = snapshot?.stats ?? {};
            const errored = snapshot?.failingPods ?? [];
            $('stat-nodes').textContent = stats.nodes ?? 0;
            $('stat-namespaces').textContent = stats.namespaces ?? 0;
            $('stat-pods').textContent = stats.pods ?? 0;
            $('stat-deployments').textContent = stats.deployments ?? 0;
            $('stat-errors').textContent = errored.length;
            $('stat-errors').closest('.stat-card').classList.toggle('has-errors', errored.length > 0);
            updateClusterHealth(stats.podsAvailable ? (stats.pods ?? 0) : null, errored);

            const body = $('overview-errors-body');
            body.innerHTML = '';
            $('overview-errors-empty').hidden = errored.length > 0;
            for (const p of errored) {
                body.appendChild(row(
                    `<td class="overview-namespace">${esc(p.namespace)}</td><td class="overview-resource-name" title="${esc(p.name)}">${esc(p.name)}</td><td>${badge(p.status, false)}</td><td class="overview-count">${p.restarts}</td>`,
                    { isError: true, actions: false, ref: { kind: 'Pod', namespace: p.namespace, name: p.name, isPod: true } },
                ));
            }
            const diagnose = $('overview-ai-diagnose');
            diagnose.hidden = errored.length === 0;
            diagnose.onclick = errored.length === 0 ? null : () => {
                const pod = errored[0];
                openDrawer({ kind: 'Pod', namespace: pod.namespace, name: pod.name, isPod: true, tab: 'ai' });
            };

            renderCapacityMetrics(snapshot?.nodeMetrics ?? []);
            renderNodeStatus(snapshot?.nodeStatus ?? []);
            renderTopPods(snapshot?.topPods ?? []);
            renderRecentEvents(snapshot?.events ?? []);
        })
        .catch((err) => {
            if (!isCurrentViewRequest(scope)) return;
            updateClusterHealth(null, []);
            renderCapacityMetrics([]);
            renderNodeStatus([]);
            renderTopPods([]);
            renderRecentEvents([]);
            showDashError(err);
        });
}

function updateClusterHealth(total, errored) {
    const score = $('cluster-health-score');
    const badgeEl = $('cluster-health-badge');
    const fill = $('cluster-health-fill');
    if (total === null) {
        score.textContent = '–';
        $('cluster-health-summary').textContent = 'Cluster health could not be loaded.';
        $('cluster-health-ratio').textContent = '– / –';
        fill.style.width = '0%';
        fill.parentElement.removeAttribute('aria-valuenow');
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
    fill.parentElement.setAttribute('aria-valuenow', String(percent));
    $('cluster-health-note').textContent = unhealthy > 0
        ? `${unhealthy} pod${unhealthy === 1 ? '' : 's'} need attention. Open a row below for live evidence.`
        : 'Calculated from live pod status returned by the connected cluster.';
    badgeEl.textContent = unhealthy === 0 ? 'Healthy' : (percent >= 90 ? `${unhealthy} warning${unhealthy === 1 ? '' : 's'}` : 'Needs attention');
    badgeEl.className = `health-state ${unhealthy === 0 ? 'health-state-ok' : (percent >= 90 ? 'health-state-warn' : 'health-state-error')}`;
}

function renderTopPods(pods) {
    const body = $('overview-toppods-body');
    body.innerHTML = '';
    $('overview-toppods-empty').hidden = (pods?.length ?? 0) > 0;
    for (const p of pods ?? []) {
        body.appendChild(row(
            `<td class="overview-namespace">${esc(p.namespace)}</td><td class="overview-resource-name" title="${esc(p.name)}">${esc(p.name)}</td><td><span class="overview-metric mono">${p.cpuMilli}m</span></td><td><span class="overview-metric mono">${p.memMi}Mi</span></td>`,
            { actions: false, ref: { kind: 'Pod', namespace: p.namespace, name: p.name, isPod: true } },
        ));
    }
}

function renderRecentEvents(events) {
    const body = $('overview-events-body');
    body.innerHTML = '';
    $('overview-events-empty').hidden = (events?.length ?? 0) > 0;
    for (const e of events ?? []) {
        const tr = document.createElement('tr');
        const cls = e.isWarn ? 'ev-type-warn' : 'ev-type-normal';
        const count = e.count > 1 ? ` (x${e.count})` : '';
        tr.innerHTML = `<td class="${cls}">${esc(e.type)}</td><td class="mono overview-resource-name" title="${esc(e.object)}">${esc(e.object)}</td><td>${esc(e.reason)}</td><td class="overview-count">${esc(e.age)}${count}</td><td class="overview-event-message" title="${esc(e.message)}">${esc(e.message)}</td>`;
        body.appendChild(tr);
    }
}

function renderCapacityMetrics(metrics) {
    if (!metrics || metrics.length === 0) {
        updateCapacitySummary(null);
        return;
    }
    const sum = metrics.reduce((a, m) => ({
        cpu: a.cpu + (m.cpuMilli || 0), cpuCap: a.cpuCap + (m.cpuCapacity || 0),
        mem: a.mem + (m.memMi || 0), memCap: a.memCap + (m.memCapacity || 0),
    }), { cpu: 0, cpuCap: 0, mem: 0, memCap: 0 });
    updateCapacitySummary(metrics, sum);
}

function renderNodeStatus(nodes) {
    const box = $('node-status');
    const empty = $('node-status-empty');
    const total = $('node-status-total');
    box.innerHTML = '';
    empty.hidden = (nodes?.length ?? 0) > 0;
    if (!nodes?.length) {
        total.textContent = '';
        return;
    }
    const troubled = nodes.filter((node) => !node.ready || !node.schedulable || (node.pressure?.length ?? 0) > 0).length;
    total.textContent = troubled > 0
        ? `${nodes.length} nodes · ${troubled} need attention`
        : `${nodes.length} nodes · all operational`;
    box.innerHTML = nodes.map((node) => {
        const pressure = node.pressure?.length
            ? node.pressure.map((signal) => `<span class="node-signal node-signal-bad">${esc(signal.replace('Pressure', ' pressure'))}</span>`).join('')
            : '<span class="node-signal">No pressure</span>';
        const unhealthy = !node.ready || (node.pressure?.length ?? 0) > 0;
        return `<article class="node-status-card${unhealthy ? ' node-status-card-bad' : ''}">
            <header>
                <button class="node-status-name" type="button" data-name="${esc(node.name)}">${nodeNameHtml(node.name)}</button>
                ${badge(node.ready ? 'Ready' : 'Not ready', node.ready)}
            </header>
            <div class="node-status-facts">
                <span><strong>${node.pods ?? 0}</strong> scheduled pods</span>
                <span class="${node.schedulable ? '' : 'node-fact-warn'}">${node.schedulable ? 'Schedulable' : 'Scheduling disabled'}</span>
                <span class="mono">${esc(node.version || 'version unknown')}</span>
            </div>
            <div class="node-signals">${pressure}</div>
        </article>`;
    }).join('');
    box.querySelectorAll('.node-status-name').forEach((button) => {
        button.addEventListener('click', () => openDrawer({ kind: 'Node', namespace: '', name: button.dataset.name }));
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
        setCapacityState($('capacity-cpu-ring'), null);
        setCapacityState($('capacity-memory-ring'), null);
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
    setCapacityState($('capacity-cpu-ring'), cpuPercent);
    setCapacityState($('capacity-memory-ring'), memoryPercent);
    hint.hidden = true;
}

function setCapacityState(bar, value) {
    bar.classList.toggle('capacity-bar-warn', value !== null && value >= 70 && value < 90);
    bar.classList.toggle('capacity-bar-critical', value !== null && value >= 90);
    const progress = bar.querySelector('[role="progressbar"]');
    if (value === null) progress.removeAttribute('aria-valuenow');
    else progress.setAttribute('aria-valuenow', String(Math.min(value, 100)));
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
    if (drawerRef && !$('drawer').hidden) stopEphemeralForwards(drawerRef);
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
    const closingRef = drawerRef;
    stopFollow();
    stopExec();
    cancelPendingForwardForDrawer(closingRef);
    requestScopes.closeDrawer();
    activeDrawerScope = null;
    yamlReady = false;
    yamlOwnerKey = '';
    yamlRequestId++;
    drawerRef = null;
    $('drawer').hidden = true;
    $('drawer-backdrop').hidden = true;
    if (closingRef) stopEphemeralForwards(closingRef);
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
    if (name === 'terminal') requestAnimationFrame(() => {
        ensureTerminal();
        fitTerminal();
        if (execConnected) execTerminal?.focus();
    });
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

// ============ Cluster structure (Entry point → Service → Workload → Pod) ============

let structureReqId = 0;

function loadClusterStructure(scope) {
    const reqId = ++structureReqId;
    resetStructureInspector();
    return ClusterStructure(scope.namespace || '')
        .then((snapshot) => {
            if (reqId !== structureReqId || !isCurrentViewRequest(scope)) return;
            renderStructureSummary(snapshot);
            const warnings = snapshot?.warnings ?? [];
            $('structure-warnings').hidden = warnings.length === 0;
            $('structure-warnings').textContent = warnings.length ? `Partial snapshot: ${warnings.join(' · ')}` : '';

            const entries = snapshot?.entries ?? [];
            const internal = snapshot?.internal ?? [];
            const unexposed = snapshot?.unexposed ?? [];
            $('structure-entries').innerHTML = entries.flatMap(structureEntryPaths).join('');
            $('structure-internal').innerHTML = internal.flatMap((service) => structureServicePaths(service, null)).join('');
            $('structure-unexposed').innerHTML = unexposed.map((workload) => structureWorkloadPath(workload)).join('');
            $('structure-internal-count').textContent = String(internal.length);
            $('structure-unexposed-count').textContent = String(unexposed.length);
            $('structure-internal-section').hidden = internal.length === 0;
            $('structure-unexposed-section').hidden = unexposed.length === 0;
            $('structure-updated').textContent = `Updated ${new Date().toLocaleTimeString()}`;
            wireStructureNodes();
            applyStructureFilter();
        })
        .catch((err) => {
            if (reqId !== structureReqId || !isCurrentViewRequest(scope)) return;
            $('structure-summary').innerHTML = '';
            $('structure-entries').innerHTML = '';
            $('structure-internal').innerHTML = '';
            $('structure-unexposed').innerHTML = '';
            $('structure-updated').textContent = '';
            showDashError(err);
        });
}

function renderStructureSummary(snapshot) {
    const summary = snapshot?.summary ?? {};
    const tiles = [
        { n: summary.nodes ?? 0, label: 'Nodes', hint: 'cluster machines' },
        { n: summary.namespaces ?? 0, label: 'Namespaces', hint: snapshot?.scope ? 'cluster total' : 'visible scopes' },
        { n: summary.pods ?? 0, label: 'Pods', hint: snapshot?.scope ? `in ${snapshot.scope}` : 'live across cluster' },
        { n: summary.unhealthy ?? 0, label: 'Unhealthy', hint: 'not ready or failing', bad: true },
    ];
    $('structure-summary').innerHTML = tiles.map((tile) => `<article class="structure-stat${tile.bad && tile.n > 0 ? ' structure-stat-bad' : ''}">
        <strong>${tile.n}</strong><span>${esc(tile.label)}</span><small>${esc(tile.hint)}</small>
    </article>`).join('');
}

function structureEntryPaths(entry) {
    if (!(entry.services?.length)) {
        return [structurePath({ entry, problem: true, search: structureSearch(entry) })];
    }
    return entry.services.flatMap((service) => structureServicePaths(service, entry));
}

function structureServicePaths(service, entry) {
    const workloads = service.workloads ?? [];
    if (workloads.length === 0) {
        return [structurePath({ entry, service, problem: true, search: `${structureSearch(entry)} ${structureSearch(service)}` })];
    }
    return workloads.map((workload) => structurePath({
        entry, service, workload,
        problem: !!entry?.warning || !!service.warning || !!workload.isError,
        search: `${structureSearch(entry)} ${structureSearch(service)} ${structureSearch(workload)} ${(workload.pods ?? []).map(structureSearch).join(' ')}`,
    }));
}

function structureWorkloadPath(workload) {
    return structurePath({
        workload, problem: !!workload.isError,
        search: `${structureSearch(workload)} ${(workload.pods ?? []).map(structureSearch).join(' ')}`,
    });
}

function structurePath({ entry, service, workload, problem, search }) {
    const entryNode = entry
        ? structureNode(entry.refKind || entry.kind, entry.name, entry.namespace, entry.warning || 'Routing', !!entry.warning, { label: entry.kind })
        : '<span class="structure-lane-empty">Internal</span>';
    const serviceNode = service
        ? structureNode('Service', service.name, service.namespace, service.warning || service.type || 'Service', !!service.warning, { label: 'Service' })
        : '<span class="structure-lane-empty">No Service</span>';
    const workloadNode = workload
        ? structureNode(workload.kind, workload.name, workload.namespace, workload.status, !!workload.isError, { label: workload.kind })
        : '<span class="structure-void">No matching workload</span>';
    const podNodes = workload?.pods?.length
        ? workload.pods.map((pod) => structureNode('Pod', pod.name, pod.namespace, `${pod.ready} · ${pod.status}`, !!pod.isError, {
            label: 'Pod', node: pod.node, restarts: pod.restarts, isPod: true,
        })).join('')
        : '<span class="structure-void">No live pods</span>';
    return `<article class="structure-path${problem ? ' structure-path-bad' : ''}" data-problem="${problem ? '1' : '0'}" data-search="${esc(search)}">
        <div class="structure-lane">${entryNode}</div><span class="structure-arrow" aria-hidden="true">→</span>
        <div class="structure-lane">${serviceNode}</div><span class="structure-arrow" aria-hidden="true">→</span>
        <div class="structure-lane">${workloadNode}</div><span class="structure-arrow" aria-hidden="true">→</span>
        <div class="structure-lane structure-pods">${podNodes}</div>
    </article>`;
}

function structureNode(kind, name, namespace, status, isError, extra = {}) {
    const bare = String(extra.label || kind).split('.')[0];
    return `<button type="button" class="structure-node${isError ? ' structure-node-bad' : ''}"
        data-kind="${esc(kind)}" data-name="${esc(name)}" data-namespace="${esc(namespace || '')}"
        data-status="${esc(status || '')}" data-node="${esc(extra.node || '')}" data-restarts="${extra.restarts ?? ''}" data-pod="${extra.isPod ? '1' : '0'}">
        <span class="structure-node-kind">${esc(bare)}</span>
        <strong title="${esc(name)}">${esc(name)}</strong>
        ${status ? `<small>${esc(status)}</small>` : ''}
    </button>`;
}

function structureSearch(resource) {
    if (!resource) return '';
    return [resource.kind, resource.name, resource.namespace, resource.status, resource.warning, resource.node].filter(Boolean).join(' ');
}

function wireStructureNodes() {
    document.querySelectorAll('#view-structure .structure-node').forEach((button) => {
        button.addEventListener('click', () => {
            document.querySelectorAll('#view-structure .structure-node').forEach((node) => node.classList.toggle('selected', node === button));
            const ref = {
                kind: button.dataset.kind, name: button.dataset.name, namespace: button.dataset.namespace,
                isPod: button.dataset.pod === '1',
            };
            showStructureInspector(ref, {
                status: button.dataset.status, node: button.dataset.node, restarts: button.dataset.restarts,
                isError: button.classList.contains('structure-node-bad'),
            });
        });
    });
}

function resetStructureInspector() {
    $('structure-inspector-empty').hidden = false;
    $('structure-inspector-content').hidden = true;
    $('structure-inspector-content').innerHTML = '';
}

function showStructureInspector(ref, detail) {
    $('structure-inspector-empty').hidden = true;
    const content = $('structure-inspector-content');
    content.hidden = false;
    content.innerHTML = `<div class="structure-inspector-head">
            <span class="structure-inspector-kind">${esc(String(ref.kind).split('.')[0])}</span>
            ${badge(detail.status || (detail.isError ? 'Needs attention' : 'Healthy'), !detail.isError)}
        </div>
        <h3 title="${esc(ref.name)}">${esc(ref.name)}</h3>
        <p class="mono structure-inspector-ns">${esc(ref.namespace || 'cluster-scoped')}</p>
        <dl class="structure-inspector-facts">
            ${detail.status ? `<div><dt>Status</dt><dd>${esc(detail.status)}</dd></div>` : ''}
            ${detail.node ? `<div><dt>Node</dt><dd>${esc(detail.node)}</dd></div>` : ''}
            ${detail.restarts !== '' ? `<div><dt>Restarts</dt><dd>${esc(detail.restarts)}</dd></div>` : ''}
        </dl>
        <div class="structure-inspector-actions">
            <button id="structure-open-detail" class="btn btn-primary btn-sm">Open full details</button>
            ${ref.isPod ? '<button id="structure-open-logs" class="btn btn-secondary btn-sm">View logs</button>' : ''}
        </div>`;
    $('structure-open-detail').addEventListener('click', () => openDrawer(ref));
    if (ref.isPod) $('structure-open-logs').addEventListener('click', () => openDrawer({ ...ref, tab: 'logs' }));
}

function applyStructureFilter() {
    const term = $('structure-filter').value.trim().toLowerCase();
    const onlyProblems = $('structure-only-unhealthy').checked;
    const paths = [...document.querySelectorAll('#view-structure .structure-path')];
    let visible = 0;
    for (const path of paths) {
        const matches = (!term || path.dataset.search.toLowerCase().includes(term))
            && (!onlyProblems || path.dataset.problem === '1');
        path.hidden = !matches;
        if (matches) visible++;
    }
    for (const sectionId of ['structure-internal-section', 'structure-unexposed-section']) {
        const section = $(sectionId);
        const hasVisible = [...section.querySelectorAll('.structure-path')].some((path) => !path.hidden);
        section.hidden = !hasVisible;
    }
    $('structure-empty').hidden = visible > 0;
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

$('btn-yaml-save').addEventListener('click', async () => {
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
    const expectedClusterName = $('cluster-select').selectedOptions[0]?.textContent ?? expectedCluster;
    const changed = summarizeLineChanges(lineDiff(yamlOriginal, text));
    if (changed.added === 0 && changed.removed === 0) {
        setYamlStatus('No changes', '');
        return;
    }
    const where = ref.namespace ? ` in namespace “${ref.namespace}”` : '';
    try {
        await confirmedAction(
            () => showConfirm(
                `Update ${ref.kind} “${ref.name}”${where} on cluster “${expectedClusterName}”?\n`
                + `YAML diff: +${changed.added} / -${changed.removed} lines.`,
                { title: `Save ${ref.kind} YAML`, icon: '✎', okText: 'Save' },
            ),
            async () => {
                if (!isCurrentDrawerRequest(scope) || yamlOwnerKey !== ownerKey
                    || $('cluster-select').value !== expectedCluster) {
                    throw new Error('This YAML no longer belongs to the active cluster or resource. Reload it before saving.');
                }
                setYamlStatus('Saving…', '');
                await UpdateYAML(expectedCluster, ref.kind, ref.namespace, ref.name, text);
                if (!isCurrentDrawerRequest(scope) || yamlOwnerKey !== ownerKey) return;
                setYamlStatus('Saved ✓', 'ok');
                yamlOriginal = text;
                refreshCurrentView();
            },
        );
    } catch (err) {
        if (isCurrentDrawerRequest(scope) && yamlOwnerKey === ownerKey) setYamlStatus(errMsg(err), 'err');
    }
});

function setYamlStatus(text, cls) {
    const el = $('yaml-status');
    el.textContent = text;
    el.className = 'save-status' + (cls ? ' ' + cls : '');
}

// ---- Logs (static fetch + live follow + search + download) ----
const logLines = new LineRingBuffer(5000);
let following = false;
let logStreamSequence = 0;
let activeLogStreamID = '';

// The backend batches lines to avoid one Wails event per line. A stream ID is
// still required because cancellation cannot retract a batch already in flight.
EventsOn('loglines', (batch) => {
    if (!following || !batch || batch.streamId !== activeLogStreamID) return;
    logLines.pushMany(batch.lines);
    logRenderScheduler.request();
});

EventsOn('logerror', (failure) => {
    if (!following || !failure || failure.streamId !== activeLogStreamID) return;
    following = false;
    activeLogStreamID = '';
    $('logs-follow').checked = false;
    logLines.replace([failure.message || 'Log stream ended unexpectedly.']);
    logRenderScheduler.flush();
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
            logLines.replace([errMsg(err)]);
            renderLogs();
        });
}

function loadStaticLogs(scope = activeDrawerScope) {
    if (!scope || !isCurrentDrawerRequest(scope)) return;
    const ref = scope.ref;
    logLines.replace(['Loading logs…']);
    renderLogs();
    PodLogs(ref.namespace, ref.name, $('logs-container').value, LOG_TAIL_LINES)
        .then((text) => {
            if (!isCurrentDrawerRequest(scope)) return;
            logLines.replace((text || '').split('\n'));
            renderLogs();
        })
        .catch((err) => {
            if (!isCurrentDrawerRequest(scope)) return;
            logLines.replace([errMsg(err)]);
            renderLogs();
        });
}

function renderLogs() {
    const term = $('logs-search').value.trim().toLowerCase();
    const buffered = logLines.toArray();
    const lines = term ? buffered.filter((l) => l.toLowerCase().includes(term)) : buffered;
    const view = $('logs-view');
    const atBottom = view.scrollHeight - view.scrollTop - view.clientHeight < 40;
    view.textContent = lines.join('\n') || '(no logs)';
    if (following || atBottom) view.scrollTop = view.scrollHeight;
}

const logRenderScheduler = createFrameScheduler(renderLogs);

function startFollow() {
    const ref = drawerRef;
    const scope = activeDrawerScope;
    if (!ref || !ref.isPod) return;
    const streamID = `${requestScopes.drawerOwnerKey(scope)}:${++logStreamSequence}`;
    following = true;
    activeLogStreamID = streamID;
    logLines.clear();
    renderLogs();
    StartLogStream(ref.namespace, ref.name, $('logs-container').value, streamID).catch((err) => {
        if (!isCurrentDrawerRequest(scope) || activeLogStreamID !== streamID) return;
        following = false;
        activeLogStreamID = '';
        $('logs-follow').checked = false;
        logLines.replace([errMsg(err)]);
        renderLogs();
    });
}

function stopFollow() {
    activeLogStreamID = '';
    logRenderScheduler.cancel();
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
    SaveTextToFile(name, logLines.toArray().join('\n'))
        .catch((err) => showError(errMsg(err)));
});

// ---- Terminal (PTY-backed interactive exec) ----
let execConnected = false;
let execSessionID = '';
let execSessionSeq = 0;
let execAcceptOutput = false;
let execTerminal = null;
let execFitAddon = null;
let execResizeObserver = null;
let execInputEpoch = 0;
let execWriter = () => Promise.resolve();
let execHasOutput = false;

function ensureTerminal() {
    if (execTerminal) return execTerminal;
    execTerminal = new Terminal({
        allowProposedApi: false,
        convertEol: false,
        cursorBlink: true,
        cursorStyle: 'bar',
        fontFamily: '"Cascadia Code", "SFMono-Regular", Consolas, "Liberation Mono", monospace',
        fontSize: 13,
        lineHeight: 1.18,
        scrollback: 5000,
        theme: {
            background: '#20242d', foreground: '#d8dee9', cursor: '#88c0d0', cursorAccent: '#20242d',
            selectionBackground: '#4c566a99', black: '#3b4252', red: '#bf616a', green: '#a3be8c',
            yellow: '#ebcb8b', blue: '#81a1c1', magenta: '#b48ead', cyan: '#88c0d0', white: '#e5e9f0',
            brightBlack: '#4c566a', brightRed: '#d06f79', brightGreen: '#b1d196', brightYellow: '#f0d399',
            brightBlue: '#8fafd2', brightMagenta: '#c19acb', brightCyan: '#9ad3df', brightWhite: '#eceff4',
        },
    });
    execFitAddon = new FitAddon();
    execTerminal.loadAddon(execFitAddon);
    execTerminal.open($('term-surface'));
    execTerminal.onData((data) => {
        if (execConnected) execWriter(data);
    });
    execTerminal.onResize(({ cols, rows }) => {
        const size = validTerminalSize(cols, rows);
        if (execConnected && size) ExecResize(size.cols, size.rows);
    });
    execResizeObserver = new ResizeObserver(() => {
        if (!$('dpanel-terminal').hidden) requestAnimationFrame(fitTerminal);
    });
    execResizeObserver.observe($('term-surface'));
    return execTerminal;
}

function fitTerminal() {
    if (!execTerminal || $('dpanel-terminal').hidden || $('term-surface').clientWidth < 1) return null;
    try {
        execFitAddon.fit();
        return validTerminalSize(execTerminal.cols, execTerminal.rows);
    } catch {
        return null;
    }
}

function setTerminalStatus(state, text) {
    const status = $('term-status');
    status.className = `term-status${state ? ` ${state}` : ''}`;
    status.innerHTML = `<i></i>${esc(text)}`;
}

function resetTerminalUI({ clear = false, preserveOutput = false } = {}) {
    execConnected = false;
    execAcceptOutput = false;
    execSessionID = '';
    execInputEpoch++;
    execWriter = () => Promise.resolve();
    $('btn-term-start').hidden = false;
    $('btn-term-start').disabled = false;
    $('btn-term-stop').hidden = true;
    $('term-container').disabled = false;
    $('term-shell').disabled = false;
    $('term-session-label').textContent = 'Not connected';
    setTerminalStatus('', 'Disconnected');
    if (clear) {
        execHasOutput = false;
        if (execTerminal) execTerminal.reset();
    }
    $('term-placeholder').hidden = preserveOutput && execHasOutput;
}

EventsOn('exec-output', (event) => {
    if (!execAcceptOutput || event?.sessionId !== execSessionID) return;
    execHasOutput = true;
    ensureTerminal().write(String(event.data ?? ''));
});
EventsOn('exec-closed', (event) => {
    if (!execAcceptOutput || event?.sessionId !== execSessionID) return;
    const msg = event.message ?? '';
    const suffix = msg ? `Session ended · ${msg}` : 'Session ended';
    execHasOutput = true;
    ensureTerminal().write(`\r\n\x1b[90m[${suffix}]\x1b[0m\r\n`);
    resetTerminalUI({ preserveOutput: true });
});

function prepareTerminal(scope = activeDrawerScope) {
    if (!scope || !isCurrentDrawerRequest(scope)) return;
    const ref = scope.ref;
    const select = $('term-container');
    select.innerHTML = '';
    resetTerminalUI({ clear: true });
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
        .catch((err) => {
            if (!isCurrentDrawerRequest(scope)) return;
            setTerminalStatus('error', errMsg(err));
        });
}

$('btn-term-start').addEventListener('click', () => {
    const ref = drawerRef;
    const scope = activeDrawerScope;
    if (!ref || !ref.isPod) return;
    const container = $('term-container').value;
    const shell = $('term-shell').value;
    const terminal = ensureTerminal();
    const size = fitTerminal() ?? { cols: 80, rows: 24 };
    const inputEpoch = ++execInputEpoch;
    const sessionID = `exec-${Date.now()}-${++execSessionSeq}`;
    execSessionID = sessionID;
    execAcceptOutput = true;
    execConnected = false;
    execHasOutput = false;
    terminal.reset();
    $('term-placeholder').hidden = true;
    $('btn-term-start').disabled = true;
    $('term-container').disabled = true;
    $('term-shell').disabled = true;
    $('term-session-label').textContent = `${ref.name} · ${container || 'default container'}`;
    setTerminalStatus('connecting', 'Connecting…');
    StartExec(sessionID, ref.namespace, ref.name, container, shell, size.cols, size.rows)
        .then(() => {
            if (!isCurrentDrawerRequest(scope) || inputEpoch !== execInputEpoch) { StopExec(); return; }
            execConnected = true;
            execWriter = createSerialWriter(
                (data) => inputEpoch === execInputEpoch ? ExecWrite(data) : Promise.resolve(),
                (err) => {
                    if (inputEpoch === execInputEpoch) setTerminalStatus('error', `Input failed: ${errMsg(err)}`);
                },
            );
            setTerminalStatus('connected', `Connected via ${shell}`);
            $('btn-term-start').hidden = true;
            $('btn-term-start').disabled = false;
            $('btn-term-stop').hidden = false;
            ExecResize(terminal.cols, terminal.rows);
            terminal.focus();
        })
        .catch((err) => {
            if (!isCurrentDrawerRequest(scope) || inputEpoch !== execInputEpoch) return;
            resetTerminalUI({ preserveOutput: true });
            setTerminalStatus('error', errMsg(err));
        });
});

$('btn-term-stop').addEventListener('click', stopExec);

function stopExec() {
    const hadSession = execConnected || execAcceptOutput;
    resetTerminalUI({ preserveOutput: hadSession });
    if (hadSession) StopExec();
}

// ---- Port forward ----
const PF_KEEP_PREF = 'kubby.portForward.keepRunning';
let activeForwards = [];
let forwardStateVersion = 0;
let forwardStartId = 0;
let pendingForwardOperation = null;
let toastForwardKey = '';
let pfToastTimer = null;

function preferredKeepRunning() {
    try { return localStorage.getItem(PF_KEEP_PREF) !== 'false'; }
    catch { return true; }
}

function saveKeepRunningPreference(value) {
    try { localStorage.setItem(PF_KEEP_PREF, value ? 'true' : 'false'); }
    catch { /* WebView storage can be unavailable in restricted profiles. */ }
}

function renderAllForwards() {
    renderForwards();
    renderPortForwardManager();
}

function hydratePortForwards() {
    const token = requestScopes.connectionToken();
    const version = forwardStateVersion;
    return ListPortForwards()
        .then((forwards) => {
            if (!canApplyForwardHydration({
                connectionCurrent: requestScopes.isCurrentConnection(token),
                requestedVersion: version,
                currentVersion: forwardStateVersion,
            })) return;
            activeForwards = [];
            for (const forward of forwards ?? []) activeForwards = upsertForward(activeForwards, forward);
            forwardStateVersion++;
            renderAllForwards();
        })
        .catch((err) => {
            if (requestScopes.isCurrentConnection(token)) showDashError(err);
        });
}

function stopForward(key) {
    activeForwards = removeForward(activeForwards, key);
    forwardStateVersion++;
    if (toastForwardKey === key) hideForwardToast();
    renderAllForwards();
    return Promise.resolve(StopPortForward(key)).catch((err) => showError(errMsg(err)));
}

function stopKnownPortForwards() {
    const keys = activeForwards.map((forward) => forward.key);
    activeForwards = [];
    forwardStateVersion++;
    hideForwardToast();
    closePortForwardManager();
    renderAllForwards();
    for (const key of keys) Promise.resolve(StopPortForward(key)).catch(() => {});
}

function stopEphemeralForwards(ref) {
    for (const key of forwardsToStopOnDrawerClose(activeForwards, ref)) stopForward(key);
}

EventsOn('portforward-closed', (event) => {
	if (!isCurrentForwardEvent(event, $('cluster-select').value)) return;
    const key = event.key;
    activeForwards = removeForward(activeForwards, key);
    forwardStateVersion++;
    if (toastForwardKey === key) hideForwardToast();
    renderAllForwards();
});

function prepareForward() {
    forwardStartId++;
    $('pf-error').hidden = true;
    $('pf-local').value = '0';
    $('pf-remote').value = '';
    $('pf-keep-running').checked = preferredKeepRunning();
    $('btn-pf-start').disabled = false;
    $('btn-pf-start').textContent = 'Start forward';
    renderAllForwards();
}

function cancelPendingForwardForDrawer(ref) {
    const pending = pendingForwardOperation;
    if (!shouldCancelPendingForward(pending, ref)) return;
    pendingForwardOperation = null;
    Promise.resolve(CancelPortForwardStart(pending.id)).catch(() => {});
}

$('pf-keep-running').addEventListener('change', (event) => saveKeepRunningPreference(event.target.checked));

$('btn-pf-start').addEventListener('click', () => {
    const ref = drawerRef ? { ...drawerRef } : null;
    const scope = activeDrawerScope;
    const connectionToken = requestScopes.connectionToken();
    if (!ref || !scope || !isCurrentDrawerRequest(scope)) return;
    const remote = parseInt($('pf-remote').value, 10);
    const local = parseInt($('pf-local').value, 10) || 0;
    const keepRunning = $('pf-keep-running').checked;
    if (Number.isNaN(remote) || remote < 1 || remote > 65535) {
        showPfError('Enter a valid remote port (1–65535).');
        return;
    }
    const requestId = ++forwardStartId;
    const operationID = `pf-${Date.now()}-${requestId}`;
    pendingForwardOperation = {
        id: operationID,
        kind: ref.kind,
        namespace: ref.namespace,
        name: ref.name,
        keepRunning,
    };
    $('pf-error').hidden = true;
    const btn = $('btn-pf-start');
    btn.disabled = true;
    btn.textContent = 'Starting…';
    StartPortForward(operationID, ref.kind, ref.namespace, ref.name, local, remote, keepRunning)
        .then((info) => {
            const drawerStillOwnsRequest = isCurrentDrawerRequest(scope);
            if (!shouldRetainStartedForward({ drawerStillOwnsRequest, keepRunning })) {
                StopPortForward(info.key);
                return;
            }
            activeForwards = upsertForward(activeForwards, info);
            forwardStateVersion++;
            renderAllForwards();
            showForwardToast(info);
        })
        .catch((err) => {
            if (isCurrentDrawerRequest(scope)) showPfError(errMsg(err));
            else if (keepRunning && requestScopes.isCurrentConnection(connectionToken)) {
                showError(errMsg(err), 'Port forward failed');
            }
        })
        .finally(() => {
            if (pendingForwardOperation?.id === operationID) pendingForwardOperation = null;
            if (!isCurrentDrawerRequest(scope) || requestId !== forwardStartId) return;
            btn.disabled = false;
            btn.textContent = 'Start forward';
        });
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
    $('pf-resource-count').textContent = String(mine.length);
    list.innerHTML = '';
    for (const f of mine) {
        const div = document.createElement('div');
        div.className = 'pf-item';
        div.innerHTML =
            `<span class="pf-status-dot" aria-hidden="true"></span>
            <div class="pf-item-main">
                <div class="pf-item-route">
                    <span class="pf-addr mono">localhost:${f.localPort}</span>
                    <span class="pf-arrow">→</span>
                    <span class="pf-target mono" title="${esc(f.podName)}:${f.remotePort}">${esc(f.podName)}:${f.remotePort}</span>
                </div>
                <span class="pf-item-meta">${f.keepRunning ? 'Runs in background' : 'Stops when this drawer closes'}</span>
            </div>`;
        const stop = document.createElement('button');
        stop.className = 'btn btn-secondary btn-sm';
        stop.textContent = 'Stop';
        stop.addEventListener('click', () => stopForward(f.key));
        div.appendChild(stop);
        list.appendChild(div);
    }
}

function renderPortForwardManager() {
    const count = activeForwards.length;
    $('pf-global-count').textContent = String(count);
    $('btn-port-forwards').classList.toggle('has-forwards', count > 0);
    $('pf-manager-summary').textContent = count === 0 ? 'No active tunnels' : `${count} running in this cluster`;
    $('pf-stop-all').hidden = count === 0;
    $('pf-global-empty').hidden = count > 0;
    const list = $('pf-global-list');
    list.innerHTML = '';
    for (const forward of activeForwards) {
        const item = document.createElement('div');
        item.className = 'pf-global-item';
        item.innerHTML = `<div class="pf-global-main">
                <span class="pf-global-owner">${esc(forward.kind)} · ${esc(forward.namespace)}/${esc(forward.name)}</span>
                <span class="pf-global-route mono"><strong>localhost:${forward.localPort}</strong><span>→</span><span class="pf-global-route-target" title="${esc(forward.podName)}:${forward.remotePort}">${esc(forward.podName)}:${forward.remotePort}</span></span>
                ${forward.keepRunning ? '<span class="pf-background-chip">Background tunnel</span>' : '<span class="pf-hint">Drawer-owned tunnel</span>'}
            </div>
            <div class="pf-global-actions">
                <button class="btn btn-secondary btn-sm pf-copy">Copy</button>
                <button class="btn btn-secondary btn-sm pf-open">Open</button>
                <button class="btn btn-danger btn-sm pf-stop">Stop</button>
            </div>`;
        const endpoint = `localhost:${forward.localPort}`;
        item.querySelector('.pf-copy').addEventListener('click', (event) => {
            const button = event.currentTarget;
            Promise.resolve(CopyToClipboard(endpoint)).then(() => {
                button.textContent = 'Copied';
                setTimeout(() => { button.textContent = 'Copy'; }, 1200);
            }).catch((err) => showError(errMsg(err)));
        });
        item.querySelector('.pf-open').addEventListener('click', () => BrowserOpenURL(`http://${endpoint}`));
        item.querySelector('.pf-stop').addEventListener('click', () => stopForward(forward.key));
        list.appendChild(item);
    }
}

function setPortForwardManagerOpen(open) {
    $('pf-manager').hidden = !open;
    $('btn-port-forwards').setAttribute('aria-expanded', String(open));
}

function closePortForwardManager() { setPortForwardManagerOpen(false); }

$('btn-port-forwards').addEventListener('click', () => setPortForwardManagerOpen($('pf-manager').hidden));
$('pf-stop-all').addEventListener('click', () => {
    showConfirm(`Stop all ${activeForwards.length} active port forwards?`, {
        title: 'Stop all tunnels', icon: '⇄', okText: 'Stop all', danger: true,
    }).then((ok) => { if (ok) stopKnownPortForwards(); });
});
document.addEventListener('click', (event) => {
    if (!$('pf-manager').hidden && !event.target.closest('.pf-manager-wrap')) closePortForwardManager();
});

function hideForwardToast() {
    if (pfToastTimer) clearTimeout(pfToastTimer);
    pfToastTimer = null;
    toastForwardKey = '';
    $('pf-toast').hidden = true;
}

function showForwardToast(forward) {
    hideForwardToast();
    toastForwardKey = forward.key;
    $('pf-toast-message').textContent = `localhost:${forward.localPort} → ${forward.namespace}/${forward.name}`;
    $('pf-toast').hidden = false;
    pfToastTimer = setTimeout(hideForwardToast, 7000);
}

$('pf-toast-manage').addEventListener('click', () => {
    hideForwardToast();
    setPortForwardManagerOpen(true);
});
$('pf-toast-stop').addEventListener('click', () => {
    const key = toastForwardKey;
    hideForwardToast();
    if (key) stopForward(key);
});

renderAllForwards();

// ============ Generic modal (scale / create / import) ============

let modalOnOk = null;
let modalOnExtra = null;
let activeModalScope = null;

// YAML editors living inside the current modal, keyed by host element id. They
// must be destroyed when the modal closes — closeModal wipes the body's HTML,
// and a CodeMirror instance left pointing at removed nodes leaks its listeners.
let modalEditors = {};

function mountModalEditor(id, { value = '', placeholder = '', onChange } = {}) {
    const host = $(id);
    if (!host) return null;
    const ed = createYamlEditor(host, { value, placeholder, onChange });
    modalEditors[id] = ed;
    return ed;
}

function modalEditor(id) { return modalEditors[id] || null; }
function modalYaml(id) { return modalEditors[id] ? modalEditors[id].getValue() : ''; }
function setModalYaml(id, text) { if (modalEditors[id]) modalEditors[id].setValue(text); }

function disposeModalContent() {
    for (const id of Object.keys(modalEditors)) modalEditors[id].destroy();
    modalEditors = {};
    $('modal-body').innerHTML = '';
}

function isCurrentModalRequest(scope) {
    return activeModalScope === scope
        && !$('modal').hidden
        && requestScopes.isCurrentModal(scope);
}

function openModal({ title, bodyHtml, okText = 'OK', onOk, onOpen, extraText = '', onExtra = null,
    ownerKey = `modal\0${title}`, okDisabled = false }) {
    // A modal can transition directly into another modal (chart search → install).
    // Dispose the old content before reusing its IDs and invalidate every callback
    // that still belongs to it.
    disposeModalContent();
    const scope = requestScopes.openModal(ownerKey);
    activeModalScope = scope;
    $('modal-title').textContent = title;
    $('modal-body').innerHTML = bodyHtml;
    $('modal-ok').textContent = okText;
    $('modal-ok').disabled = okDisabled;
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
    return scope;
}

function closeModal(scope = null) {
    // Async completion from A must never close a newer modal B.
    if (scope && activeModalScope !== scope) return;
    requestScopes.closeModal();
    activeModalScope = null;
    $('modal').hidden = true;
    $('modal-backdrop').hidden = true;
    disposeModalContent();
    $('modal-extra').hidden = true;
    $('modal-extra').disabled = false;
    $('modal-ok').disabled = false;
    modalOnOk = null;
    modalOnExtra = null;
}

function modalError(msg, scope = activeModalScope) {
    if (!isCurrentModalRequest(scope)) return;
    const el = $('modal-error');
    el.textContent = msg;
    el.hidden = false;
}

function submitModal() {
    if (!modalOnOk) { closeModal(); return; }
    const scope = activeModalScope;
    const onOk = modalOnOk;
    if (!isCurrentModalRequest(scope)) return;
    const btn = $('modal-ok');
    btn.disabled = true;
    $('modal-error').hidden = true;
    Promise.resolve()
        .then(() => onOk())
        .then(() => { if (isCurrentModalRequest(scope)) closeModal(scope); })
        .catch((err) => modalError(errMsg(err), scope))
        .finally(() => { if (isCurrentModalRequest(scope)) btn.disabled = false; });
}

// The secondary action runs in place — unlike OK it does not close the modal,
// because its whole purpose is to show you something before you commit.
$('modal-extra').addEventListener('click', () => {
    if (!modalOnExtra) return;
    const scope = activeModalScope;
    const onExtra = modalOnExtra;
    if (!isCurrentModalRequest(scope)) return;
    const btn = $('modal-extra');
    btn.disabled = true;
    $('modal-error').hidden = true;
    Promise.resolve()
        .then(() => onExtra())
        .catch((err) => modalError(errMsg(err), scope))
        .finally(() => { if (isCurrentModalRequest(scope)) btn.disabled = false; });
});

$('modal-ok').addEventListener('click', submitModal);
// Do not pass closeModal directly: addEventListener supplies a MouseEvent as
// argument 1, which the scope-aware closeModal would interpret as a stale scope.
$('modal-cancel').addEventListener('click', () => closeModal());
$('modal-close').addEventListener('click', () => closeModal());
$('modal-backdrop').addEventListener('click', () => closeModal());

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
        if (!$('pf-manager').hidden) { closePortForwardManager(); return; }
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
    const verb = paused ? 'Pause' : 'Resume';
    const cluster = $('cluster-select').value;
    const clusterName = $('cluster-select').selectedOptions[0]?.textContent ?? cluster;
    confirmedAction(
        () => showConfirm(
            `${verb} rollout for Deployment “${ref.name}” in namespace “${ref.namespace}” on cluster “${clusterName}”?`,
            { title: `${verb} deployment`, icon: paused ? '⏸' : '▶', okText: verb },
        ),
        () => {
            if ($('cluster-select').value !== cluster) throw new Error('The active cluster changed before the action started.');
            return SetDeploymentPaused(ref.namespace, ref.name, paused);
        },
    )
        .then((changed) => { if (changed) refreshCurrentView(); })
        .catch((err) => showError(errMsg(err)));
}

function nodeSchedule(ref, schedulable) {
    const verb = schedulable ? 'Uncordon' : 'Cordon';
    const cluster = $('cluster-select').value;
    const clusterName = $('cluster-select').selectedOptions[0]?.textContent ?? cluster;
    confirmedAction(
        () => showConfirm(
            `${verb} Node “${ref.name}” on cluster “${clusterName}”?`,
            { title: `${verb} node`, icon: schedulable ? '🟢' : '🚫', okText: verb },
        ),
        () => {
            if ($('cluster-select').value !== cluster) throw new Error('The active cluster changed before the action started.');
            return SetNodeSchedulable(ref.name, schedulable);
        },
    )
        .then((changed) => { if (changed) refreshCurrentView(); })
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

function modalOwner(type, ...parts) {
    return [type, ...parts].join('\u0000');
}

function openHelmDetailModal(ref) {
    const scope = openModal({
        title: `Helm — ${ref.name}`,
        ownerKey: modalOwner('helm-detail', ref.namespace, ref.name),
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
    const metaBox = $('helm-meta');
    const resourcesBox = $('helm-resources');
    const contentBox = $('helm-content');
    const tabs = [...$('modal-body').querySelectorAll('.helm-tab')];
    const runTestsButton = $('helm-run-tests');
    HelmGet(ref.namespace, ref.name)
        .then((d) => {
            if (!isCurrentModalRequest(scope)) return;
            metaBox.innerHTML = `<span class="chip">chart: ${esc(d.chart)}</span> <span class="chip">app: ${esc(d.appVersion || '-')}</span> <span class="chip">rev ${d.revision}</span> <span class="chip">${esc(d.status)}</span>`;
            const panes = { values: d.values || '(no user-supplied values)', manifest: d.manifest || '', notes: d.notes || '(no notes)' };
            const show = (tab) => {
                const isRes = tab === 'resources';
                resourcesBox.hidden = !isRes;
                contentBox.hidden = isRes;
                if (!isRes) contentBox.textContent = panes[tab];
            };
            show('resources');
            tabs.forEach((t) => {
                t.addEventListener('click', () => {
                    tabs.forEach((x) => x.classList.toggle('active', x === t));
                    show(t.dataset.htab);
                });
            });
        })
        .catch((err) => {
            if (isCurrentModalRequest(scope)) metaBox.innerHTML = `<p class="error">${esc(errMsg(err))}</p>`;
        });

    // Resources tab: live health of every object the release owns.
    loadHelmReleaseResources(ref, scope, resourcesBox);

    runTestsButton.addEventListener('click', async () => {
        const btn = runTestsButton;
        const cluster = $('cluster-select').value;
        const clusterName = $('cluster-select').selectedOptions[0]?.textContent ?? cluster;
        try {
            await confirmedAction(
                () => showConfirm(
                    `Run Helm tests for release “${ref.name}” in namespace “${ref.namespace}” on cluster “${clusterName}”?\n`
                    + 'Test hooks may create or delete cluster resources.',
                    { title: 'Run Helm tests', icon: '🧪', okText: 'Run tests' },
                ),
                async () => {
                    if (!isCurrentModalRequest(scope) || $('cluster-select').value !== cluster) {
                        throw new Error('The active cluster or release changed before the test started.');
                    }
                    btn.disabled = true; btn.textContent = 'Testing…';
                    const out = await HelmTest(ref.namespace, ref.name);
                    if (isCurrentModalRequest(scope)) showAlert(out, { title: `Test results — ${ref.name}`, icon: '🧪' });
                },
            );
        } catch (err) {
            if (isCurrentModalRequest(scope)) showError(errMsg(err));
        } finally {
            if (!isCurrentModalRequest(scope)) return;
            btn.disabled = false;
            btn.textContent = 'Run tests';
        }
    });
}

function loadHelmReleaseResources(ref, scope, box) {
    HelmReleaseResources(ref.namespace, ref.name)
        .then((list) => {
            if (!isCurrentModalRequest(scope)) return;
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
        .catch((err) => { if (isCurrentModalRequest(scope)) box.innerHTML = `<p class="error">${esc(errMsg(err))}</p>`; });
}

function openHelmHistoryModal(ref) {
    const scope = openModal({
        title: `Helm history — ${ref.name}`,
        ownerKey: modalOwner('helm-history', ref.namespace, ref.name),
        okText: 'Close',
        bodyHtml: `<div id="helm-history" class="rollout-list"><p class="empty-inline">Loading…</p></div>`,
        onOk: () => Promise.resolve(),
    });
    const historyBox = $('helm-history');
    HelmHistory(ref.namespace, ref.name)
        .then((revs) => {
            if (!isCurrentModalRequest(scope)) return;
            const box = historyBox;
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
                            if (!isCurrentModalRequest(scope) || pre.dataset.rev !== String(rev)) return;
                            renderDiffInto(pre, older.manifest, current.manifest);
                            pre.scrollIntoView({ block: 'nearest' });
                        })
                        .catch((err) => {
                            if (isCurrentModalRequest(scope) && pre.dataset.rev === String(rev)) pre.textContent = errMsg(err);
                        });
                });
            });
            box.querySelectorAll('.helm-rollback-btn').forEach((btn) => {
                btn.addEventListener('click', () => {
                    const rev = parseInt(btn.dataset.rev, 10);
                    showConfirm(`Rollback release “${ref.name}” to revision ${rev}?`, { title: 'Rollback release', icon: '↩', okText: 'Rollback' }).then((ok) => {
                        if (!ok || !isCurrentModalRequest(scope)) return;
                        btn.disabled = true;
                        HelmRollback(ref.namespace, ref.name, rev)
                            .then(() => {
                                if (isCurrentModalRequest(scope)) closeModal(scope);
                                refreshCurrentView();
                            })
                            .catch((err) => {
                                if (!isCurrentModalRequest(scope)) return;
                                showError(errMsg(err));
                                btn.disabled = false;
                            });
                    });
                });
            });
        })
        .catch((err) => { if (isCurrentModalRequest(scope)) historyBox.innerHTML = `<p class="error">${esc(errMsg(err))}</p>`; });
}

function openHelmUpgradeModal(ref) {
    let valuesEditor = null;
    const scope = openModal({
        title: `Upgrade values — ${ref.name}`,
        ownerKey: modalOwner('helm-upgrade', ref.namespace, ref.name),
        okText: 'Upgrade',
        okDisabled: true,
        bodyHtml: `<p class="modal-hint">Edit the release values, then <strong>Preview</strong> the rendered diff before you Upgrade (reuses the current chart).</p>
            <div id="helm-values" class="yaml-host yaml-host-modal"></div>
            <div class="install-actions">
                <button id="helm-preview-btn" class="btn btn-secondary btn-sm">👁 Preview diff</button>
                <span id="helm-preview-status" class="modal-hint"></span>
            </div>
            <pre id="helm-upg-diff" class="helm-content diff-view" hidden></pre>`,
        onOpen: () => { valuesEditor = mountModalEditor('helm-values', { value: '# loading…' }); },
        onOk: () => {
            if (!isCurrentModalRequest(scope) || !valuesEditor) return Promise.reject('This Upgrade dialog is stale. Reopen it.');
            const vals = valuesEditor.getValue();
            return HelmUpgradeValues(ref.namespace, ref.name, vals).then(() => refreshCurrentView());
        },
    });
    const okButton = $('modal-ok');
    const previewButton = $('helm-preview-btn');
    const previewBox = $('helm-upg-diff');
    const previewStatus = $('helm-preview-status');
    previewButton.disabled = true;
    HelmGet(ref.namespace, ref.name)
        .then((d) => {
            if (!isCurrentModalRequest(scope)) return;
            valuesEditor.setValue(d.values || '');
            okButton.disabled = false;
            previewButton.disabled = false;
        })
        .catch((err) => {
            if (!isCurrentModalRequest(scope)) return;
            valuesEditor.setValue('');
            modalError(`Could not load current values: ${errMsg(err)}`, scope);
        });

    let previewReqId = 0;
    previewButton.addEventListener('click', () => {
        if (!isCurrentModalRequest(scope) || !valuesEditor) return;
        const vals = valuesEditor.getValue();
        const pre = previewBox;
        const btn = previewButton;
        const status = previewStatus;
        if (!pre.hidden) {
            previewReqId++;
            pre.hidden = true;
            status.textContent = '';
            btn.textContent = '👁 Preview diff';
            return;
        }
        const reqId = ++previewReqId;
        pre.hidden = false; pre.textContent = 'Rendering (dry-run)…'; status.textContent = ''; btn.textContent = '✕ Hide preview';
        HelmUpgradePreview(ref.namespace, ref.name, vals)
            .then((diff) => {
                if (!isCurrentModalRequest(scope) || reqId !== previewReqId) return;
                renderDiffInto(pre, diff.current, diff.proposed);
                status.textContent = 'Diff = current manifest → what Upgrade would apply.';
            })
            .catch((err) => {
                if (isCurrentModalRequest(scope) && reqId === previewReqId) pre.textContent = errMsg(err);
            });
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
    const scope = openModal({
        title: 'Search & install charts',
        ownerKey: 'chart-search',
        okText: 'Close',
        bodyHtml: `<div class="chart-search-row">
                <input id="chart-query" class="pf-input no-enter-submit" style="flex:1" type="text" placeholder="Search Artifact Hub (e.g. nginx, redis, prometheus)…" autocomplete="off">
                <button id="chart-search-go" class="btn btn-primary btn-sm">Search</button>
            </div>
            <p class="modal-hint">Requires internet access to artifacthub.io.</p>
            <div id="chart-results" class="chart-results"></div>`,
        onOk: () => Promise.resolve(),
    });
    const queryInput = $('chart-query');
    const resultsBox = $('chart-results');
    const searchButton = $('chart-search-go');
    let searchReqId = 0;
    const run = () => {
        if (!isCurrentModalRequest(scope)) return;
        const q = queryInput.value.trim();
        if (!q) return;
        const reqId = ++searchReqId;
        resultsBox.innerHTML = '<p class="empty-inline">Searching…</p>';
        SearchCharts(q)
            .then((results) => {
                if (isCurrentModalRequest(scope) && reqId === searchReqId) renderChartResults(results, resultsBox);
            })
            .catch((err) => {
                if (isCurrentModalRequest(scope) && reqId === searchReqId) resultsBox.innerHTML = `<p class="error">${esc(errMsg(err))}</p>`;
            });
    };
    searchButton.addEventListener('click', run);
    queryInput.addEventListener('keydown', (e) => { if (e.key === 'Enter') { e.preventDefault(); run(); } });
    queryInput.focus();
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
    let valuesEditor = null;
    let defaultsLoading = false;
    let loadedDefaultsVersion = null;
    let approvedPreview = null;
    let okButton = null;
    let statusBox = null;
    const invalidatePreview = (message = '') => {
        approvedPreview = null;
        if (okButton) okButton.disabled = true;
        if (message && statusBox) statusBox.textContent = message;
    };
    const scope = openModal({
        title: `Install ${chart.name}`,
        ownerKey: modalOwner('chart-install', chart.repoURL, chartName),
        okText: 'Install',
        okDisabled: true,
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
        onOpen: () => {
            valuesEditor = mountModalEditor('inst-values', {
                placeholder: '# leave empty for chart defaults, or click “Load chart defaults”',
                onChange: () => invalidatePreview('Values changed — preview again before installing.'),
            });
        },
        onOk: () => {
            if (!isCurrentModalRequest(scope) || !valuesEditor) return Promise.reject('This Install dialog is stale. Reopen it.');
            if (defaultsLoading) return Promise.reject('Wait for chart defaults to finish loading.');
            const name = nameInput.value.trim();
            const nsv = namespaceInput.value.trim() || 'default';
            const ver = versionSelect.value.trim();
            const vals = valuesEditor.getValue();
            if (!name) return Promise.reject('Enter a release name.');
            if (!approvedPreview || approvedPreview.name !== name || approvedPreview.namespace !== nsv
                || approvedPreview.version !== ver || approvedPreview.values !== vals) {
                return Promise.reject('Preview this exact release, namespace, version, and values before installing.');
            }
            return HelmInstall(nsv, name, chart.repoURL, chartName, ver, vals, approvedPreview.digest)
                .then(() => { selectView('helm'); loadSidebarCounts(); });
        },
    });
    const modalBody = $('modal-body');
    const nameInput = $('inst-name');
    const namespaceInput = $('inst-ns');
    const versionSelect = $('inst-ver');
    const linksBox = $('inst-links');
    const readmeBox = $('inst-readme');
    const valuesPane = $('inst-pane-values');
    const defaultsButton = $('inst-load-defaults');
    const previewButton = $('inst-preview');
    statusBox = $('inst-status');
    const diffBox = $('inst-diff');
    okButton = $('modal-ok');
    const installTabs = [...modalBody.querySelectorAll('.install-tab')];

    // Tabs: Values / README.
    installTabs.forEach((t) => {
        t.addEventListener('click', () => {
            installTabs.forEach((x) => x.classList.toggle('active', x === t));
            const readme = t.dataset.itab === 'readme';
            valuesPane.hidden = readme;
            readmeBox.hidden = !readme;
        });
    });

    // Load chart defaults into the values editor (authoritative, from the repo).
    let defaultsReqId = 0;
    versionSelect.addEventListener('change', () => {
        // Defaults belong to an exact chart version. A version switch invalidates
        // both an in-flight download and defaults already loaded for the old one.
        defaultsReqId++;
        defaultsLoading = false;
        defaultsButton.disabled = false;
        defaultsButton.textContent = '↓ Load chart defaults';
        previewButton.disabled = false;
        invalidatePreview('Version changed — preview again before installing.');
        if (loadedDefaultsVersion !== null && loadedDefaultsVersion !== versionSelect.value.trim()) {
            valuesEditor.setValue('');
            loadedDefaultsVersion = null;
            statusBox.textContent = 'Version changed — old defaults were cleared. Load defaults for this version.';
        }
    });
    defaultsButton.addEventListener('click', () => {
        if (!isCurrentModalRequest(scope) || !valuesEditor) return;
        const btn = defaultsButton;
        const requestedVersion = versionSelect.value.trim();
        const reqId = ++defaultsReqId;
        defaultsLoading = true;
        okButton.disabled = true;
        previewButton.disabled = true;
        btn.disabled = true; btn.textContent = 'Loading…';
        ChartDefaultValues(chart.repoURL, chartName, requestedVersion)
            .then((vals) => {
                if (!isCurrentModalRequest(scope) || reqId !== defaultsReqId) return;
                if (versionSelect.value.trim() !== requestedVersion) {
                    statusBox.textContent = 'Version changed — load defaults again for the selected version.';
                    return;
                }
                valuesEditor.setValue(vals);
                loadedDefaultsVersion = requestedVersion;
                statusBox.textContent = 'Loaded chart defaults.';
            })
            .catch((err) => {
                if (isCurrentModalRequest(scope) && reqId === defaultsReqId) statusBox.textContent = errMsg(err);
            })
            .finally(() => {
                if (!isCurrentModalRequest(scope) || reqId !== defaultsReqId) return;
                defaultsLoading = false;
                previewButton.disabled = false;
                btn.disabled = false;
                btn.textContent = '↓ Load chart defaults';
            });
    });

    nameInput.addEventListener('input', () => invalidatePreview('Release name changed — preview again before installing.'));
    namespaceInput.addEventListener('input', () => invalidatePreview('Namespace changed — preview again before installing.'));

    // Dry-run preview of the manifest this install would create (toggle on/off).
    let previewReqId = 0;
    previewButton.addEventListener('click', () => {
        if (!isCurrentModalRequest(scope) || !valuesEditor) return;
        const pre = diffBox;
        const btn = previewButton;
        if (!pre.hidden) {
            previewReqId++;
            pre.hidden = true;
            btn.textContent = '👁 Preview (dry-run)';
            return;
        }
        const reqId = ++previewReqId;
        approvedPreview = null;
        okButton.disabled = true;
        pre.hidden = false; pre.textContent = 'Rendering (dry-run)…'; btn.textContent = '✕ Hide preview';
        const previewInput = {
            namespace: namespaceInput.value.trim() || 'default',
            name: nameInput.value.trim() || 'preview',
            version: versionSelect.value.trim(),
            values: valuesEditor.getValue(),
        };
        HelmInstallPreview(previewInput.namespace, previewInput.name,
            chart.repoURL, chartName, previewInput.version, previewInput.values)
            .then((diff) => {
                if (!isCurrentModalRequest(scope) || reqId !== previewReqId) return;
                if (!diff.chartDigest) throw new Error('Preview did not return a chart digest.');
                approvedPreview = { ...previewInput, digest: diff.chartDigest };
                okButton.disabled = false;
                statusBox.textContent = `Previewed exact chart ${diff.chartDigest.slice(0, 19)}…`;
                renderDiffInto(pre, diff.current, diff.proposed);
            })
            .catch((err) => {
                if (isCurrentModalRequest(scope) && reqId === previewReqId) {
                    approvedPreview = null;
                    okButton.disabled = true;
                    pre.textContent = errMsg(err);
                }
            });
    });

    // Enrich from Artifact Hub (best-effort): version list, README, home/links.
    ChartDetails(chart.repo, chartName)
        .then((d) => {
            if (!isCurrentModalRequest(scope)) return;
            if (d.versions && d.versions.length > 0) {
                const sel = versionSelect;
                sel.innerHTML = d.versions.map((v) => `<option value="${esc(v)}"${v === chart.version ? ' selected' : ''}>${esc(v)}</option>`).join('');
            }
            const links = [];
            if (d.homeURL) links.push(extLink(d.homeURL, 'home'));
            for (const l of d.links || []) links.push(extLink(l.url, l.name || 'link'));
            const linkBox = linksBox;
            linkBox.innerHTML = `<span class="modal-hint">Repo: </span>${extLink(chart.repoURL)}` +
                (links.length ? ` &nbsp;·&nbsp; <span class="modal-hint">Links: </span>${links.join(' ')}` : '') +
                (d.maintainers && d.maintainers.length ? ` &nbsp;·&nbsp; <span class="modal-hint">By: </span>${esc(d.maintainers.join(', '))}` : '');
            readmeBox.textContent = d.readme || '(no README published)';
        })
        .catch(() => {
            if (isCurrentModalRequest(scope)) readmeBox.textContent = '(chart details unavailable — not on Artifact Hub or offline)';
        });
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
                        RemoveHelmRepo(btn.dataset.name).then(() => refreshCurrentView()).catch((err) => showError(errMsg(err)));
                    });
                }));
        })
        .catch((err) => viewError(scope, err));
}

function openRepoAddModal() {
    openModal({
        title: 'Add Helm repository',
        ownerKey: 'helm-repo-add',
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
            return AddHelmRepo(name, url, $('repo-user').value, $('repo-pass').value).then(() => refreshCurrentView());
        },
    });
}

function openRepoBrowseModal(name) {
    const scope = openModal({
        title: `Browse ${name}`,
        ownerKey: modalOwner('helm-repo-browse', name),
        okText: 'Close',
        bodyHtml: `<div class="chart-search-row">
                <input id="repo-filter" class="pf-input no-enter-submit" style="flex:1" type="text" placeholder="Filter charts…" autocomplete="off">
            </div>
            <div id="repo-browse-results" class="chart-results"><p class="empty-inline">Loading…</p></div>`,
        onOk: () => Promise.resolve(),
    });
    const filterInput = $('repo-filter');
    const resultsBox = $('repo-browse-results');
    let all = [];
    const render = () => {
        if (!isCurrentModalRequest(scope)) return;
        const q = filterInput.value.trim().toLowerCase();
        const filtered = q ? all.filter((c) => c.name.toLowerCase().includes(q)) : all;
        renderChartResults(filtered, resultsBox);
    };
    BrowseHelmRepo(name)
        .then((charts) => {
            if (!isCurrentModalRequest(scope)) return;
            all = charts || [];
            render();
        })
        .catch((err) => {
            if (isCurrentModalRequest(scope)) resultsBox.innerHTML = `<p class="error">${esc(errMsg(err))}</p>`;
        });
    filterInput.addEventListener('input', render);
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
const LIVE_REFRESH_MS = 5000;
const LIVE_SIDEBAR_REFRESH_MS = 30000;

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
            if (Date.now() - navCountsRefreshedAt >= LIVE_SIDEBAR_REFRESH_MS) loadSidebarCounts();
        }, LIVE_REFRESH_MS);
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
    let configuredProvider = '';
    let hasStoredKey = false;
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
            if (provider !== 'ollama' && !$('ai-key').value.trim()
                && !(hasStoredKey && provider === configuredProvider)) {
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
        configuredProvider = cfg.provider || '';
        hasStoredKey = !!cfg.hasApiKey;
        $('ai-provider').value = cfg.provider || '';
        $('ai-endpoint').value = cfg.endpoint || '';
        $('ai-key').value = '';
        $('ai-key').placeholder = hasStoredKey
            ? 'saved key is hidden — leave blank to keep it'
            : 'stored locally, never shown again';
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
