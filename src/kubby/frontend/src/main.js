import './style.css';
import './app.css';
import './option-b.css';
import './responsive.css';
import './incident.css';
import '@fontsource-variable/inter/wght.css';
import '@xterm/xterm/css/xterm.css';

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
import { createFrameScheduler, IncrementalLogView, LineRingBuffer } from './log-buffer.js';
import { lineDiff } from './line-diff.js';
import { confirmedAction, summarizeLineChanges } from './confirmed-action.js';
import { overviewHealthModel } from './overview-health.js';
import { createKeyedRequestOwner } from './keyed-request.js';
import {
    allTableRows,
    appendVirtualRows,
    clearVirtualRows,
    clearVirtualSelections,
    filterVirtualRows,
    setVirtualRows,
    sortVirtualRows,
    updateVirtualPaging,
} from './virtual-table.js';

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
    PodsPage,
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
    ListHorizontalPodAutoscalers,
    ListPodDisruptionBudgets,
    ListNetworkPolicies,
    CheckTrafficPolicy,
    HelmGet,
    HelmSnapshot,
    HelmHistory,
    HelmRollbackOwned,
    HelmUninstallOwned,
    HelmUpgradeValuesOwned,
    HelmInstallOwned,
    SearchCharts,
    ChartDetails,
    ChartDefaultValues,
    HelmInstallPreview,
    HelmUpgradePreview,
    HelmGetRevision,
    HelmTestOwned,
    ListHelmRepos,
    AddHelmRepo,
    RemoveHelmRepo,
    UpdateHelmRepos,
    BrowseHelmRepo,
    GetYAML,
    UpdateYAML,
    ApplyYAMLOwned,
    ApplyPreview,
    PlanApplyPermissions,
    PlanDrainPermissions,
    DrainImpact,
    PlanHelmPermissions,
    CanI,
    Sizing,
    ClusterChecks,
    ClusterHygiene,
    ExplainPodScheduling,
    CancelViewReads,
    GetDetail,
    GetDrawerSnapshotOwned,
    CancelDrawerSnapshot,
    ListEvents,
	DeleteResourceOwned,
	ScaleDeploymentOwned,
    RestartDeploymentOwned,
    RestartStatefulSetOwned,
    RestartDaemonSetOwned,
    SearchResources,
    SidebarCounts,
    CustomKinds,
    ListCustomPage,
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
    SetDeploymentPausedOwned,
	RolloutHistory,
	RollbackDeploymentOwned,
    SetNodeSchedulableOwned,
    DrainNodeOwned,
    RunCronJobNowOwned,
    SecretData,
    RecentConnections,
    ForgetConnection,
    OverviewSnapshot,
    InvestigateResource,
    SaveIncidentReport,
    PodContainerStates,
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
import { EventsOn, BrowserOpenURL, ClipboardGetText } from '../wailsjs/runtime/runtime';

const PAGE_TITLES = {
    overview: 'Overview',
    structure: 'Topology',
    nodes: 'Nodes',
    namespaces: 'Namespaces',
    sizing: 'Right-sizing',
    checks: 'Health checks',
    hygiene: 'Cleanup',
    pods: 'Pods',
    deployments: 'Deployments',
    services: 'Services',
    traffic: 'Topology',
    configmaps: 'ConfigMaps',
    secrets: 'Secrets',
    statefulsets: 'StatefulSets',
    daemonsets: 'DaemonSets',
    jobs: 'Jobs',
    cronjobs: 'CronJobs',
    ingresses: 'Ingresses',
    networkpolicies: 'NetworkPolicies',
    hpas: 'HorizontalPodAutoscalers',
    pdbs: 'PodDisruptionBudgets',
    pvcs: 'PersistentVolumeClaims',
    serviceaccounts: 'ServiceAccounts',
    pvs: 'PersistentVolumes',
    storageclasses: 'StorageClasses',
    roles: 'Roles',
    rolebindings: 'RoleBindings',
    clusterroles: 'ClusterRoles',
    clusterrolebindings: 'ClusterRoleBindings',
    crds: 'CustomResourceDefinitions',
    helm: 'Helm',
    resourcequotas: 'ResourceQuotas',
    limitranges: 'LimitRanges',
};

const PAGE_SUBTITLES = {
    overview: 'Live health and capacity across the connected cluster.',
    structure: 'Trace dependencies from entry points through services and workloads to pods.',
    nodes: 'Inspect cluster machines, readiness, versions, and scheduled workloads.',
    namespaces: 'Browse logical scopes and the resources running inside them.',
    sizing: 'Compare requested resources with live usage and find waste or risk.',
    checks: 'Find broken admission webhooks, expiring certificates and deletions stuck on finalizers.',
    hygiene: 'Find objects nothing references any more, and images that are not pinned.',
    pods: 'Monitor workload health, resource usage, logs, terminals, and events.',
    deployments: 'Review rollout health and safely scale, restart, pause, or roll back.',
    services: 'Inspect stable network endpoints and the workloads behind them.',
    traffic: 'Follow ingress and service routes to the pods that actually receive traffic.',
    configmaps: 'Browse application configuration stored in the cluster.',
    secrets: 'Inspect secret metadata and reveal values only when explicitly requested.',
    statefulsets: 'Monitor ordered, stateful workloads and rolling restarts.',
    daemonsets: 'Review node-wide workloads and their rollout health.',
    jobs: 'Track one-time workloads and completion status.',
    cronjobs: 'Inspect schedules, suspension state, and trigger jobs safely.',
    ingresses: 'Review external routes, hosts, and ingress configuration.',
    networkpolicies: 'See which Pods are isolated and what traffic each policy admits.',
    hpas: 'Check that autoscalers can read metrics and scale their targets.',
    pdbs: 'Find budgets that block evictions before a drain or upgrade stalls.',
    pvcs: 'Inspect namespaced storage claims and their binding state.',
    serviceaccounts: 'Review workload identities in the selected scope.',
    pvs: 'Inspect cluster-wide volumes, claims, and storage classes.',
    storageclasses: 'Review dynamic provisioning and reclaim policies.',
    roles: 'Inspect namespaced access rules.',
    rolebindings: 'See which subjects receive namespaced permissions.',
    clusterroles: 'Inspect cluster-wide access rules.',
    clusterrolebindings: 'See which subjects receive cluster-wide permissions.',
    crds: 'Browse the custom APIs installed in this cluster.',
    helm: 'Manage releases, discover charts, and configure repositories in one workspace.',
    resourcequotas: 'Review namespace resource limits and current usage.',
    limitranges: 'Inspect default and enforced container resource policies.',
};

const NAMESPACED_VIEWS = new Set([
    'structure',
    'pods', 'deployments', 'services', 'configmaps', 'secrets',
    'statefulsets', 'daemonsets', 'jobs', 'cronjobs', 'ingresses', 'pvcs', 'serviceaccounts',
    'networkpolicies', 'hpas', 'pdbs',
    'roles', 'rolebindings', 'helm', 'resourcequotas', 'limitranges',
    'sizing', 'checks', 'hygiene',
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
    networkpolicies: 'NetworkPolicy',
    hpas: 'HorizontalPodAutoscaler',
    pdbs: 'PodDisruptionBudget',
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
let welcomeConnectPending = false;
let welcomeContextRequestID = 0;
let currentView = 'overview';
let currentNamespace = '';
let helmSection = 'releases';
let renderedViewOwner = '';
const requestScopes = createRequestScopes();
const pasteEditor = createYamlEditor($('paste-area'), {
	placeholder: 'apiVersion: v1\nkind: Config\nclusters:\n- ...',
});

function isCurrentViewRequest(scope) {
    return requestScopes.isCurrentView(scope, currentView, currentNamespace);
}

function viewError(scope, err) {
    if (!isCurrentViewRequest(scope)) return;
    setViewStatus('error', 'Could not refresh this view');
    showDashError(err);
}

function connectionOwnershipChanged() {
	if (!$('modal').hidden) closeModal();
	if (!$('dialog').hidden) closeDialog(false);
	if (!$('palette').hidden) closePalette({ restoreFocus: false });
	requestScopes.connectionChanged();
	clearCustomSections();
}

// ============ Welcome screen ============

document.querySelectorAll('.tab').forEach((tab) => {
    tab.addEventListener('click', () => {
        // A late file/content parse must not repopulate the context selector
        // after the user has deliberately moved to the other source.
        welcomeContextRequestID++;
        document.querySelectorAll('.tab').forEach((t) => t.classList.toggle('active', t === tab));
        $('tab-file').hidden = tab.dataset.tab !== 'file';
        $('tab-paste').hidden = tab.dataset.tab !== 'paste';
    });
});

$('btn-pick-kubeconfig').addEventListener('click', () => {
    clearWelcomeError();
    const requestID = ++welcomeContextRequestID;
    PickKubeconfigFile()
        .then((path) => {
            if (requestID !== welcomeContextRequestID || !path) return null;
            source = { mode: 'path', path, content: '' };
            const picked = $('picked-path');
            picked.textContent = path;
            picked.hidden = false;
            return ContextsFromPath(path).then((res) => fillContexts(res, requestID));
        })
        .catch((err) => { if (requestID === welcomeContextRequestID) showWelcomeError(err); });
});

$('btn-load-paste').addEventListener('click', () => {
    clearWelcomeError();
	const content = pasteEditor.getValue().trim();
	if (!content) {
        showWelcomeError('Paste your kubeconfig content first.');
        return;
	}
    const requestID = ++welcomeContextRequestID;
    source = { mode: 'content', path: '', content };
    ContextsFromContent(content)
        .then((res) => fillContexts(res, requestID))
        .catch((err) => { if (requestID === welcomeContextRequestID) showWelcomeError(err); });
});

function fillContexts(res, requestID = welcomeContextRequestID) {
    if (requestID !== welcomeContextRequestID) return;
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
    setWelcomeStep(2);
}

$('btn-connect').addEventListener('click', () => {
	if (welcomeConnectPending) return;
	clearWelcomeError();
    const ctx = $('context-select').value;
    const btn = $('btn-connect');
	btn.disabled = true;
	btn.textContent = 'Connecting…';
	setWelcomeConnectPending(true);
    setWelcomeStep(3);
    $('connecting-overlay').hidden = false;

    const connect = source.mode === 'path'
        ? ConnectWithPath(source.path, ctx)
        : ConnectWithContent(source.content, ctx);

    connect
		.then(() => {
			if (source.mode === 'content') {
				source.content = '';
				pasteEditor.setValue('');
			}
			return enterDashboard(ctx);
		})
        .catch(showWelcomeError)
		.finally(() => {
            $('connecting-overlay').hidden = true;
            btn.disabled = false;
			btn.textContent = 'Connect';
			setWelcomeConnectPending(false);
			if (!$('welcome').hidden) setWelcomeStep(2);
		});
});

function setWelcomeStep(step) {
    document.querySelectorAll('.welcome-steps li').forEach((item, index) => {
        item.classList.toggle('active', index + 1 === step);
        item.classList.toggle('completed', index + 1 < step);
    });
}

function setWelcomeConnectPending(pending) {
	welcomeConnectPending = pending;
	for (const item of document.querySelectorAll('.recent-item')) {
		item.classList.toggle('recent-item-disabled', pending);
		item.setAttribute('aria-disabled', String(pending));
	}
}

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

$('btn-command-palette').addEventListener('click', openPalette);
$('btn-brand-home').addEventListener('click', () => selectView('overview'));

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
    const defaults = ['network', 'config', 'storage', 'access', 'ecosystem', 'custom'];
    let collapsed = defaults;
    try {
        const saved = localStorage.getItem(NAV_COLLAPSE_KEY);
        if (saved !== null) collapsed = JSON.parse(saved);
    } catch { /* keep the calm first-run defaults */ }
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
    const navView = view === 'structure' ? 'traffic' : view;
    const item = document.querySelector(`.nav-item[data-view="${navView}"]`);
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
let namespaceActiveIndex = 0;

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

function visibleNamespaceOptions() {
    return [...$('namespace-options').querySelectorAll('.namespace-option')];
}

function setActiveNamespaceOption(index) {
    const options = visibleNamespaceOptions();
    if (options.length === 0) {
        namespaceActiveIndex = 0;
        $('namespace-search').removeAttribute('aria-activedescendant');
        return;
    }
    namespaceActiveIndex = Math.max(0, Math.min(index, options.length - 1));
    options.forEach((option, i) => option.classList.toggle('active', i === namespaceActiveIndex));
    const active = options[namespaceActiveIndex];
    $('namespace-search').setAttribute('aria-activedescendant', active.id);
    active.scrollIntoView({ block: 'nearest' });
}

function selectNamespace(value) {
    const select = $('namespace-select');
    if (![...select.options].some((option) => option.value === value)) return;
    if (select.value === value) {
        syncNamespacePicker();
        closeNamespacePicker({ restoreFocus: true });
        return;
    }
    select.value = value;
    select.dispatchEvent(new Event('change', { bubbles: true }));
}

function renderNamespacePicker(query = '') {
    const filter = query.trim().toLocaleLowerCase();
    const selected = $('namespace-select').value;
    const matches = [...$('namespace-select').options].filter((option) => {
        const label = option.value === '' ? 'All namespaces' : option.textContent;
        return !filter || label.toLocaleLowerCase().includes(filter);
    });
    const list = $('namespace-options');
    list.innerHTML = '';
    matches.forEach((option, index) => {
        const button = document.createElement('button');
        button.type = 'button';
        button.id = `namespace-option-${index}`;
        button.className = 'namespace-option';
        button.dataset.value = option.value;
        button.setAttribute('role', 'option');
        button.tabIndex = -1;
        button.setAttribute('aria-selected', String(option.value === selected));
        const label = option.value === '' ? 'All namespaces' : option.textContent;
        const hint = option.value === '' ? 'Cluster-wide scope' : 'Namespace';
        button.innerHTML = `<span><strong>${esc(label)}</strong><small>${hint}</small></span><span class="namespace-option-check" aria-hidden="true">✓</span>`;
        button.addEventListener('click', () => selectNamespace(option.value));
        list.appendChild(button);
    });
    $('namespace-empty').hidden = matches.length !== 0;
    const selectedIndex = matches.findIndex((option) => option.value === selected);
    setActiveNamespaceOption(selectedIndex >= 0 ? selectedIndex : 0);
}

function syncNamespacePicker() {
    const selected = $('namespace-select').selectedOptions[0];
    const label = selected?.value ? selected.textContent : 'All namespaces';
    $('namespace-current').textContent = label;
    $('namespace-toggle').title = `Namespace: ${label}`;
    renderNamespacePicker($('namespace-search').value);
}

function closeNamespacePicker({ restoreFocus = false } = {}) {
    if ($('namespace-popover').hidden) return;
    $('namespace-popover').hidden = true;
    $('namespace-toggle').setAttribute('aria-expanded', 'false');
    $('namespace-search').setAttribute('aria-expanded', 'false');
    $('namespace-search').removeAttribute('aria-activedescendant');
    if (restoreFocus) $('namespace-toggle').focus();
}

function openNamespacePicker() {
    $('namespace-popover').hidden = false;
    $('namespace-toggle').setAttribute('aria-expanded', 'true');
    $('namespace-search').setAttribute('aria-expanded', 'true');
    $('namespace-search').value = '';
    renderNamespacePicker();
    // Show what is known immediately, then pick up namespaces created since.
    loadNamespaceOptions();
    requestAnimationFrame(() => {
        $('namespace-search').focus();
        $('namespace-search').select();
    });
}

$('namespace-toggle').addEventListener('click', () => {
    if ($('namespace-popover').hidden) openNamespacePicker();
    else closeNamespacePicker();
});
$('namespace-toggle').addEventListener('keydown', (event) => {
    if (event.key === 'ArrowDown') {
        event.preventDefault();
        openNamespacePicker();
    }
});
$('namespace-search').addEventListener('input', (event) => renderNamespacePicker(event.target.value));
$('namespace-search').addEventListener('keydown', (event) => {
    const options = visibleNamespaceOptions();
    if (event.key === 'ArrowDown') {
        event.preventDefault();
        setActiveNamespaceOption(namespaceActiveIndex + 1);
    } else if (event.key === 'ArrowUp') {
        event.preventDefault();
        setActiveNamespaceOption(namespaceActiveIndex - 1);
    } else if (event.key === 'Home') {
        event.preventDefault();
        setActiveNamespaceOption(0);
    } else if (event.key === 'End') {
        event.preventDefault();
        setActiveNamespaceOption(options.length - 1);
    } else if (event.key === 'Enter' && options[namespaceActiveIndex]) {
        event.preventDefault();
        options[namespaceActiveIndex].click();
    } else if (event.key === 'Escape') {
        event.preventDefault();
        event.stopPropagation();
        closeNamespacePicker({ restoreFocus: true });
    }
});
document.addEventListener('pointerdown', (event) => {
    if (!event.target.closest('#namespace-picker')) closeNamespacePicker();
});
$('namespace-picker').addEventListener('focusout', (event) => {
    if (!event.relatedTarget || !$('namespace-picker').contains(event.relatedTarget)) closeNamespacePicker();
});

$('namespace-select').addEventListener('change', (e) => {
    currentNamespace = e.target.value;
    syncNamespacePicker();
    closeNamespacePicker();
    updateNsScope();
    loadSidebarCounts({ includeCluster: false });
    refreshCurrentView();
});

$('btn-refresh').addEventListener('click', () => { loadSidebarCounts(); loadNamespaceOptions(); refreshCurrentView(); });
$('btn-cluster-structure').addEventListener('click', () => selectView('structure'));
$('topology-dependencies').addEventListener('click', () => selectView('structure'));
$('topology-traffic').addEventListener('click', () => selectView('traffic'));
document.querySelectorAll('#topology-switcher [role="tab"]').forEach((tab) => {
    tab.addEventListener('keydown', (event) => {
        if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return;
        event.preventDefault();
        const target = event.key === 'ArrowLeft' || event.key === 'Home' ? 'structure' : 'traffic';
        selectView(target);
        requestAnimationFrame(() => $(target === 'structure' ? 'topology-dependencies' : 'topology-traffic').focus());
    });
});
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
const CUSTOM_VIEW_KEYS = new Set();

function clearCustomSections() {
	for (const view of CUSTOM_VIEW_KEYS) {
		delete PAGE_TITLES[view];
		NAMESPACED_VIEWS.delete(view);
	}
	CUSTOM_VIEW_KEYS.clear();
	CUSTOM_KINDS.clear();
}

function loadCustomSections() {
    const box = $('nav-custom-items');
    const section = $('nav-section-custom');
    const connection = requestScopes.connectionToken();
    return CustomKinds()
        .then((res) => {
            if (!requestScopes.isCurrentConnection(connection)) return;
            const kinds = res?.kinds ?? [];
			clearCustomSections();
            box.innerHTML = '';
            section.hidden = kinds.length === 0;
            if (kinds.length === 0) return;

            for (const k of kinds) {
                const view = `custom:${k.refKind}`;
				CUSTOM_KINDS.set(view, k);
				CUSTOM_VIEW_KEYS.add(view);
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
    customPageState = { scope, meta, token: '', loading: false, loaded: 0 };
    return loadNextCustomPage(true);
}

let customPageState = null;

function loadNextCustomPage(reset = false) {
    const state = customPageState;
    if (!state || state.loading || !isCurrentViewRequest(state.scope)) return Promise.resolve();
    if (!reset && !state.token) return Promise.resolve();
    state.loading = true;
    $('custom-load-more').disabled = true;
    $('custom-page-status').textContent = reset ? 'Loading first page…' : `Loading after ${state.loaded}…`;
    return ListCustomPage(state.meta.refKind, state.meta.namespaced ? state.scope.namespace : '', state.token, RESOURCE_PAGE_LIMIT)
        .then((page) => {
            if (state !== customPageState || !isCurrentViewRequest(state.scope)) return;
            const items = page?.items ?? [];
            const body = $('custom-body');
            const table = body.closest('table');
            const rows = items.map((it) => {
                const tr = row(
                    `<td>${esc(it.namespace)}</td><td>${esc(it.name)}</td>`
                    + `<td>${it.status ? badge(it.status, !it.isError) : '<span class="dim">—</span>'}</td>`
                    + `<td>${esc(it.age)}</td>`,
                    { isError: !!it.isError, ref: { kind: state.meta.refKind, namespace: it.namespace, name: it.name } },
                );
                padRowToHeader(tr, table);
                return tr;
            });
            state.loaded += rows.length;
            state.token = page?.page?.continue ?? '';
            const options = { hasMore: !!state.token, onNearEnd: () => loadNextCustomPage(false) };
            if (reset) setVirtualRows(body, rows, options);
            else appendVirtualRows(body, rows, options);
            updatePageControls('custom', state.loaded, page?.page);
            $('custom-empty').hidden = state.loaded > 0;
            filterCurrentTable();
        })
        .catch((err) => viewError(state.scope, err))
        .finally(() => {
            if (state !== customPageState) return;
            state.loading = false;
            $('custom-load-more').disabled = false;
        });
}

$('custom-load-more').addEventListener('click', () => loadNextCustomPage(false));

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
    const navView = view === 'structure' ? 'traffic' : view;
    document.querySelectorAll('.nav-item').forEach((b) => {
        const active = b.dataset.view === navView;
        b.classList.toggle('active', active);
        if (active) b.setAttribute('aria-current', 'page');
        else b.removeAttribute('aria-current');
    });
    document.querySelectorAll('.view').forEach((v) => (v.hidden = v.id !== sectionId));
    $('page-title').textContent = PAGE_TITLES[view] ?? view;
    $('page-subtitle').textContent = PAGE_SUBTITLES[view]
        ?? (String(view).startsWith('custom:') ? 'Browse this custom API and inspect its live resources.' : 'Browse and manage live cluster resources.');
    const navItem = document.querySelector(`.nav-item[data-view="${navView}"]`);
    $('page-eyebrow').textContent = navItem?.closest('.nav-section')?.querySelector('.nav-group span')?.textContent ?? 'Workspace';
    const structureButton = $('btn-cluster-structure');
    structureButton.hidden = view !== 'overview';
    const topology = view === 'structure' || view === 'traffic';
    $('topology-switcher').hidden = !topology;
    $('topology-dependencies').setAttribute('aria-selected', String(view === 'structure'));
    $('topology-traffic').setAttribute('aria-selected', String(view === 'traffic'));
    $('topology-dependencies').tabIndex = view === 'structure' ? 0 : -1;
    $('topology-traffic').tabIndex = view === 'traffic' ? 0 : -1;
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
    const hasTable = view !== 'overview' && view !== 'structure' && view !== 'traffic' && view !== 'sizing' && view !== 'helm' && view !== 'checks' && view !== 'hygiene';
    $('view-search').hidden = !hasTable;
    $('view-filter').value = '';
    updateNsScope();
    refreshCurrentView();
}

function updateNsScope() {
    const scopeEl = $('ns-scope');
    scopeEl.textContent = currentView === 'helm' && helmSection === 'repositories'
        ? 'Local machine'
        : NAMESPACED_VIEWS.has(currentView)
        ? (currentNamespace === '' ? 'All namespaces' : `Namespace: ${currentNamespace}`)
        : '';
}

// viewLoadsInFlight counts the view loads that have not settled. It is what
// decides whether the previous screen still has reads worth abandoning.
let viewLoadsInFlight = 0;

function refreshCurrentView() {
    clearDashError();
    clearSelection();
    const scope = requestScopes.beginView(currentView, currentNamespace);
    const owner = `${$('cluster-select').value}\0${scope.view}\0${scope.namespace}`;
    const refreshing = owner === renderedViewOwner;
    // A same-scope refresh keeps the last trustworthy result visible. A scope
    // transition still clears it immediately so old-cluster rows cannot be used.
    if (!refreshing) clearRenderedView(scope.view);
    setViewStatus(refreshing ? 'refreshing' : 'loading', refreshing ? 'Refreshing…' : 'Loading…');
    // The epochs above already stop a superseded response from rendering, but
    // the request itself kept running: a cluster-wide list can hold the
    // connection for seconds after the user moved on, delaying the screen they
    // did ask for. Abandoning it first is the difference the user feels.
    // Awaited, so the reads started below are never the ones cancelled.
    const abandoned = viewLoadsInFlight > 0
        ? Promise.resolve(CancelViewReads()).catch(() => 0)
        : Promise.resolve(0);
    viewLoadsInFlight++;
    const p = abandoned.then(() => doRefresh(scope));
    p.finally(() => {
        viewLoadsInFlight--;
        if (!isCurrentViewRequest(scope)) return;
        filterCurrentTable();
        if ($('view-status').dataset.state !== 'error') {
            renderedViewOwner = owner;
            setViewStatus('current', 'Updated just now');
        }
    });
    return p;
}

function setViewStatus(state, message) {
    const status = $('view-status');
    status.dataset.state = state;
    status.querySelector('span').textContent = message;
    status.hidden = false;
}

function clearRenderedView(view = currentView) {
    document.querySelectorAll(`#${viewSectionId(view)} tbody`).forEach((body) => clearVirtualRows(body));
    // These dashboard canvases contain actionable buttons rather than table
    // rows. Remove the previous owner immediately so a slow cluster/namespace
    // response cannot leave a clickable topology from the old scope.
	if (view === 'overview') {
		$('node-status').innerHTML = '';
		$('overview-toppods-body').innerHTML = '';
		$('node-status-total').textContent = '';
		$('overview-warnings').hidden = true;
	}
    if (view === 'structure') {
        for (const id of ['structure-summary', 'structure-entries', 'structure-internal', 'structure-unexposed']) $(id).innerHTML = '';
        $('structure-warnings').hidden = true;
        $('structure-updated').textContent = '';
        resetStructureInspector();
    }
	if (view === 'traffic') $('traffic-warnings').hidden = true;
    // The check and cleanup cards carry buttons that open an object by
    // namespace and name. Left on screen during a scope change they would open
    // the previous scope's object, so they go with the scope.
    if (view === 'checks') {
        checksReport = null;
        $('checks-summary').innerHTML = '';
        for (const tab of Object.keys(CHECK_TABS)) $(`checks-${tab}-list`).innerHTML = '';
        $('checks-warnings').hidden = true;
        $('checks-updated').textContent = '';
    }
    if (view === 'hygiene') {
        hygieneReport = null;
        $('hygiene-summary').innerHTML = '';
        $('hygiene-groups').innerHTML = '';
        $('hygiene-warnings').hidden = true;
        $('hygiene-updated').textContent = '';
        $('hygiene-empty').hidden = true;
    }
}

// Instant client-side filter over the current view's table rows.
$('view-filter').addEventListener('input', filterCurrentTable);

function filterCurrentTable() {
    if (currentView === 'overview' || currentView === 'structure') { $('view-count').textContent = ''; return; }
    const body = document.querySelector(`#${viewSectionId(currentView)} tbody`);
    if (!body) { $('view-count').textContent = ''; return; }
    const term = $('view-filter').value.trim().toLowerCase();
    const rows = allTableRows(body);
    const matchRow = (tr) => {
        const searchText = tr.dataset.searchText ??= tr.textContent.toLowerCase();
        return !term || searchText.includes(term);
    };
    const virtual = filterVirtualRows(body, term ? matchRow : null);
    if (virtual) {
        $('view-count').textContent = term ? `${virtual.shown} / ${virtual.total}` : `${virtual.total}`;
        return;
    }
    let shown = 0;
    for (const tr of rows) {
        // Reading textContent walks every descendant. Cache the normalized row
        // text after its first filter pass so later keystrokes stay a string scan.
        const searchText = tr.dataset.searchText ??= tr.textContent.toLowerCase();
        const match = !term || searchText.includes(term);
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
        case 'checks': return loadChecks(scope);
        case 'hygiene': return loadHygiene(scope);
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
        case 'networkpolicies': return loadSimple(scope, ListNetworkPolicies, 'networkpolicies', 'NetworkPolicy',
            (n) => `<td>${esc(n.namespace)}</td><td>${esc(n.name)}</td><td class="mono">${esc(n.podSelector)}</td><td>${esc(n.policyTypes)}</td><td>${esc(n.effect)}</td><td>${esc(n.age)}</td>`);
        case 'hpas': return loadSimple(scope, ListHorizontalPodAutoscalers, 'hpas', 'HorizontalPodAutoscaler',
            (h) => `<td>${esc(h.namespace)}</td><td>${esc(h.name)}</td><td class="mono">${esc(h.target)}</td><td>${esc(h.replicas)}</td><td>${esc(h.minMax)}</td><td class="mono">${h.metrics ? esc(h.metrics) : '<span class="dim">—</span>'}</td><td>${badge(h.status, !h.isError)}</td><td>${esc(h.age)}</td>`);
        case 'pdbs': return loadSimple(scope, ListPodDisruptionBudgets, 'pdbs', 'PodDisruptionBudget',
            (p) => `<td>${esc(p.namespace)}</td><td>${esc(p.name)}</td><td class="mono">${esc(p.budget)}</td><td>${p.allowedDisruptions}</td><td>${esc(p.healthy)}</td><td>${badge(p.status, !p.isError)}</td><td>${esc(p.age)}</td>`);
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
        case 'traffic': return loadTraffic(scope);
    }
}

function loadHelm(scope) {
    renderHelmSection();
    if (helmSection === 'catalog') return loadHelmCatalogSources(scope);
    if (helmSection === 'repositories') return loadHelmRepos(scope);
    return loadHelmReleases(scope);
}

function setHelmSection(section, { refresh = true } = {}) {
    if (!['releases', 'catalog', 'repositories'].includes(section)) return;
    helmSection = section;
    renderHelmSection();
    updateNsScope();
    if (refresh && currentView === 'helm') refreshCurrentView();
}

function renderHelmSection() {
    document.querySelectorAll('.helm-workspace-tab').forEach((tab) => {
        const active = tab.dataset.helmSection === helmSection;
        tab.classList.toggle('active', active);
        tab.setAttribute('aria-selected', String(active));
        tab.tabIndex = active ? 0 : -1;
    });
    document.querySelectorAll('[data-helm-panel]').forEach((panel) => {
        panel.hidden = panel.dataset.helmPanel !== helmSection;
    });
}

function helmStatusBadge(release) {
    const status = String(release.status || 'unknown').toLowerCase();
    const tone = release.isError || status === 'failed' ? 'bad'
        : release.isPending || status.startsWith('pending-') || status === 'uninstalling' ? 'warn'
        : status === 'deployed' ? 'ok' : 'neutral';
    return `<span class="helm-status helm-status-${tone}"><i></i>${esc(status)}</span>`;
}

// The backend returns one latest revision per release; history belongs inside
// release detail rather than appearing as duplicate rows here.
function loadHelmReleases(scope) {
    $('helm-loading').hidden = false;
    $('helm-empty').hidden = true;
    return ListHelmReleases(scope.namespace)
        .then((rels) => {
            if (!isCurrentViewRequest(scope)) return;
            const body = $('helm-body');
            body.innerHTML = '';
            $('helm-loading').hidden = true;
            const releases = rels ?? [];
            $('helm-empty').hidden = releases.length > 0;
            $('helm-total').textContent = String(releases.length);
            $('helm-deployed').textContent = String(releases.filter((r) => String(r.status).toLowerCase() === 'deployed').length);
            $('helm-pending').textContent = String(releases.filter((r) => r.isPending).length);
            $('helm-failed').textContent = String(releases.filter((r) => r.isError).length);
            for (const r of releases) {
                const ref = { kind: 'HelmRelease', namespace: r.namespace, name: r.name, secretName: r.secretName };
                const tr = document.createElement('tr');
                tr.className = 'clickable helm-release-row';
                if (r.isError) tr.classList.add('error-row');
                tr.dataset.filter = `${r.name} ${r.namespace} ${r.revision} ${r.status}`.toLowerCase();
                tr.innerHTML = `<td><button class="helm-release-link" type="button">${esc(r.name)}</button></td>
                    <td>${esc(r.namespace)}</td><td class="mono">${esc(r.revision)}</td><td>${helmStatusBadge(r)}</td>
                    <td>${esc(r.updated)}</td><td class="col-actions"><button class="btn btn-secondary btn-sm helm-open-release" type="button">Open</button>${actionsBtn(ref)}</td>`;
                tr.addEventListener('click', (event) => {
                    if (!event.target.closest('button')) openHelmDetailModal(ref);
                });
                tr.querySelector('.helm-release-link').addEventListener('click', () => openHelmDetailModal(ref));
                tr.querySelector('.helm-open-release').addEventListener('click', () => openHelmDetailModal(ref));
                wireRowActions(tr, ref);
                body.appendChild(tr);
            }
            filterHelmReleases();
        })
        .catch((err) => {
            if (isCurrentViewRequest(scope)) $('helm-loading').hidden = true;
            viewError(scope, err);
        });
}

function filterHelmReleases() {
    const term = $('helm-release-filter').value.trim().toLowerCase();
    const rows = [...$('helm-body').querySelectorAll('tr')];
    let shown = 0;
    for (const tr of rows) {
        const match = !term || tr.dataset.filter.includes(term);
        tr.hidden = !match;
        if (match) shown++;
    }
    $('helm-release-count').textContent = term ? `${shown} / ${rows.length}` : String(rows.length);
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
            $(`${viewId}-empty`).hidden = (items?.length ?? 0) > 0;
            const rows = (items ?? []).map((it) => {
                const tr = row(cellsFn(it), {
                    isError: !!it.isError,
                    ref: { kind, namespace: it.namespace ?? '', name: it.name },
                });
                padRowToHeader(tr, table);
                return tr;
            });
            setVirtualRows(body, rows);
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

// The picker used to be filled only at connect, so a namespace created from
// kubectl, a Helm install or a controller stayed invisible until reconnect.
// It now also reloads on Refresh and whenever the picker opens; concurrent
// callers share one request, and an unchanged list is not re-rendered, so the
// picker never jumps under the user's keyboard for nothing.
let namespaceOptionsRequest = null;

function loadNamespaceOptions(connection = requestScopes.connectionToken()) {
    if (namespaceOptionsRequest?.connection === connection) return namespaceOptionsRequest.promise;
    const promise = ListNamespaces()
        .then((namespaces) => {
            if (!requestScopes.isCurrentConnection(connection)) return;
            const select = $('namespace-select');
            const names = (namespaces ?? []).map((ns) => ns.name);
            const current = [...select.options].slice(1).map((option) => option.value);
            const unchanged = select.options.length > 0 && names.length === current.length && names.every((name, i) => name === current[i]);
            if (unchanged && select.value === currentNamespace) return;
            const wanted = currentNamespace;
            select.innerHTML = '<option value="">All namespaces</option>';
            for (const ns of namespaces ?? []) {
                const opt = document.createElement('option');
                opt.value = ns.name;
                opt.textContent = ns.name;
                select.appendChild(opt);
            }
            if ([...select.options].some((option) => option.value === wanted)) select.value = wanted;
            else currentNamespace = '';
            syncNamespacePicker();
        })
        .catch((err) => { if (requestScopes.isCurrentConnection(connection)) showDashError(err); })
        .finally(() => { if (namespaceOptionsRequest?.promise === promise) namespaceOptionsRequest = null; });
    namespaceOptionsRequest = { connection, promise };
    return promise;
}

// Build a table row; if `ref` is given the row is clickable and opens the drawer.
// Unless opts.actions === false, a leading checkbox cell and trailing ⋯ actions
// cell are added automatically (headers get matching columns via JS).
function row(cellsHtml, opts = {}) {
    const tr = document.createElement('tr');
    tr.innerHTML = cellsHtml;
    const withActions = !!opts.ref && opts.actions !== false;
    if (withActions) {
        const resourceLabel = `${opts.ref.kind} ${opts.ref.namespace ? `${opts.ref.namespace}/` : ''}${opts.ref.name}`;
        tr.insertAdjacentHTML('afterbegin', `<td class="col-check"><input type="checkbox" class="row-check" aria-label="Select ${esc(resourceLabel)}"></td>`);
        tr.insertAdjacentHTML('beforeend', `<td class="col-actions">${actionsBtn(opts.ref)}</td>`);
    }
    if (opts.isError) tr.classList.add('error-row');
    if (opts.ref) {
        tr.classList.add('clickable');
        tr.tabIndex = 0;
        tr.setAttribute('aria-label', `Open ${opts.ref.kind} ${opts.ref.namespace ? `${opts.ref.namespace}/` : ''}${opts.ref.name}`);
        const open = (event) => {
            if (event.target.closest('button, input, a, select, textarea, .col-check')) return;
            openDrawer(opts.ref);
        };
        tr.addEventListener('click', open);
        tr.addEventListener('keydown', (event) => {
            if ((event.key === 'Enter' || event.key === ' ') && event.target === tr) {
                event.preventDefault();
                openDrawer(opts.ref);
            }
        });
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
            const sectionErrors = snapshot?.sectionErrors ?? {};
            const warning = $('overview-warnings');
            const failedSections = Object.keys(sectionErrors);
            warning.hidden = failedSections.length === 0;
            warning.textContent = failedSections.length
                ? `Partial snapshot — unavailable: ${failedSections.join(', ')}. Successful sections remain live.` : '';
            $('stat-nodes').textContent = sectionErrors.nodes ? '—' : (stats.nodes ?? 0);
            $('stat-namespaces').textContent = sectionErrors.namespaces || sectionErrors.metadata ? '—' : (stats.namespaces ?? 0);
            $('stat-pods').textContent = sectionErrors.pods ? '—' : (stats.pods ?? 0);
            $('stat-deployments').textContent = sectionErrors.deployments || sectionErrors.metadata ? '—' : (stats.deployments ?? 0);
            $('stat-errors').textContent = sectionErrors.pods ? '—' : errored.length;
            $('attention-count').textContent = sectionErrors.pods ? 'Unavailable' : `${errored.length} active`;
            $('attention-count').classList.toggle('has-issues', errored.length > 0);
            updateClusterHealth(
                stats.podsAvailable ? (stats.pods ?? 0) : null,
                errored,
                sectionErrors.nodes ? null : (snapshot?.nodeStatus ?? []),
            );
            updateWorkloadSummary(stats, errored, sectionErrors);

            const body = $('overview-errors-body');
            body.innerHTML = '';
            setAttentionEmptyState(Boolean(sectionErrors.pods), errored.length > 0);
            for (const p of errored) {
                const ref = { kind: 'Pod', namespace: p.namespace, name: p.name, isPod: true };
                // A Pod with no node yet has nothing to investigate: no logs, no
                // container state, no events beyond the scheduler's. The useful
                // first step is the scheduling answer, so it takes the lead here.
                const unscheduled = p.status === 'Pending' || p.status === 'Unschedulable';
                const tr = row(
                    `<td class="overview-namespace" title="${esc(p.namespace)}">${esc(p.namespace)}</td><td class="overview-resource-name" title="${esc(p.name)}">${esc(p.name)}</td><td>${badge(p.status, false)}</td><td class="overview-count">${p.restarts}</td><td class="overview-issue-actions"><button type="button" class="btn btn-quiet btn-sm issue-inspect">Inspect</button>${
                        unscheduled
                            ? '<button type="button" class="btn btn-soft btn-sm issue-scheduling">Why Pending?</button>'
                            : '<button type="button" class="btn btn-soft btn-sm issue-diagnose">Investigate</button>'}</td>`,
                    { isError: true, actions: false, ref },
                );
                tr.querySelector('.issue-inspect').addEventListener('click', () => openDrawer({ ...ref, tab: 'details' }));
                tr.querySelector('.issue-diagnose')?.addEventListener('click', () => openDrawer({ ...ref, tab: 'investigate' }));
                tr.querySelector('.issue-scheduling')?.addEventListener('click', () => openSchedulingModal(ref));
                body.appendChild(tr);
            }

            renderCapacityMetrics(snapshot?.nodeMetrics ?? []);
            renderNodeStatus(snapshot?.nodeStatus ?? []);
            renderTopPods(snapshot?.topPods ?? []);
            renderRecentEvents(snapshot?.events ?? []);
        })
		.catch((err) => {
			if (!isCurrentViewRequest(scope)) return;
			$('overview-warnings').hidden = true;
            updateClusterHealth(null, [], null);
            updateWorkloadSummary({}, [], { pods: String(err) });
            $('attention-count').textContent = 'Unavailable';
            $('attention-count').classList.remove('has-issues');
            setAttentionEmptyState(true, false);
            renderCapacityMetrics([]);
            renderNodeStatus([]);
            renderTopPods([]);
            renderRecentEvents([]);
            showDashError(err);
        });
}

function updateClusterHealth(total, errored, nodes) {
    const banner = $('overview-pulse');
    const icon = $('cluster-health-icon');
    const title = $('cluster-health-title');
    const summary = $('cluster-health-summary');
    const model = overviewHealthModel({ total, errored, nodes });
    banner.className = `overview-health-banner overview-health-${model.tone}`;
    icon.textContent = model.icon;
    title.textContent = model.title;
    summary.textContent = model.summary;
    $('cluster-health-ratio').textContent = model.ratio;
}

function updateWorkloadSummary(stats, errored, sectionErrors) {
    const state = $('workload-state');
    const meta = $('workload-meta');
    if (sectionErrors.pods || sectionErrors.metadata || sectionErrors.deployments) {
        setMetricState(state, 'Unavailable', 'unavailable');
        meta.textContent = 'Workload summary is incomplete';
        return;
    }
    if (errored.length > 0) {
        setMetricState(state, 'Degraded', 'error');
        meta.textContent = errored.length === 1 ? '1 pod is not serving' : `${errored.length} pods are not serving`;
        return;
    }
    setMetricState(state, 'Healthy', 'ok');
    meta.textContent = (stats.pods ?? 0) === 0 ? 'No workloads are running yet' : 'All monitored pods are ready';
}

function setAttentionEmptyState(unavailable, hasErrors) {
    const empty = $('overview-errors-empty');
    empty.hidden = hasErrors;
    empty.classList.toggle('overview-empty-unavailable', unavailable);
    if (hasErrors) return;
    empty.children[0].textContent = unavailable ? '—' : '✓';
    empty.querySelector('strong').textContent = unavailable ? 'Pod health unavailable' : 'Nothing needs attention';
    empty.querySelector('small').textContent = unavailable
        ? 'This snapshot could not verify active workload failures.'
        : 'All monitored workloads are currently healthy.';
}

function renderTopPods(pods) {
    const body = $('overview-toppods-body');
    body.innerHTML = '';
    const hasPods = (pods?.length ?? 0) > 0;
    $('overview-toppods-empty').hidden = hasPods;
    $('top-consumers-state').textContent = hasPods ? 'Live metrics' : 'Unavailable';
    $('top-consumers-state').classList.toggle('has-issues', !hasPods);
    const maxCPU = Math.max(...(pods ?? []).map((pod) => pod.cpuMilli || 0), 1);
    for (const p of pods ?? []) {
        const button = document.createElement('button');
        button.type = 'button';
        button.className = 'overview-consumer';
        const share = Math.max(Math.round(((p.cpuMilli || 0) / maxCPU) * 100), 3);
        button.innerHTML = `<span class="overview-consumer-resource"><strong title="${esc(p.name)}">${esc(p.name)}</strong><small>${esc(p.namespace)}</small></span>
            <span class="overview-consumer-values"><strong class="mono">${p.cpuMilli || 0}m</strong><small class="mono">${p.memMi || 0}Mi</small></span>
            <span class="overview-consumer-meter" aria-hidden="true"><i style="width:${share}%"></i></span>`;
        button.addEventListener('click', () => openDrawer({ kind: 'Pod', namespace: p.namespace, name: p.name, isPod: true }));
        body.appendChild(button);
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
        total.textContent = 'Unavailable';
        total.classList.remove('has-issues');
        $('cluster-node-ratio').textContent = '– / –';
        setMetricState($('infrastructure-state'), 'Unavailable', 'unavailable');
        $('infrastructure-pressure').textContent = 'No node data';
        $('infrastructure-meta').textContent = 'Cluster capacity unavailable';
        return;
    }
    const ready = nodes.filter((node) => node.ready).length;
    // A deliberately unschedulable control-plane node is common and is not a
    // health failure by itself. Keep it visible in the row without raising the
    // infrastructure alarm unless readiness or pressure is also unhealthy.
    const troubled = nodes.filter((node) => !node.ready || (node.pressure?.length ?? 0) > 0).length;
    total.textContent = `${nodes.length} node${nodes.length === 1 ? '' : 's'}`;
    total.classList.toggle('has-issues', troubled > 0);
    $('cluster-node-ratio').textContent = `${ready} / ${nodes.length}`;
    setMetricState($('infrastructure-state'), troubled > 0 ? 'Attention' : 'Ready', troubled > 0 ? 'error' : 'ok');
    $('infrastructure-pressure').textContent = troubled > 0
        ? `${troubled} node${troubled === 1 ? ' needs' : 's need'} attention` : 'No pressure';
    $('infrastructure-meta').textContent = nodes.length === 1 && nodes[0].version
        ? nodes[0].version : `${ready} of ${nodes.length} nodes ready`;
    box.innerHTML = nodes.map((node) => {
        const pressure = node.pressure?.length ? node.pressure.map((signal) => signal.replace('Pressure', '')).join(', ') : 'None';
        const unhealthy = !node.ready || (node.pressure?.length ?? 0) > 0;
        return `<article class="overview-node-row${unhealthy ? ' overview-node-row-bad' : ''}">
            <button class="overview-node-name" type="button" data-name="${esc(node.name)}"><span class="overview-node-dot" aria-hidden="true"></span>${nodeNameHtml(node.name)}</button>
            <span class="overview-node-fact"><small>Status</small><strong>${node.ready ? 'Ready' : 'Not ready'}</strong></span>
            <span class="overview-node-fact"><small>Scheduling</small><strong>${node.schedulable ? 'Enabled' : 'Disabled'}</strong></span>
            <span class="overview-node-fact"><small>Pods</small><strong>${node.pods ?? 0}</strong></span>
            <span class="overview-node-fact"><small>Pressure</small><strong>${esc(pressure)}</strong></span>
        </article>`;
    }).join('');
    box.querySelectorAll('.overview-node-name').forEach((button) => {
        button.addEventListener('click', () => openDrawer({ kind: 'Node', namespace: '', name: button.dataset.name }));
    });
}

function updateCapacitySummary(metrics, sum) {
    const hint = $('capacity-hint');
    if (!metrics || !sum) {
        $('capacity-node-count').textContent = 'Metrics unavailable';
        $('capacity-cpu-pct').textContent = '–%';
        $('capacity-memory-pct').textContent = '–%';
        $('capacity-cpu-value').textContent = 'Live usage unavailable';
        $('capacity-memory-value').textContent = 'Live usage unavailable';
        $('capacity-cpu-meta').textContent = 'Install Metrics Server for usage data';
        $('capacity-memory-meta').textContent = 'Allocatable memory is not reported';
        $('capacity-cpu-ring').style.setProperty('--value', 0);
        $('capacity-memory-ring').style.setProperty('--value', 0);
        setCapacityState($('capacity-cpu-ring'), null);
        setCapacityState($('capacity-memory-ring'), null);
        hint.hidden = false;
        return;
    }
    const cpuPercent = pct(sum.cpu, sum.cpuCap);
    const memoryPercent = pct(sum.mem, sum.memCap);
    $('capacity-node-count').textContent = `${metrics.length} reporting`;
    $('capacity-cpu-pct').textContent = `${cpuPercent}%`;
    $('capacity-memory-pct').textContent = `${memoryPercent}%`;
    $('capacity-cpu-value').textContent = `${fmtCores(sum.cpu)} / ${fmtCores(sum.cpuCap)} cores`;
    $('capacity-memory-value').textContent = `${fmtMem(sum.mem)} / ${fmtMem(sum.memCap)}`;
    $('capacity-cpu-meta').textContent = `${fmtCores(Math.max(sum.cpuCap - sum.cpu, 0))} cores allocatable headroom`;
    $('capacity-memory-meta').textContent = `${fmtMem(Math.max(sum.memCap - sum.mem, 0))} allocatable headroom`;
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
    const state = bar.id === 'capacity-cpu-ring' ? $('capacity-cpu-state') : $('capacity-memory-state');
    if (value === null) setMetricState(state, 'Unavailable', 'unavailable');
    else if (value >= 90) setMetricState(state, 'Critical', 'error');
    else if (value >= 70) setMetricState(state, 'High', 'warn');
    else setMetricState(state, 'Normal', 'ok');
}

function setMetricState(element, label, state) {
    element.textContent = label;
    element.className = `overview-metric-state metric-state-${state}`;
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
            const rows = (nodes ?? []).map((n) => row(
                    `<td>${esc(n.name)}</td><td>${badge(n.status, n.ready)}</td><td>${esc(n.role)}</td><td class="mono">${esc(n.version)}</td><td>${esc(n.age)}</td>`,
                    { ref: { kind: 'Node', namespace: '', name: n.name } },
                ));
            setVirtualRows(body, rows);
        })
        .catch((err) => viewError(scope, err));
}

// ---- Namespaces ----
function loadNamespaces(scope) {
    return ListNamespaces()
        .then((namespaces) => {
            if (!isCurrentViewRequest(scope)) return;
            const body = $('namespaces-body');
            const rows = (namespaces ?? []).map((ns) => row(
                    `<td>${esc(ns.name)}</td><td>${badge(ns.status, ns.status === 'Active')}</td><td>${esc(ns.age)}</td>`,
                    { ref: { kind: 'Namespace', namespace: '', name: ns.name } },
                ));
            setVirtualRows(body, rows);
        })
        .catch((err) => viewError(scope, err));
}

// ---- Pods (enriched: live CPU/mem, IP, node, age, per-row actions) ----
function loadPods(scope) {
    podsPageState = { scope, token: '', loading: false, loaded: 0 };
    return loadNextPodsPage(true);
}

const RESOURCE_PAGE_LIMIT = 200;
let podsPageState = null;

function updatePageControls(prefix, loaded, page) {
    const box = $(`${prefix}-pagination`);
    const button = $(`${prefix}-load-more`);
    const token = page?.continue ?? '';
    const remaining = Number(page?.remaining ?? -1);
    box.hidden = loaded === 0;
    button.hidden = !token;
    $(`${prefix}-page-status`).textContent = token
        ? (remaining >= 0 ? `${loaded} loaded · ${remaining} remaining` : `${loaded} loaded · more available`)
        : `${loaded} loaded · complete`;
}

function loadNextPodsPage(reset = false) {
    const state = podsPageState;
    if (!state || state.loading || !isCurrentViewRequest(state.scope)) return Promise.resolve();
    if (!reset && !state.token) return Promise.resolve();
    state.loading = true;
    $('pods-load-more').disabled = true;
    $('pods-page-status').textContent = reset ? 'Loading first page…' : `Loading after ${state.loaded}…`;
    return PodsPage(state.scope.namespace, state.token, RESOURCE_PAGE_LIMIT)
        .then((snapshot) => {
            if (state !== podsPageState || !isCurrentViewRequest(state.scope)) return;
            const pods = snapshot?.pods ?? [];
            const metrics = snapshot?.metrics ?? [];
            const usage = {};
            for (const m of metrics) usage[`${m.namespace}/${m.name}`] = m;
            const body = $('pods-body');
            const rows = pods.map((p) => {
                const m = usage[`${p.namespace}/${p.name}`];
                const cpu = m ? `${m.cpuMilli}m` : '–';
                const mem = m ? `${m.memMi}Mi` : '–';
                const ref = { kind: 'Pod', namespace: p.namespace, name: p.name, isPod: true };
                return row(
                    `<td>${esc(p.name)}</td><td>${esc(p.namespace)}</td><td>${esc(p.ready)}</td><td>${badge(p.status, !p.isError)}</td><td>${p.restarts}</td><td class="mono">${cpu}</td><td class="mono">${mem}</td><td class="mono">${esc(p.podIP)}</td><td class="mono">${esc(p.node)}</td><td>${esc(p.age)}</td>`,
                    { isError: p.isError, ref },
                );
            });
            state.loaded += rows.length;
            state.token = snapshot?.page?.continue ?? '';
            const options = { hasMore: !!state.token, onNearEnd: () => loadNextPodsPage(false) };
            if (reset) setVirtualRows(body, rows, options);
            else appendVirtualRows(body, rows, options);
            updatePageControls('pods', state.loaded, snapshot?.page);
            $('pods-empty').hidden = state.loaded > 0;
            filterCurrentTable();
        })
        .catch((err) => viewError(state.scope, err))
        .finally(() => {
            if (state !== podsPageState) return;
            state.loading = false;
            $('pods-load-more').disabled = false;
        });
}

$('pods-load-more').addEventListener('click', () => loadNextPodsPage(false));

// A three-dot actions button + per-row menu, like Lens/Headlamp.
function actionsBtn(ref = null) {
    const owner = ref ? ` for ${ref.kind} ${ref.namespace ? `${ref.namespace}/` : ''}${ref.name}` : '';
    return `<button class="row-actions-btn" type="button" aria-haspopup="menu" aria-expanded="false" aria-label="Actions${esc(owner)}">⋯</button>`;
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
            $('deployments-empty').hidden = (deps?.length ?? 0) > 0;
            const rows = (deps ?? []).map((d) => row(
                    `<td>${esc(d.namespace)}</td><td>${esc(d.name)}</td><td>${badge(d.ready, !d.isError)}</td><td>${d.upToDate}</td><td>${d.available}</td><td>${esc(d.age)}</td>`,
                    { isError: d.isError, ref: { kind: 'Deployment', namespace: d.namespace, name: d.name } },
                ));
            setVirtualRows(body, rows);
        })
        .catch((err) => viewError(scope, err));
}

// ---- Services ----
function loadServices(scope) {
    return ListServices(scope.namespace)
        .then((svcs) => {
            if (!isCurrentViewRequest(scope)) return;
            const body = $('services-body');
            $('services-empty').hidden = (svcs?.length ?? 0) > 0;
            const rows = (svcs ?? []).map((s) => row(
                    `<td>${esc(s.namespace)}</td><td>${esc(s.name)}</td><td>${esc(s.type)}</td><td class="mono">${esc(s.clusterIP)}</td><td class="mono">${esc(s.ports)}</td><td>${esc(s.age)}</td>`,
                    { ref: { kind: 'Service', namespace: s.namespace, name: s.name } },
                ));
            setVirtualRows(body, rows);
        })
        .catch((err) => viewError(scope, err));
}

// ---- ConfigMaps ----
function loadConfigMaps(scope) {
    return ListConfigMaps(scope.namespace)
        .then((items) => {
            if (!isCurrentViewRequest(scope)) return;
            const body = $('configmaps-body');
            $('configmaps-empty').hidden = (items?.length ?? 0) > 0;
            const rows = (items ?? []).map((c) => row(
                    `<td>${esc(c.namespace)}</td><td>${esc(c.name)}</td><td>${c.keys}</td><td>${esc(c.age)}</td>`,
                    { ref: { kind: 'ConfigMap', namespace: c.namespace, name: c.name } },
                ));
            setVirtualRows(body, rows);
        })
        .catch((err) => viewError(scope, err));
}

// ---- Secrets ----
function loadSecrets(scope) {
    return ListSecrets(scope.namespace)
        .then((items) => {
            if (!isCurrentViewRequest(scope)) return;
            const body = $('secrets-body');
            $('secrets-empty').hidden = (items?.length ?? 0) > 0;
            const rows = (items ?? []).map((s) => row(
                    `<td>${esc(s.namespace)}</td><td>${esc(s.name)}</td><td class="mono">${esc(s.type)}</td><td>${s.keys}</td><td>${esc(s.age)}</td>`,
                    { ref: { kind: 'Secret', namespace: s.namespace, name: s.name } },
                ));
            setVirtualRows(body, rows);
        })
        .catch((err) => viewError(scope, err));
}

// ============ Detail drawer ============

let drawerRef = null; // { kind, namespace, name, isPod }
let activeDrawerScope = null;
let drawerLoadedTabs = new Set();
let drawerContainerOwnerKey = '';
let drawerContainerPromise = null;
let drawerReturnFocus = null;

function isCurrentDrawerRequest(scope) {
    return !!drawerRef
        && !$('drawer').hidden
        && requestScopes.isCurrentDrawer(scope, drawerRef);
}

function openDrawer(ref) {
    if (ref.kind === 'HelmRelease') { openHelmDetailModal(ref); return; }
    const drawerWasHidden = $('drawer').hidden;
    if (drawerWasHidden) drawerReturnFocus = document.activeElement;
    if (activeDrawerScope) CancelDrawerSnapshot(requestScopes.drawerOwnerKey(activeDrawerScope)).catch(() => {});
    if (drawerRef && !$('drawer').hidden) stopEphemeralForwards(drawerRef);
    stopIncidentWatch();
    stopFollow();
    stopExec();
    drawerRef = ref;
    activeDrawerScope = requestScopes.openDrawer(ref);
    const scope = activeDrawerScope;
    resetDrawerLoads(scope);
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
    $('sidebar').inert = true;
    document.querySelector('.main').inert = true;

    // Gate the write buttons and the subresource tabs. Applied twice: once from
    // whatever answer is already cached, then again when the probe returns — but
    // only if this drawer is still the one open.
    applyDrawerAccess(ref);
    accessSet(ref.kind, ref.namespace).then(() => {
        if (isCurrentDrawerRequest(scope)) applyDrawerAccess(ref);
    });

    resetAIPanel(ref);
    resetIncidentPanel();
    setDrawerTab(ref.tab ?? 'details');
    if (drawerWasHidden) requestAnimationFrame(() => $('drawer-close').focus());
}

function resetDrawerLoads(scope) {
    drawerLoadedTabs = new Set();
    drawerContainerOwnerKey = requestScopes.drawerOwnerKey(scope);
    drawerContainerPromise = null;
}

function drawerPodContainers(scope) {
    const ownerKey = requestScopes.drawerOwnerKey(scope);
    if (ownerKey !== drawerContainerOwnerKey) return Promise.reject(new Error('The Pod drawer changed.'));
    if (!drawerContainerPromise) {
        const ref = scope.ref;
        // One read serves both tabs: Terminal needs the names, Logs also the
        // restart state that points at the instance that failed.
        drawerContainerPromise = PodContainerStates(ref.namespace, ref.name);
    }
    return drawerContainerPromise;
}

function ensureDrawerTabLoaded(name) {
    const scope = activeDrawerScope;
    if (!scope || !isCurrentDrawerRequest(scope) || drawerLoadedTabs.has(name)) return;
    drawerLoadedTabs.add(name);
    if (name === 'details') loadDetails(scope);
    else if (name === 'yaml') loadYAML(scope);
    else if (name === 'logs' && scope.ref.isPod) prepareLogs(scope);
    else if (name === 'terminal' && scope.ref.isPod) prepareTerminal(scope);
    else if (name === 'forward' && (scope.ref.isPod || scope.ref.kind === 'Service')) prepareForward();
    else if (name === 'investigate') loadIncident(scope);
}

// Disable what this token cannot do, rather than hiding it — a greyed-out
// Terminal tab with "your token cannot exec" says something true; a missing tab
// would suggest Kubby has no terminal.
function applyDrawerAccess(ref) {
    const set = accessPeek(ref.kind, ref.namespace);
    const reason = (verb) => denyReason(ref.kind, ref.namespace, verb);

    // Drawer YAML Save sends a full-object Update. Patch permission alone is
    // insufficient even though Create/Import use server-side apply elsewhere.
    const ownsYAML = yamlReady
        && yamlOwnerKey === requestScopes.drawerOwnerKey(activeDrawerScope)
        && isCurrentDrawerRequest(activeDrawerScope);
    gate($('btn-yaml-save'), ownsYAML && allowed(set, 'update'),
        ownsYAML ? reason('update') : 'Wait for this resource YAML to finish loading.');
    gate($('btn-delete'), allowed(set, 'delete'), reason('delete'));
    gate($('btn-scale'), allowed(set, 'scale'), reason('scale'));
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
    const closingScope = activeDrawerScope;
    stopFollow();
    stopExec();
    stopIncidentWatch();
    cancelPendingForwardForDrawer(closingRef);
    if (closingScope) CancelDrawerSnapshot(requestScopes.drawerOwnerKey(closingScope)).catch(() => {});
    requestScopes.closeDrawer();
    activeDrawerScope = null;
    yamlReady = false;
    yamlOwnerKey = '';
    yamlRequestId++;
    drawerRef = null;
    $('drawer').hidden = true;
    $('drawer-backdrop').hidden = true;
    $('sidebar').inert = false;
    document.querySelector('.main').inert = false;
    if (closingRef) stopEphemeralForwards(closingRef);
    const target = drawerReturnFocus;
    drawerReturnFocus = null;
    if (target?.isConnected) requestAnimationFrame(() => target.focus());
}

// ---- Scale / Restart (Deployment) ----
$('btn-scale').addEventListener('click', () => { if (drawerRef) scaleRef(drawerRef); });

// Open the Scale modal for a deployment, prefilling its current replica count.
function scaleRef(ref) {
	const connectionID = $('cluster-select').value;
	const scope = openScaleModal(ref, 1, { loading: true, connectionID });
	// A Deployment an HPA manages is scaled back at the next sync, so a manual
	// scale looks successful and then silently reverts. Say so before Scale.
	ListHorizontalPodAutoscalers(ref.namespace)
		.then((hpas) => {
			if (!isCurrentModalRequest(scope)) return;
			const hpa = (hpas ?? []).find((h) => h.target === `Deployment/${ref.name}`);
			if (!hpa) return;
			const note = $('scale-hpa-note');
			note.innerHTML = `<strong>HorizontalPodAutoscaler ${esc(hpa.name)} manages this Deployment (${esc(hpa.minMax)} replicas).</strong>
				<span>A manual scale is overwritten at the autoscaler's next sync — change its replica range instead.</span>
				<button type="button" class="btn btn-secondary btn-sm" id="scale-open-hpa">Open autoscaler</button>`;
			note.hidden = false;
			$('scale-open-hpa').addEventListener('click', () => {
				closeModal();
				openDrawer({ kind: 'HorizontalPodAutoscaler', namespace: hpa.namespace, name: hpa.name });
			});
		})
		.catch(() => {});
	GetDetail(ref.kind, ref.namespace, ref.name)
		.then((d) => {
			if (!isCurrentModalRequest(scope) || $('cluster-select').value !== connectionID) return;
			const field = (d.info ?? []).find((f) => f.label === 'Replicas');
			const current = field ? parseInt(field.value, 10) : 1;
			const input = $('modal-input');
			input.value = String(Number.isNaN(current) ? 1 : current);
			input.disabled = false;
			$('modal-ok').disabled = false;
		})
		.catch((err) => {
			if (!isCurrentModalRequest(scope)) return;
			$('modal-error').textContent = errMsg(err);
			$('modal-error').hidden = false;
		});
}

function restartRef(ref) {
    const connectionID = $('cluster-select').value;
    showConfirm(`Rolling-restart deployment “${ref.name}”?`, { title: 'Restart deployment', icon: '🔄', okText: 'Restart' }).then((ok) => {
        if (!ok) return;
        RestartDeploymentOwned(connectionID, ref.namespace, ref.name)
            .then(() => { if (drawerRef && drawerRef.name === ref.name) loadDetails(); refreshCurrentView(); })
            .catch((err) => showError(errMsg(err)));
    });
}

// Rolling-restart for StatefulSet / DaemonSet (fn = RestartStatefulSet|RestartDaemonSet).
function restartWorkload(ref, fn) {
    const connectionID = $('cluster-select').value;
    showConfirm(`Rolling-restart ${ref.kind} “${ref.name}”?`, { title: `Restart ${ref.kind}`, icon: '🔄', okText: 'Restart' }).then((ok) => {
        if (!ok) return;
        fn(connectionID, ref.namespace, ref.name)
            .then(() => { if (drawerRef && drawerRef.name === ref.name) loadDetails(); refreshCurrentView(); })
            .catch((err) => showError(errMsg(err)));
    });
}

function openScaleModal(ref, current, { loading = false, connectionID = $('cluster-select').value } = {}) {
	let scope;
	scope = openModal({
		title: `Scale "${ref.name}"`,
        eyebrow: 'Workload action',
		description: `${ref.namespace || 'cluster-scoped'} · Deployment`,
		size: 'compact',
		ownerKey: modalOwner('scale', connectionID, ref.kind, ref.namespace, ref.name),
		okText: 'Scale',
		okDisabled: loading,
		bodyHtml: `<div class="modal-field">
			<label for="modal-input">Desired replicas</label>
			<input type="number" id="modal-input" class="modal-number" min="0" max="1000" value="${current}"${loading ? ' disabled' : ''}>
            <p class="modal-hint">Setting this to zero stops every Pod managed by this Deployment.</p>
            <div id="scale-hpa-note" class="modal-callout modal-callout-warn" role="note" hidden></div>
        </div>`,
		onOpen: () => { const el = $('modal-input'); el.focus(); el.select(); },
		onOk: () => {
			if (!isCurrentModalRequest(scope) || $('cluster-select').value !== connectionID) return Promise.reject('The active cluster changed. Reopen Scale.');
			const n = parseInt($('modal-input').value, 10);
			if (Number.isNaN(n) || n < 0) return Promise.reject('Enter a non-negative number.');
			return ScaleDeploymentOwned(connectionID, ref.namespace, ref.name, n).then(() => {
                if (drawerRef && drawerRef.name === ref.name && !$('drawer').hidden) loadDetails();
                loadSidebarCounts();
                refreshCurrentView();
            });
		},
	});
	return scope;
}

$('btn-restart').addEventListener('click', () => {
    const ref = drawerRef;
    const scope = activeDrawerScope;
    if (!ref || !scope) return;
    const connectionID = $('cluster-select').value;
    showConfirm(`Rolling-restart deployment “${ref.name}”?`, { title: 'Restart deployment', icon: '🔄', okText: 'Restart' }).then((ok) => {
        if (!ok || !isCurrentDrawerRequest(scope) || $('cluster-select').value !== connectionID) return;
        RestartDeploymentOwned(connectionID, ref.namespace, ref.name)
            .then(() => {
                if (isCurrentDrawerRequest(scope)) loadDetails(scope);
                if (requestScopes.isCurrentConnection(scope.connectionEpoch)) refreshCurrentView();
            })
            .catch((err) => {
                if (requestScopes.isCurrentConnection(scope.connectionEpoch)) showError(errMsg(err));
            });
    });
});

$('drawer-close').addEventListener('click', closeDrawer);
$('drawer-backdrop').addEventListener('click', closeDrawer);

$('btn-delete').addEventListener('click', () => {
    const ref = drawerRef;
    const scope = activeDrawerScope;
    if (!ref || !scope) return;
    const connectionID = $('cluster-select').value;
    showConfirm(`Delete ${ref.kind} “${ref.name}”${ref.namespace ? ` in ${ref.namespace}` : ''}?\nThis cannot be undone.`, { title: `Delete ${ref.kind}`, icon: '🗑', okText: 'Delete', danger: true }).then((ok) => {
        if (!ok || !isCurrentDrawerRequest(scope) || $('cluster-select').value !== connectionID) return;
        DeleteResourceOwned(connectionID, ref.kind, ref.namespace, ref.name)
            .then(() => {
                if (isCurrentDrawerRequest(scope)) closeDrawer();
                if (requestScopes.isCurrentConnection(scope.connectionEpoch)) refreshCurrentView();
            })
            .catch((err) => {
                if (requestScopes.isCurrentConnection(scope.connectionEpoch)) showError(errMsg(err));
            });
    });
});

document.querySelectorAll('.drawer-tab').forEach((tab) => {
    tab.addEventListener('click', () => setDrawerTab(tab.dataset.dtab));
    tab.addEventListener('keydown', (event) => {
        if (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight' && event.key !== 'Home' && event.key !== 'End') return;
        const tabs = [...document.querySelectorAll('.drawer-tab:not([hidden]):not(:disabled)')];
        const current = tabs.indexOf(tab);
        const next = event.key === 'Home' ? 0
            : event.key === 'End' ? tabs.length - 1
            : (current + (event.key === 'ArrowRight' ? 1 : -1) + tabs.length) % tabs.length;
        event.preventDefault();
        tabs[next]?.focus();
        if (tabs[next]) setDrawerTab(tabs[next].dataset.dtab);
    });
});

function setDrawerTab(name) {
    document.querySelectorAll('.drawer-tab').forEach((t) => {
        const active = t.dataset.dtab === name;
        t.classList.toggle('active', active);
        t.setAttribute('aria-selected', String(active));
        t.tabIndex = active ? 0 : -1;
    });
    $('dpanel-details').hidden = name !== 'details';
    $('dpanel-yaml').hidden = name !== 'yaml';
    $('dpanel-logs').hidden = name !== 'logs';
    $('dpanel-terminal').hidden = name !== 'terminal';
    $('dpanel-forward').hidden = name !== 'forward';
    $('dpanel-investigate').hidden = name !== 'investigate';
    $('dpanel-ai').hidden = name !== 'ai';
    ensureDrawerTabLoaded(name);
    if (name === 'ai') { prepareAIPanel(); $('ai-input').focus(); }
    if (name === 'terminal') requestAnimationFrame(() => {
        loadTerminalRuntime().then(() => {
            if ($('dpanel-terminal').hidden) return;
            fitTerminal();
            if (execConnected) execTerminal?.focus();
        }).catch((err) => {
            if (!$('dpanel-terminal').hidden) setTerminalStatus('error', `Terminal failed to load: ${errMsg(err)}`);
        });
    });
}

// ---- Incident Studio: deterministic evidence before AI ----

const INCIDENT_WATCH_MS = 60_000;
const INCIDENT_POLL_MS = 5_000;
let incidentReport = null;
let incidentRequestSeq = 0;
let incidentWatchTimer = null;
let incidentWatchDeadline = 0;
let incidentWatchOwner = '';
let incidentWatchBusy = false;

function resetIncidentPanel() {
    incidentRequestSeq++;
    incidentReport = null;
    $('incident-hero').dataset.state = 'loading';
    $('incident-summary').textContent = 'Building an incident snapshot…';
    $('incident-meta').textContent = 'Correlating state, Events, ownership and serving endpoints.';
    $('incident-loading').hidden = false;
    $('incident-content').hidden = true;
    $('incident-error').hidden = true;
    $('btn-incident-export').disabled = true;
    $('btn-incident-watch').disabled = false;
    $('btn-incident-watch').textContent = 'Watch recovery';
    $('btn-incident-watch').setAttribute('aria-pressed', 'false');
}

function loadIncident(scope = activeDrawerScope, { silent = false } = {}) {
    if (!scope || !isCurrentDrawerRequest(scope)) return Promise.resolve(null);
    const requestID = ++incidentRequestSeq;
    const ref = scope.ref;
    if (!silent) {
        $('incident-loading').hidden = false;
        $('incident-error').hidden = true;
        $('btn-incident-refresh').disabled = true;
    }
    return InvestigateResource(ref.kind, ref.namespace, ref.name)
        .then((payload) => {
            if (requestID !== incidentRequestSeq || !isCurrentDrawerRequest(scope)) return null;
            const report = typeof payload === 'string' ? JSON.parse(payload) : payload;
            incidentReport = report;
            renderIncident(report);
            if (incidentWatchTimer && report.state === 'healthy') {
                stopIncidentWatch('verified');
            }
            return report;
        })
        .catch((err) => {
            if (requestID !== incidentRequestSeq || !isCurrentDrawerRequest(scope)) return null;
            $('incident-error').textContent = `Investigation unavailable: ${errMsg(err)}`;
            $('incident-error').hidden = false;
            if (!incidentReport) $('incident-content').hidden = true;
            return null;
        })
        .finally(() => {
            if (requestID !== incidentRequestSeq || !isCurrentDrawerRequest(scope)) return;
            $('incident-loading').hidden = true;
            $('btn-incident-refresh').disabled = false;
        });
}

function renderIncident(report) {
    const findings = report?.findings ?? [];
    const related = report?.related ?? [];
    const timeline = report?.timeline ?? [];
    const actions = report?.actions ?? [];
    const limitations = report?.limitations ?? [];
    const state = report?.state ?? 'warning';
    $('incident-hero').dataset.state = state;
    $('incident-summary').textContent = report?.summary || 'No conclusion is available.';
    const observed = report?.observedAt ? new Date(report.observedAt).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' }) : 'now';
    $('incident-meta').textContent = `${stateLabel(state)} · ${findings.length} finding${findings.length === 1 ? '' : 's'} · observed ${observed}`;
    $('incident-finding-count').textContent = findings.length ? `${findings.length} finding${findings.length === 1 ? '' : 's'}` : 'Healthy snapshot';

    $('incident-findings').innerHTML = findings.length
        ? findings.map((finding) => `<article class="incident-finding" data-severity="${esc(finding.severity)}">
            <div class="incident-finding-head"><h5>${esc(finding.title)}</h5><span class="incident-confidence">${esc(finding.confidence)} confidence</span></div>
            <p>${esc(finding.explanation)}</p>
            ${(finding.evidence ?? []).length ? `<ul class="incident-evidence">${finding.evidence.map((evidence) => `<li>${esc(evidence)}</li>`).join('')}</ul>` : ''}
        </article>`).join('')
        : '<div class="incident-healthy"><span>✓</span>No deterministic issue is visible in this snapshot.</div>';

    $('incident-actions').innerHTML = actions.length
        ? actions.map((action, index) => `<button type="button" class="incident-action" data-incident-action="${index}"><strong>${esc(action.label)}</strong><small>${esc(action.description)}</small><span aria-hidden="true">→</span></button>`).join('')
        : '<p class="incident-empty">No remediation hand-off is available for this resource.</p>';
    $('incident-actions').querySelectorAll('[data-incident-action]').forEach((button) => {
        button.addEventListener('click', () => runIncidentAction(actions[Number(button.dataset.incidentAction)]));
    });

    $('incident-related').innerHTML = related.length
        ? related.map((item, index) => `<button type="button" class="incident-related-button" data-incident-related="${index}"><strong>${esc(item.kind)} · ${esc(item.name)}</strong><small>${esc(item.role)}${item.status ? ` · ${esc(item.status)}` : ''}</small><span aria-hidden="true">→</span></button>`).join('')
        : '<p class="incident-empty">No owner or serving relationship was found.</p>';
    $('incident-related').querySelectorAll('[data-incident-related]').forEach((button) => {
        button.addEventListener('click', () => {
            const item = related[Number(button.dataset.incidentRelated)];
            openDrawer({ kind: item.kind, namespace: item.namespace, name: item.name, isPod: item.kind === 'Pod' });
        });
    });

    $('incident-timeline').innerHTML = timeline.length
        ? timeline.map((moment) => `<li><time class="incident-time" datetime="${esc(moment.at)}">${esc(moment.age || shortIncidentTime(moment.at))}</time><div class="incident-moment" data-severity="${esc(moment.severity)}"><span class="incident-source">${esc(moment.source)}</span><strong>${esc(moment.title)}</strong>${moment.detail ? `<small>${esc(moment.detail)}</small>` : ''}</div></li>`).join('')
        : '<li class="incident-empty">No timestamped evidence is retained.</li>';

    $('incident-limits-section').hidden = limitations.length === 0;
    $('incident-limitations').innerHTML = limitations.map((limitation) => `<li>${esc(limitation)}</li>`).join('');
    $('incident-loading').hidden = true;
    $('incident-content').hidden = false;
    $('incident-error').hidden = true;
    $('btn-incident-export').disabled = false;
}

function stateLabel(state) {
    if (state === 'critical') return 'Critical evidence';
    if (state === 'warning') return 'Needs attention';
    return 'Healthy now';
}

function shortIncidentTime(value) {
    if (!value) return '—';
    const date = new Date(value);
    return Number.isNaN(date.getTime()) ? value : date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
}

function runIncidentAction(action) {
    if (!action) return;
    const ref = { kind: action.kind, namespace: action.namespace, name: action.name, isPod: action.kind === 'Pod' };
    if (action.action === 'logs' || action.action === 'yaml' || action.action === 'ai') {
        openDrawer({ ...ref, tab: action.action });
        return;
    }
    if (action.action === 'rollout') {
        closeDrawer();
        openRolloutModal(ref);
        return;
    }
    if (action.action === 'sizing' || action.action === 'topology') {
        const targetView = action.action === 'sizing' ? 'sizing' : 'structure';
        closeDrawer();
        currentNamespace = action.namespace || '';
        $('namespace-select').value = currentNamespace;
        syncNamespacePicker();
        selectView(targetView);
    }
}

function startIncidentWatch() {
    const scope = activeDrawerScope;
    if (!scope || !isCurrentDrawerRequest(scope)) return;
    if (incidentWatchTimer) {
        stopIncidentWatch();
        return;
    }
    incidentWatchOwner = requestScopes.drawerOwnerKey(scope);
    incidentWatchDeadline = Date.now() + INCIDENT_WATCH_MS;
    $('btn-incident-watch').textContent = 'Stop watching';
    $('btn-incident-watch').setAttribute('aria-pressed', 'true');
    $('incident-meta').textContent = 'Watching recovery every 5 seconds · up to 60 seconds';
    const poll = () => {
        if (incidentWatchBusy || !activeDrawerScope || requestScopes.drawerOwnerKey(activeDrawerScope) !== incidentWatchOwner) {
            if (!activeDrawerScope || requestScopes.drawerOwnerKey(activeDrawerScope) !== incidentWatchOwner) stopIncidentWatch();
            return;
        }
        if (Date.now() >= incidentWatchDeadline) {
            stopIncidentWatch('timeout');
            return;
        }
        incidentWatchBusy = true;
        loadIncident(activeDrawerScope, { silent: true }).finally(() => { incidentWatchBusy = false; });
    };
    incidentWatchTimer = window.setInterval(poll, INCIDENT_POLL_MS);
    poll();
}

function stopIncidentWatch(outcome = '') {
    if (incidentWatchTimer) window.clearInterval(incidentWatchTimer);
    incidentWatchTimer = null;
    incidentWatchOwner = '';
    incidentWatchBusy = false;
    const button = $('btn-incident-watch');
    if (button) {
        button.textContent = outcome === 'verified' ? 'Recovery verified' : 'Watch recovery';
        button.setAttribute('aria-pressed', 'false');
        button.disabled = outcome === 'verified';
    }
    if (outcome === 'verified') {
        $('incident-meta').textContent = `Recovery verified · no deterministic issue at ${new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })}`;
    } else if (outcome === 'timeout') {
        $('incident-meta').textContent = 'Recovery was not verified within 60 seconds. Refresh evidence or continue watching.';
    }
}

$('btn-incident-refresh').addEventListener('click', () => loadIncident());
$('btn-incident-watch').addEventListener('click', startIncidentWatch);
$('btn-incident-export').addEventListener('click', () => {
    const report = incidentReport;
    const scope = activeDrawerScope;
    if (!report || !scope || !isCurrentDrawerRequest(scope)) return;
    const button = $('btn-incident-export');
    button.disabled = true;
    SaveIncidentReport(JSON.stringify(report))
        .then((path) => {
            if (path && isCurrentDrawerRequest(scope) && incidentReport === report) {
                $('incident-meta').textContent = `Incident report saved to ${path}`;
            }
        })
        .catch((err) => { if (isCurrentDrawerRequest(scope)) showError(errMsg(err)); })
        .finally(() => {
            if (isCurrentDrawerRequest(scope) && incidentReport === report) button.disabled = false;
        });
});

// ---- Details + Events ----
function loadDetails(scope = activeDrawerScope) {
    if (!scope || !isCurrentDrawerRequest(scope)) return;
    const ref = scope.ref;
    $('detail-meta').innerHTML = '<p class="empty-inline">Loading…</p>';
    $('detail-relations').innerHTML = '';
    $('detail-events-body').innerHTML = '';
    $('detail-events-empty').hidden = true;

    const empty = $('detail-events-empty');
    empty.className = 'empty-inline';
    empty.textContent = 'Loading events…';
    empty.hidden = false;
    const operationID = requestScopes.drawerOwnerKey(scope);
    const connectionID = $('cluster-select').value;
    GetDrawerSnapshotOwned(connectionID, operationID, ref.kind, ref.namespace, ref.name)
        .then((payload) => {
            if (!isCurrentDrawerRequest(scope)) return;
            const snapshot = typeof payload === 'string' ? JSON.parse(payload) : payload;
            renderDetailMeta(snapshot.detail);
            renderDrawerEvents(snapshot.events, snapshot.sectionErrors?.events, scope);
            loadRelations(scope, snapshot);
        })
        .catch((err) => {
            if (!isCurrentDrawerRequest(scope)) return;
            $('detail-meta').innerHTML = `<p class="error">${esc(errMsg(err))}</p>`;
            renderDrawerEvents([], errMsg(err), scope);
        });

    if (ref.kind === 'Secret') loadSecretData(scope);
}

function loadDrawerEvents(scope = activeDrawerScope) {
    if (!scope || !isCurrentDrawerRequest(scope)) return;
    const ref = scope.ref;
    const empty = $('detail-events-empty');
    empty.className = 'empty-inline';
    empty.textContent = 'Loading events…';
    empty.hidden = false;
    ListEvents(ref.kind, ref.namespace, ref.name)
        .then((events) => {
            if (isCurrentDrawerRequest(scope)) renderDrawerEvents(events, '', scope);
        })
        .catch((err) => { if (isCurrentDrawerRequest(scope)) renderDrawerEvents([], errMsg(err), scope); });
}

function renderDrawerEvents(events, error, scope) {
    const body = $('detail-events-body');
    const empty = $('detail-events-empty');
    body.innerHTML = '';
    if (error) {
        empty.className = 'empty-inline detail-events-unavailable';
        empty.innerHTML = `<strong>Events unavailable</strong><span>${esc(error)}</span><button type="button" class="btn btn-secondary btn-sm">Try again</button>`;
        empty.hidden = false;
        empty.querySelector('button').addEventListener('click', () => loadDrawerEvents(scope));
        return;
    }
    empty.className = 'empty-inline';
    empty.textContent = 'No events for this resource.';
    empty.hidden = (events?.length ?? 0) > 0;
    for (const e of events ?? []) {
        const tr = document.createElement('tr');
        const cls = e.isWarn ? 'ev-type-warn' : 'ev-type-normal';
        const count = e.count > 1 ? ` (x${e.count})` : '';
        tr.innerHTML = `<td class="${cls}">${esc(e.type)}</td><td>${esc(e.reason)}</td><td>${esc(e.age)}${count}</td><td>${esc(e.message)}</td>`;
        body.appendChild(tr);
    }
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
        <span aria-hidden="true">⌁</span> Investigate this ${esc(d.kind ?? drawerRef?.kind ?? 'resource')}
    </button>`;
    html += `<div class="detail-group"><div class="detail-group-title">Overview</div>${rows.join('')}</div>`;
    html += chipsGroup('Labels', d.labels);
    html += chipsGroup('Annotations', d.annotations);
    $('detail-meta').innerHTML = html;
    $('detail-meta').querySelector('.ai-explain-btn')?.addEventListener('click', () => setDrawerTab('investigate'));
}

function detailRow(k, v) {
    return `<div class="detail-row"><span class="k">${esc(k)}</span><span class="v">${esc(v)}</span></div>`;
}

// Relations tree: Deployment→RS→Pod, Service→Pod, Ingress→Service→Pod.
function loadRelations(scope, snapshot = {}) {
    const ref = scope.ref;
    $('detail-relations').innerHTML = '';
    if (ref.kind === 'Node') {
        renderNodePods(snapshot.nodePods, scope, snapshot.sectionErrors?.nodePods);
        return;
    }
    if (ref.kind === 'Namespace') {
        renderNamespaceSummary(snapshot.namespaceInfo, scope, snapshot.sectionErrors?.namespaceInfo);
        return;
    }
    const tree = snapshot.relation;
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
function renderNodePods(pods, scope, error = '') {
    const ref = scope.ref;
    const box = $('detail-relations');
    if (error) { box.innerHTML = `<p class="error">${esc(error)}</p>`; return; }
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
}

// Namespace → summary: per-kind counts, click a card to jump into that view scoped here.
function renderNamespaceSummary(counts, scope, error = '') {
    const ref = scope.ref;
    const box = $('detail-relations');
    if (error) { box.innerHTML = `<p class="error">${esc(error)}</p>`; return; }
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
}

// Point the namespace selector at a specific namespace and refresh scope/counts.
function setNamespaceScope(ns) {
    const sel = $('namespace-select');
    if ([...sel.options].some((o) => o.value === ns)) {
        sel.value = ns;
        currentNamespace = ns;
        syncNamespacePicker();
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
// The last rendered snapshot, which Check traffic uses to suggest Services and Pods.
let lastTrafficFlows = null;
// NetworkPolicy overlay for the snapshot being rendered, keyed "namespace/pod".
// The backend sends it once per Pod rather than on every endpoint row.
let trafficPodPolicies = {};

function podPoliciesFor(pod) {
    return trafficPodPolicies[`${pod.namespace}/${pod.name}`] ?? { ingress: [], egress: [] };
}

function loadTraffic(scope) {
    const reqId = ++trafficReqId;
    // Never suggest another cluster's or namespace's Services while this loads.
    lastTrafficFlows = null;
    const ingBox = $('traffic-ingress');
    const svcBox = $('traffic-services');
    return NetworkFlows(scope.namespace || '')
        .then((flows) => {
            if (reqId !== trafficReqId || !isCurrentViewRequest(scope)) return;
            lastTrafficFlows = flows;
            trafficPodPolicies = flows?.podPolicies ?? {};
            const ings = flows?.ingresses ?? [];
            const svcs = flows?.services ?? [];
			const warnings = flows?.warnings ?? [];
			$('traffic-warnings').textContent = warnings.join(' · ');
			$('traffic-warnings').hidden = warnings.length === 0;

            renderFlowSummary(flows, ings, svcs);

            $('flow-ing-count').textContent = ings.length ? String(ings.length) : '';
            $('traffic-ingress-empty').hidden = ings.length > 0;
            ingBox.innerHTML = ings.map(flowIngressCard).join('');

            $('traffic-svc-count').textContent = svcs.length ? String(svcs.length) : '';
            $('traffic-services-empty').hidden = svcs.length > 0;
            svcBox.innerHTML = svcs.map((s) => flowHopRow(s, { standalone: true })).join('');

            wireFlowNodes(ingBox);
            wireFlowNodes(svcBox);
            wireFlowChecks(ingBox);
            wireFlowChecks(svcBox);
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
			$('traffic-warnings').hidden = true;
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
    // Isolation is not breakage — whether traffic passes needs a source — so
    // policies get their own neutral tile rather than joining Broken paths.
    if (flows?.policiesAvailable) {
        const isolated = flows.isolatedPods ?? 0;
        tiles.push({ n: flows.policyCount ?? 0, label: 'Network policies', hint: `${isolated} endpoint pod${isolated === 1 ? '' : 's'} isolated` });
    }
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
        ${ing.entryNote ? `<p class="flow-note">${esc(ing.entryNote)}</p>` : ''}
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
            ${flowEntryPolicyHtml(svc)}
            ${svc.namespace && podCount ? `<button type="button" class="flow-check" data-check-service="${esc(`${svc.namespace}/${svc.name}`)}">Check access</button>` : ''}
        </div>
        <div class="flow-arrow" aria-hidden="true"></div>
        <div class="flow-lane flow-lane-pods">
            <span class="flow-lane-label">Pods <span class="flow-lane-count">${esc(podsHead)}</span></span>
            <div class="flow-pods">${pods}</div>
        </div>
    </div>`;
}

// A blocked or partially blocked hop already carries its policy warning; an
// admitted hop under policy says which policy lets the entry point through.
function flowEntryPolicyHtml(svc) {
    const policy = svc.entryPolicy;
    if (!policy || policy.verdict !== 'allowed') return '';
    const names = (policy.policies ?? []).join(', ');
    return `<p class="flow-policy-ok">NetworkPolicy${names ? ` ${esc(names)}` : ''} admits the ${esc(policy.source)}</p>`;
}

function wireFlowChecks(box) {
    box.querySelectorAll('[data-check-service]').forEach((button) => {
        button.addEventListener('click', (event) => {
            event.stopPropagation();
            openTrafficCheckModal({ destinationKind: 'Service', destination: button.dataset.checkService });
        });
    });
}

function flowPodPill(p) {
    const state = p.isError ? 'err' : (p.isReady ? 'ok' : 'warn');
    const { ingress: ingressPolicies = [], egress: egressPolicies = [] } = podPoliciesFor(p);
    const details = [`${p.status} · ${p.ready}${p.node ? ` · node ${p.node}` : ''}${p.ip ? ` · ${p.ip}` : ''}`];
    if (ingressPolicies.length) details.push(`Ingress isolated by: ${ingressPolicies.join(', ')}`);
    if (egressPolicies.length) details.push(`Egress isolated by: ${egressPolicies.join(', ')}`);
    return `<button type="button" class="flow-pod flow-pod-${state}"
                    data-kind="Pod" data-namespace="${esc(p.namespace)}" data-name="${esc(p.name)}"
                    title="${esc(details.join('\n'))}">
        <span class="flow-dot"></span>
        <span class="flow-pod-name">${esc(p.name)}</span>
        <span class="flow-pod-meta">${esc(p.status)} · ${esc(p.ready)}${ingressPolicies.length ? '<span class="flow-pod-policy">isolated</span>' : ''}</span>
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
    for (const p of obj.pods ?? []) {
        const policies = podPoliciesFor(p);
        bits.push(p.name, p.node, p.ip, p.status, ...(policies.ingress ?? []), ...(policies.egress ?? []));
    }
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
$('btn-traffic-check').addEventListener('click', () => openTrafficCheckModal());

// ---- Check traffic A → B: NetworkPolicy evaluation for one connection ----
//
// Check runs as the modal's in-place secondary action, so the verdict, the
// policies behind it and its limitations stay beside the inputs that produced it.

function parseResourceRef(value, fallbackNamespace) {
    const text = String(value || '').trim();
    const slash = text.indexOf('/');
    return slash > 0
        ? { namespace: text.slice(0, slash), name: text.slice(slash + 1) }
        : { namespace: fallbackNamespace, name: text };
}

function trafficSuggestions(flows) {
    const services = [...(flows?.ingresses ?? []).flatMap((entry) => entry.services ?? []), ...(flows?.services ?? [])];
    return {
        services: [...new Set(services.map((s) => `${s.namespace}/${s.name}`))],
        pods: new Set(services.flatMap((s) => s.pods ?? []).map((p) => `${p.namespace}/${p.name}`)),
    };
}

function openTrafficCheckModal(prefill = {}) {
    const suggestions = trafficSuggestions(lastTrafficFlows);
    const fallbackNamespace = currentNamespace || 'default';
    const options = (values) => [...values].map((value) => `<option value="${esc(value)}"></option>`).join('');
    const renderDestinationOptions = () => {
        $('traffic-dst-options').innerHTML = options($('traffic-dst-kind').value === 'Service' ? suggestions.services : suggestions.pods);
    };
    let scope;
    const runCheck = () => {
        if (!isCurrentModalRequest(scope)) return Promise.resolve();
        const source = parseResourceRef($('traffic-src').value, fallbackNamespace);
        const kind = $('traffic-dst-kind').value;
        const destination = parseResourceRef($('traffic-dst').value, source.namespace);
        const portText = $('traffic-port').value.trim();
        const port = portText === '' ? 0 : Number(portText);
        if (!source.name || !destination.name) return Promise.reject('Enter a source Pod and a destination.');
        if (!Number.isInteger(port) || port < 0 || port > 65535) {
            return Promise.reject('Enter a port between 1 and 65535, or leave it empty when the destination declares exactly one.');
        }
        const box = $('traffic-check-result');
        box.innerHTML = '<p class="modal-hint">Evaluating NetworkPolicies…</p>';
        return CheckTrafficPolicy(source.namespace, source.name, kind, destination.namespace, destination.name, port, $('traffic-protocol').value)
            .then((result) => { if (isCurrentModalRequest(scope)) renderTrafficCheck(box, result); })
            .catch((err) => {
                if (isCurrentModalRequest(scope)) box.innerHTML = '';
                throw err;
            });
    };
    scope = openModal({
        title: 'Check traffic A → B',
        eyebrow: 'Network policy',
        description: 'Evaluates the NetworkPolicies on both sides of one new connection.',
        ownerKey: modalOwner('traffic-check', $('cluster-select').value),
        okText: 'Done',
        cancelText: null,
        extraText: 'Check',
        onExtra: runCheck,
        bodyHtml: `<div class="modal-form-grid">
            <label class="field-full">Source Pod
                <input id="traffic-src" class="pf-input no-enter-submit" list="traffic-src-options" placeholder="namespace/pod" autocomplete="off">
            </label>
            <label>Destination kind
                <select id="traffic-dst-kind" class="pf-input"><option value="Service">Service</option><option value="Pod">Pod</option></select>
            </label>
            <label>Destination
                <input id="traffic-dst" class="pf-input no-enter-submit" list="traffic-dst-options" placeholder="namespace/name" autocomplete="off">
            </label>
            <label>Port
                <input id="traffic-port" class="pf-input no-enter-submit" type="number" min="1" max="65535" placeholder="Service or container port">
            </label>
            <label>Protocol
                <select id="traffic-protocol" class="pf-input"><option value="">TCP</option><option value="UDP">UDP</option><option value="SCTP">SCTP</option></select>
            </label>
        </div>
        <datalist id="traffic-src-options">${options(suggestions.pods)}</datalist>
        <datalist id="traffic-dst-options"></datalist>
        <p class="modal-hint">A name without a namespace uses ${esc(fallbackNamespace)}. For a Service, enter the Service port — Kubby maps it to each Pod's target port.</p>
        <div id="traffic-check-result" class="traffic-check-result" aria-live="polite"></div>`,
        onOpen: () => {
            if (prefill.destination) {
                $('traffic-dst-kind').value = prefill.destinationKind ?? 'Service';
                $('traffic-dst').value = prefill.destination;
                requestAnimationFrame(() => $('traffic-src').focus());
            }
            renderDestinationOptions();
            $('traffic-dst-kind').addEventListener('change', renderDestinationOptions);
            // Enter evaluates in place; the global handler would otherwise treat it
            // as Done and close the modal.
            for (const id of ['traffic-src', 'traffic-dst', 'traffic-port']) {
                $(id).addEventListener('keydown', (event) => {
                    if (event.key !== 'Enter') return;
                    event.preventDefault();
                    $('modal-extra').click();
                });
            }
            // Any Pod in scope can be a source, not only the Service endpoints
            // the Traffic snapshot already knows.
            PodsPage(currentNamespace, '', RESOURCE_PAGE_LIMIT)
                .then((page) => {
                    if (!isCurrentModalRequest(scope)) return;
                    for (const pod of page?.pods ?? []) suggestions.pods.add(`${pod.namespace}/${pod.name}`);
                    $('traffic-src-options').innerHTML = options(suggestions.pods);
                    if ($('traffic-dst-kind').value === 'Pod') renderDestinationOptions();
                })
                .catch(() => {});
        },
    });
}

function renderTrafficCheck(box, result) {
    const tone = { allowed: 'ok', blocked: 'err', partial: 'warn' }[result.verdict] ?? 'warn';
    const heading = { allowed: 'Allowed', blocked: 'Blocked', partial: 'Partially allowed' }[result.verdict] ?? 'Cannot tell';
    const side = (label, verdict) => {
        const [badgeClass, badgeText] = !verdict.isolated ? ['status-ok', 'Not isolated']
            : verdict.allowed ? ['status-ok', 'Allowed']
            : ['status-error', 'Blocked'];
        // An allowed isolated side is explained by the policies that allow it; a
        // blocked one by the policies that isolate it.
        const refs = verdict.isolated && verdict.allowed ? verdict.allowing : verdict.selecting;
        const policies = (refs ?? []).map((ref) => `<button type="button" class="chip traffic-policy"
                data-namespace="${esc(ref.namespace)}" data-name="${esc(ref.name)}">${esc(ref.name)}</button>`).join('');
        return `<div class="traffic-side">
            <span class="traffic-side-label">${esc(label)}</span>
            <span><span class="status-badge ${badgeClass}">${badgeText}</span></span>
            <p>${esc(verdict.reason)}</p>
            ${policies ? `<div class="traffic-policies">${policies}</div>` : ''}
        </div>`;
    };
    const targets = (result.targets ?? []).map((target) => `<article class="traffic-target">
        <header class="traffic-target-head">
            <span class="status-badge ${target.allowed ? 'status-ok' : 'status-error'}">${target.allowed ? 'Allowed' : 'Blocked'}</span>
            <span class="mono">${esc(target.namespace)}/${esc(target.pod)}</span>
            <span class="dim mono">${esc([target.ip, target.port].filter(Boolean).join(' · '))}</span>
        </header>
        <div class="traffic-sides">${side('Egress from source', target.egress)}${side('Ingress to destination', target.ingress)}</div>
    </article>`).join('');
    const limitations = (result.limitations ?? []).map((line) => `<li>${esc(line)}</li>`).join('');
    box.innerHTML = `<div class="traffic-verdict traffic-verdict-${tone}" role="status"><strong>${heading}</strong><span>${esc(result.summary)}</span></div>
        ${targets}
        ${limitations ? `<ul class="traffic-limitations">${limitations}</ul>` : ''}`;
    box.querySelectorAll('.traffic-policy').forEach((button) => {
        button.addEventListener('click', () => {
            closeModal();
            openDrawer({ kind: 'NetworkPolicy', namespace: button.dataset.namespace, name: button.dataset.name });
        });
    });
}

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
const logTextView = new IncrementalLogView($('logs-view'), logLines.capacity);
let following = false;
let logStreamSequence = 0;
let activeLogStreamID = '';
let pendingLiveLogLines = [];

// The backend batches lines to avoid one Wails event per line. A stream ID is
// still required because cancellation cannot retract a batch already in flight.
EventsOn('loglines', (batch) => {
    if (!following || !batch || batch.streamId !== activeLogStreamID) return;
    logLines.pushMany(batch.lines);
    pendingLiveLogLines.push(...(batch.lines ?? []));
    logRenderScheduler.request();
});

EventsOn('logerror', (failure) => {
    if (!following || !failure || failure.streamId !== activeLogStreamID) return;
    following = false;
    activeLogStreamID = '';
    $('logs-follow').checked = false;
    pendingLiveLogLines = [];
    logRenderScheduler.cancel();
    logLines.replace([failure.message || 'Log stream ended unexpectedly.']);
    renderLogs();
});

function prepareLogs(scope = activeDrawerScope) {
    if (!scope || !isCurrentDrawerRequest(scope)) return;
    const ref = scope.ref;
    const select = $('logs-container');
    select.innerHTML = '';
    $('logs-follow').checked = false;
    $('logs-follow').disabled = false;
    $('logs-previous').checked = false;
    $('logs-previous').disabled = false;
    $('logs-restart-hint').hidden = true;
    drawerContainerStates = [];
    $('logs-search').value = '';
    logLines.replace(['Loading containers…']);
    renderLogs();
    drawerPodContainers(scope)
        .then((containers) => {
            if (!isCurrentDrawerRequest(scope)) return;
            drawerContainerStates = containers ?? [];
            for (const c of drawerContainerStates) {
                const opt = document.createElement('option');
                opt.value = c.name;
                opt.textContent = c.restartCount ? `${c.name} · ${c.restartCount} restart${c.restartCount === 1 ? '' : 's'}` : c.name;
                select.appendChild(opt);
            }
            // Open on the container most likely to explain a failure: the first
            // one that has restarted.
            const restarted = drawerContainerStates.find((c) => c.hasPrevious);
            if (restarted) select.value = restarted.name;
            renderLogsRestartHint();
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
    const previous = $('logs-previous').checked;
    logLines.replace([previous ? 'Loading logs from before the last restart…' : 'Loading logs…']);
    renderLogs();
    PodLogs(ref.namespace, ref.name, $('logs-container').value, LOG_TAIL_LINES, previous)
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
    pendingLiveLogLines = [];
    const term = $('logs-search').value.trim().toLowerCase();
    const buffered = logLines.toArray();
    const lines = term ? buffered.filter((l) => l.toLowerCase().includes(term)) : buffered;
    const view = $('logs-view');
    const atBottom = view.scrollHeight - view.scrollTop - view.clientHeight < 40;
    logTextView.replace(lines);
    if (following || atBottom) view.scrollTop = view.scrollHeight;
}

function flushLiveLogLines() {
    const lines = pendingLiveLogLines;
    pendingLiveLogLines = [];
    if (!following || lines.length === 0) return;
    if ($('logs-search').value.trim()) {
        renderLogs();
        return;
    }
    logTextView.append(lines);
    const view = $('logs-view');
    view.scrollTop = view.scrollHeight;
}

const logRenderScheduler = createFrameScheduler(flushLiveLogLines);

function startFollow() {
    const ref = drawerRef;
    const scope = activeDrawerScope;
    if (!ref || !ref.isPod) return;
    const streamID = `${requestScopes.drawerOwnerKey(scope)}:${++logStreamSequence}`;
    following = true;
    activeLogStreamID = streamID;
    pendingLiveLogLines = [];
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
    pendingLiveLogLines = [];
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

// A terminated instance has nothing to follow, so Previous is a static read and
// Follow waits until the current instance is selected again.
$('logs-previous').addEventListener('change', (e) => {
    if (e.target.checked) stopFollow();
    $('logs-follow').disabled = e.target.checked;
    renderLogsRestartHint();
    loadStaticLogs();
});

// Container restart state for the open Pod, from the drawer's shared read.
let drawerContainerStates = [];

function selectedContainerState() {
    return drawerContainerStates.find((c) => c.name === $('logs-container').value);
}

// A restarted container's failure is usually written by the instance that
// ended, not the one now running, so say so and offer that instance's logs.
function renderLogsRestartHint() {
    const state = selectedContainerState();
    const previous = $('logs-previous');
    const hint = $('logs-restart-hint');
    previous.disabled = !state?.hasPrevious;
    previous.closest('label').title = state?.hasPrevious
        ? 'Logs of the container instance that ran before its last restart — where a crash was written'
        : 'This container has not restarted, so there is no previous instance to read.';
    if (!state?.hasPrevious) {
        hint.hidden = true;
        return;
    }
    const ended = [state.lastTermination, state.lastTerminationAge ? `${state.lastTerminationAge} ago` : ''].filter(Boolean).join(', ');
    $('logs-restart-text').textContent = previous.checked
        ? `Showing the instance before the last restart${ended ? ` — it ended ${ended}` : ''}.`
        : `${state.name} restarted ${state.restartCount} time${state.restartCount === 1 ? '' : 's'}${ended ? ` · last exit ${ended}` : ''}. The failure is usually in the logs from before the restart.`;
    $('btn-logs-restart-toggle').textContent = previous.checked ? 'Back to current logs' : 'View logs before restart';
    hint.hidden = false;
}

$('btn-logs-restart-toggle').addEventListener('click', () => $('logs-previous').click());

$('btn-logs-copy-command').addEventListener('click', () => {
    const ref = drawerRef;
    if (!ref) return;
    const container = $('logs-container').value;
    const command = ['kubectl', 'logs', ref.name, '-n', ref.namespace, container ? `-c ${container}` : '',
        `--tail=${LOG_TAIL_LINES}`, $('logs-previous').checked ? '--previous' : '', following ? '-f' : '']
        .filter(Boolean).join(' ');
    copyCommand($('btn-logs-copy-command'), command);
});

// kubectl addresses a custom resource as lower-case "kind.group"; Kubby's
// reference is "Kind.group", so only the kind is lower-cased.
function kubectlResource(kind) {
    const [name, ...group] = String(kind).split('.');
    return [name.toLowerCase(), ...group].join('.');
}

function copyCommand(button, command) {
    Promise.resolve(CopyToClipboard(command))
        .then(() => terminalButtonFeedback(button, 'Copied'))
        .catch((err) => showError(errMsg(err)));
}

$('btn-copy-kubectl').addEventListener('click', () => {
    const ref = drawerRef;
    if (!ref) return;
    copyCommand($('btn-copy-kubectl'), `kubectl describe ${kubectlResource(ref.kind)} ${ref.name}${ref.namespace ? ` -n ${ref.namespace}` : ''}`);
});

$('logs-search').addEventListener('input', () => {
    logRenderScheduler.cancel();
    renderLogs();
});

$('btn-logs-reload').addEventListener('click', () => {
    if (following) { StopLogStream(); startFollow(); }
    else loadStaticLogs();
});

$('logs-container').addEventListener('change', () => {
    if ($('logs-previous').checked && !selectedContainerState()?.hasPrevious) {
        $('logs-previous').checked = false;
        $('logs-follow').disabled = false;
    }
    renderLogsRestartHint();
    if (following) { StopLogStream(); startFollow(); }
    else loadStaticLogs();
});

$('btn-logs-download').addEventListener('click', () => {
    const ref = drawerRef;
    if (!ref) return;
    const name = `${ref.name}${$('logs-container').value ? '-' + $('logs-container').value : ''}${$('logs-previous').checked ? '-previous' : ''}.log`;
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
let terminalRuntimePromise = null;
let TerminalClass = null;
let FitAddonClass = null;

function loadTerminalRuntime() {
    if (execTerminal) return Promise.resolve(execTerminal);
    if (!terminalRuntimePromise) {
        terminalRuntimePromise = Promise.all([
            import('@xterm/xterm'),
            import('@xterm/addon-fit'),
        ]).then(([terminalModule, fitModule]) => {
            TerminalClass = terminalModule.Terminal;
            FitAddonClass = fitModule.FitAddon;
            return createTerminal();
        }).catch((err) => {
            terminalRuntimePromise = null;
            throw err;
        });
    }
    return terminalRuntimePromise;
}

function createTerminal() {
    if (execTerminal) return execTerminal;
    execTerminal = new TerminalClass({
        allowProposedApi: false,
        convertEol: false,
        cursorBlink: true,
        cursorStyle: 'bar',
        fontFamily: '"Cascadia Code", "SFMono-Regular", Consolas, "Liberation Mono", monospace',
        fontSize: 13,
        lineHeight: 1.18,
        scrollback: 5000,
        theme: {
            background: '#151b26', foreground: '#d8dee9', cursor: '#8fbcff', cursorAccent: '#151b26',
            selectionBackground: '#4c566a99', black: '#3b4252', red: '#bf616a', green: '#a3be8c',
            yellow: '#ebcb8b', blue: '#81a1c1', magenta: '#b48ead', cyan: '#88c0d0', white: '#e5e9f0',
            brightBlack: '#4c566a', brightRed: '#d06f79', brightGreen: '#b1d196', brightYellow: '#f0d399',
            brightBlue: '#8fafd2', brightMagenta: '#c19acb', brightCyan: '#9ad3df', brightWhite: '#eceff4',
        },
    });
    execFitAddon = new FitAddonClass();
    execTerminal.loadAddon(execFitAddon);
    execTerminal.open($('term-surface'));
    execTerminal.onData((data) => {
        if (execConnected) execWriter(data);
    });
    execTerminal.onSelectionChange(() => {
        $('btn-term-copy').disabled = !execTerminal?.hasSelection();
    });
    execTerminal.attachCustomKeyEventHandler((event) => {
        if (event.type !== 'keydown') return true;
        const key = event.key.toLowerCase();
        const terminalShortcut = event.metaKey || (event.ctrlKey && event.shiftKey);
        if (terminalShortcut && key === 'c' && execTerminal?.hasSelection()) {
            copyTerminalSelection();
            return false;
        }
        if (terminalShortcut && key === 'v') {
            pasteTerminalClipboard();
            return false;
        }
        return true;
    });
    execTerminal.onResize(({ cols, rows }) => {
        const size = validTerminalSize(cols, rows);
        if (execConnected && size) ExecResize(size.cols, size.rows);
    });
    execResizeObserver = new ResizeObserver(() => {
        if (!$('dpanel-terminal').hidden) requestAnimationFrame(fitTerminal);
    });
    execResizeObserver.observe($('term-surface'));
    $('term-surface').addEventListener('contextmenu', (event) => {
        event.preventDefault();
        if (execTerminal?.hasSelection()) copyTerminalSelection();
        else pasteTerminalClipboard();
    });
    return execTerminal;
}

function terminalButtonFeedback(button, label) {
    const original = button.textContent;
    button.textContent = label;
    button.dataset.feedback = 'success';
    window.setTimeout(() => {
        if (!button.isConnected) return;
        button.textContent = original;
        delete button.dataset.feedback;
    }, 1200);
}

function copyTerminalSelection() {
    const selection = execTerminal?.getSelection() ?? '';
    if (!selection) return Promise.resolve(false);
    const button = $('btn-term-copy');
    return Promise.resolve(CopyToClipboard(selection))
        .then(() => {
            terminalButtonFeedback(button, 'Copied');
            execTerminal?.focus();
            return true;
        })
        .catch((err) => {
            setTerminalStatus('error', `Copy failed: ${errMsg(err)}`);
            return false;
        });
}

function pasteTerminalClipboard() {
    if (!execConnected || !execTerminal) return Promise.resolve(false);
    const sessionID = execSessionID;
    const button = $('btn-term-paste');
    return Promise.resolve(ClipboardGetText())
        .then((text) => {
            if (!execConnected || execSessionID !== sessionID || !text) return false;
            execTerminal.paste(String(text));
            execTerminal.focus();
            terminalButtonFeedback(button, 'Pasted');
            return true;
        })
        .catch((err) => {
            if (execSessionID === sessionID) setTerminalStatus('error', `Paste failed: ${errMsg(err)}`);
            return false;
        });
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

function setTerminalPlaceholder(title, copy) {
    $('term-placeholder-title').textContent = title;
    $('term-placeholder-copy').textContent = copy;
}

function resetTerminalUI({ clear = false, preserveOutput = false } = {}) {
    execConnected = false;
    execAcceptOutput = false;
    execSessionID = '';
    execInputEpoch++;
    execWriter = () => Promise.resolve();
    $('btn-term-start').hidden = false;
    $('btn-term-start').disabled = false;
    $('btn-term-start').textContent = 'Retry';
    $('btn-term-stop').hidden = true;
    $('btn-term-copy').disabled = true;
    $('btn-term-paste').disabled = true;
    $('term-container').disabled = false;
    $('term-shell').disabled = false;
    $('term-session-label').textContent = 'Not connected';
    $('term-surface').setAttribute('aria-label', 'Interactive container terminal');
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
    loadTerminalRuntime().then((terminal) => {
        if (execAcceptOutput && event?.sessionId === execSessionID) terminal.write(String(event.data ?? ''));
    }).catch(() => {});
});
EventsOn('exec-closed', (event) => {
    if (!execAcceptOutput || event?.sessionId !== execSessionID) return;
    const msg = event.message ?? '';
    const suffix = msg ? `Session ended · ${msg}` : 'Session ended';
    execHasOutput = true;
    loadTerminalRuntime().then((terminal) => {
        if (!execAcceptOutput || event?.sessionId !== execSessionID) return;
        terminal.write(`\r\n\x1b[90m[${suffix}]\x1b[0m\r\n`);
        resetTerminalUI({ preserveOutput: true });
        setTerminalStatus('', 'Session ended');
    }).catch(() => {});
});

function prepareTerminal(scope = activeDrawerScope) {
    if (!scope || !isCurrentDrawerRequest(scope)) return;
    const ref = scope.ref;
    const select = $('term-container');
    select.innerHTML = '';
    resetTerminalUI({ clear: true });
    $('btn-term-start').disabled = true;
    $('btn-term-start').textContent = 'Attaching…';
    setTerminalStatus('connecting', 'Discovering container…');
    setTerminalPlaceholder('Preparing Kubby Shell', 'Finding the container and best available shell…');
    Promise.all([drawerPodContainers(scope), loadTerminalRuntime()])
        .then(([containers]) => {
            if (!isCurrentDrawerRequest(scope)) return;
            for (const c of containers ?? []) {
                const opt = document.createElement('option');
                opt.value = c.name;
                opt.textContent = c.name;
                select.appendChild(opt);
            }
            if (!select.options.length) {
                resetTerminalUI();
                setTerminalStatus('error', 'This Pod has no regular containers.');
                setTerminalPlaceholder('No container available', 'Kubby Shell can attach only to a regular running container.');
                return;
            }
            requestAnimationFrame(() => startTerminal(scope));
        })
        .catch((err) => {
            if (!isCurrentDrawerRequest(scope)) return;
            resetTerminalUI();
            setTerminalStatus('error', errMsg(err));
            setTerminalPlaceholder('Containers unavailable', 'Retry after the Pod becomes available or check your access.');
        });
}

async function startTerminal(scope = activeDrawerScope) {
    if (!scope || !isCurrentDrawerRequest(scope) || !scope.ref.isPod) return;
    let terminal;
    try {
        terminal = await loadTerminalRuntime();
    } catch (err) {
        if (isCurrentDrawerRequest(scope)) {
            resetTerminalUI();
            setTerminalStatus('error', `Terminal failed to load: ${errMsg(err)}`);
        }
        return;
    }
    if (!isCurrentDrawerRequest(scope)) return;
    const ref = scope.ref;
    const container = $('term-container').value;
    const shell = $('term-shell').value;
    if (!container) return;
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
    $('btn-term-start').textContent = 'Attaching…';
    $('btn-term-stop').hidden = true;
    $('term-container').disabled = true;
    $('term-shell').disabled = true;
    $('term-session-label').textContent = `${ref.namespace || 'default'} / ${ref.name} / ${container}`;
    $('term-surface').setAttribute('aria-label', `Terminal for ${ref.name}, container ${container}`);
    setTerminalStatus('connecting', shell === 'auto' ? 'Finding shell and attaching…' : `Attaching via ${shell}…`);
    StartExec(sessionID, ref.namespace, ref.name, container, shell, size.cols, size.rows)
        .then((resolvedShell) => {
            if (!isCurrentDrawerRequest(scope) || inputEpoch !== execInputEpoch) { StopExec(); return; }
            const activeShell = typeof resolvedShell === 'string' && resolvedShell ? resolvedShell : shell;
            execConnected = true;
            execWriter = createSerialWriter(
                (data) => inputEpoch === execInputEpoch ? ExecWrite(data) : Promise.resolve(),
                (err) => {
                    if (inputEpoch === execInputEpoch) setTerminalStatus('error', `Input failed: ${errMsg(err)}`);
                },
            );
            setTerminalStatus('connected', `Attached · ${activeShell}`);
            $('btn-term-start').textContent = 'Reconnect';
            $('btn-term-start').disabled = false;
            $('btn-term-stop').hidden = false;
            $('btn-term-paste').disabled = false;
            $('term-container').disabled = false;
            $('term-shell').disabled = false;
            ExecResize(terminal.cols, terminal.rows);
            terminal.focus();
        })
        .catch((err) => {
            if (!isCurrentDrawerRequest(scope) || inputEpoch !== execInputEpoch) return;
            resetTerminalUI({ preserveOutput: true });
            setTerminalStatus('error', errMsg(err));
            setTerminalPlaceholder('Shell unavailable', 'This may be a distroless image. Choose an explicit shell path or retry after the container is ready.');
        });
}

$('btn-term-start').addEventListener('click', () => startTerminal());
$('btn-term-stop').addEventListener('click', stopExec);
$('btn-term-copy').addEventListener('click', copyTerminalSelection);
$('btn-term-paste').addEventListener('click', pasteTerminalClipboard);
$('btn-term-clear').addEventListener('click', () => execTerminal?.clear());
$('term-container').addEventListener('change', () => startTerminal());
$('term-shell').addEventListener('change', () => startTerminal());

function stopExec() {
    const hadSession = execConnected || execAcceptOutput;
    resetTerminalUI({ preserveOutput: hadSession });
    if (hadSession) StopExec();
    setTerminalStatus('', 'Disconnected by user');
    setTerminalPlaceholder('Session disconnected', 'Select Retry whenever you want to attach again.');
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
			if (!shouldRetainStartedForward({
				drawerStillOwnsRequest,
				keepRunning,
				connectionCurrent: requestScopes.isCurrentConnection(connectionToken),
				resultConnectionMatches: info.connectionId === $('cluster-select').value,
			})) {
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
    $('btn-port-forwards').hidden = count === 0;
    if (count === 0) setPortForwardManagerOpen(false);
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
let modalReturnFocus = null;
let dialogReturnFocus = null;

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
    ownerKey = `modal\0${title}`, okDisabled = false, wide = false, size = 'standard',
    eyebrow = '', description = '', cancelText = 'Cancel', okStyle = 'primary' }) {
    // A modal can transition directly into another modal (chart search → install).
    // Dispose the old content before reusing its IDs and invalidate every callback
    // that still belongs to it.
    const modal = $('modal');
    if (modal.hidden) modalReturnFocus = document.activeElement;
    disposeModalContent();
    const scope = requestScopes.openModal(ownerKey);
    activeModalScope = scope;
    $('modal-title').textContent = title;
    const eyebrowEl = $('modal-eyebrow');
    eyebrowEl.textContent = eyebrow;
    eyebrowEl.hidden = !eyebrow;
    const descriptionEl = $('modal-description');
    descriptionEl.textContent = description;
    descriptionEl.hidden = !description;
    if (description) modal.setAttribute('aria-describedby', 'modal-description');
    else modal.removeAttribute('aria-describedby');
    modal.classList.remove('modal-compact', 'modal-wide', 'modal-editor');
    const resolvedSize = wide ? 'wide' : size;
    if (resolvedSize !== 'standard') modal.classList.add(`modal-${resolvedSize}`);
    $('modal-body').innerHTML = bodyHtml;
    const ok = $('modal-ok');
    ok.textContent = okText;
    ok.disabled = okDisabled;
    ok.className = `btn btn-${okStyle}`;
    const cancel = $('modal-cancel');
    cancel.hidden = cancelText === null;
    if (cancelText !== null) cancel.textContent = cancelText;
    $('modal-error').hidden = true;
    modalOnOk = onOk;
    // An optional secondary action (Import YAML's Preview). Hidden unless given.
    modalOnExtra = onExtra;
    const extra = $('modal-extra');
    extra.hidden = !onExtra;
    extra.disabled = false;
    if (onExtra) extra.textContent = extraText || 'More';
    $('modal-backdrop').hidden = false;
    modal.hidden = false;
    $('sidebar').inert = true;
    document.querySelector('.main').inert = true;
    if (!$('drawer').hidden) $('drawer').inert = true;
    if (onOpen) onOpen();
    setTimeout(() => {
        if (!isCurrentModalRequest(scope) || modal.contains(document.activeElement)) return;
        const target = modal.querySelector('[autofocus]')
            || modal.querySelector('#modal-body input:not([type="hidden"]), #modal-body select, #modal-body textarea, #modal-body button:not([disabled])')
            || $('modal-close');
        target?.focus();
    }, 0);
    return scope;
}

function closeModal(scope = null) {
    // Async completion from A must never close a newer modal B.
    if (scope && activeModalScope !== scope) return;
    requestScopes.closeModal();
    activeModalScope = null;
    $('modal').hidden = true;
    $('modal').classList.remove('modal-compact', 'modal-wide', 'modal-editor');
    $('modal-backdrop').hidden = true;
    $('drawer').inert = false;
    if ($('drawer').hidden && $('palette').hidden) {
        $('sidebar').inert = false;
        document.querySelector('.main').inert = false;
    }
    disposeModalContent();
    $('modal-extra').hidden = true;
    $('modal-extra').disabled = false;
    $('modal-ok').disabled = false;
    modalOnOk = null;
    modalOnExtra = null;
    const target = modalReturnFocus;
    modalReturnFocus = null;
    if (target?.isConnected) setTimeout(() => target.focus(), 0);
}

function modalError(msg, scope = activeModalScope) {
    if (!isCurrentModalRequest(scope)) return;
    const el = $('modal-error');
    el.querySelector('p').textContent = msg;
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

function openDialog({ title, message, icon, okText, cancelText, danger, tone = '' }) {
    return new Promise((resolve) => {
        dialogReturnFocus = document.activeElement;
        $('dialog-title').textContent = title;
        $('dialog-icon').textContent = icon;
        const resolvedTone = tone || (danger ? 'danger' : title === 'Error' ? 'error' : icon === '✅' ? 'success' : 'default');
        $('dialog').className = `dialog dialog-${resolvedTone}`;
        $('dialog-eyebrow').textContent = resolvedTone === 'danger' ? 'Confirmation required'
            : resolvedTone === 'error' ? 'Something went wrong'
            : resolvedTone === 'success' ? 'Action completed' : (cancelText === null ? 'Notice' : 'Please confirm');
        $('dialog-message').textContent = message; // textContent → newlines preserved via CSS, no HTML injection
        $('dialog-ok').textContent = okText;
        const cancelBtn = $('dialog-cancel');
        cancelBtn.hidden = cancelText === null;
        if (cancelText !== null) cancelBtn.textContent = cancelText;
        $('dialog-ok').classList.toggle('btn-danger', !!danger);
        $('dialog-backdrop').hidden = false;
        $('dialog').hidden = false;
        dialogResolve = resolve;
        setTimeout(() => (danger && cancelText !== null ? cancelBtn : $('dialog-ok')).focus(), 0);
    });
}

function closeDialog(result) {
    if ($('dialog').hidden) return;
    $('dialog').hidden = true;
    $('dialog-backdrop').hidden = true;
    $('dialog-ok').classList.remove('btn-danger');
    $('dialog').className = 'dialog';
    const resolve = dialogResolve;
    dialogResolve = null;
    if (resolve) resolve(result);
    const target = dialogReturnFocus;
    dialogReturnFocus = null;
    if (target?.isConnected) setTimeout(() => target.focus(), 0);
}

function trapOverlayFocus(container, event) {
    const items = [...container.querySelectorAll('button:not([disabled]):not([hidden]), input:not([disabled]):not([type="hidden"]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])')]
        .filter((element) => element.getClientRects().length > 0);
    if (items.length === 0) return;
    const first = items[0];
    const last = items[items.length - 1];
    if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last.focus(); }
    else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus(); }
}

// A simple notice (one OK button). Returns a promise that resolves when dismissed.
function showAlert(message, { title = 'Notice', icon = 'ℹ️' } = {}) {
    return openDialog({ title, message, icon, okText: 'OK', cancelText: null });
}
// An error notice with warning styling.
function showError(message, title = 'Error') {
    recordError(message);
    return openDialog({ title, message, icon: '⚠️', okText: 'OK', cancelText: null, tone: 'error' });
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
	const set = accessPeek(kind, ns);
	if (!allowed(set, 'create')) {
		showError(denyReason(kind, ns, 'create'));
		return;
	}
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
    const connectionID = $('cluster-select').value;
    openModal({
        title,
        eyebrow: 'Kubernetes manifest',
        description: 'Review the YAML and preview server-side changes before applying them to the cluster.',
        size: 'editor',
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
            return PlanApplyPermissions(v)
                .then((plan) => {
                    ensurePermissionPlan(plan);
                    return ApplyYAMLOwned(connectionID, v);
                })
                .then(applyReport, (err) => {
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
        permissionPlanHTML(d?.permissions),
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
        case 'NetworkPolicy':
            return `apiVersion: networking.k8s.io/v1\nkind: NetworkPolicy\nmetadata:\n  name: allow-from-frontend${nsLine}\nspec:\n  podSelector:\n    matchLabels:\n      app: my-app\n  policyTypes:\n    - Ingress\n  ingress:\n    - from:\n        - podSelector:\n            matchLabels:\n              app: frontend\n      ports:\n        - protocol: TCP\n          port: 8080\n`;
        case 'HorizontalPodAutoscaler':
            return `apiVersion: autoscaling/v2\nkind: HorizontalPodAutoscaler\nmetadata:\n  name: my-hpa${nsLine}\nspec:\n  scaleTargetRef:\n    apiVersion: apps/v1\n    kind: Deployment\n    name: my-deployment\n  minReplicas: 1\n  maxReplicas: 5\n  metrics:\n    - type: Resource\n      resource:\n        name: cpu\n        target:\n          type: Utilization\n          averageUtilization: 70\n`;
        case 'PodDisruptionBudget':
            return `apiVersion: policy/v1\nkind: PodDisruptionBudget\nmetadata:\n  name: my-pdb${nsLine}\nspec:\n  maxUnavailable: 1\n  selector:\n    matchLabels:\n      app: my-app\n`;
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
        else if (e.key === 'Enter') { e.preventDefault(); closeDialog(e.target !== $('dialog-cancel')); }
        else if (e.key === 'Tab') trapOverlayFocus($('dialog'), e);
        return;
    }
    // A CodeMirror editor handles some of these keys itself — Escape closes its
    // search panel and releases Tab back to focus navigation. Its listener sits on
    // an element inside document, so it has already run; if it acted on the key it
    // called preventDefault, and this handler must not act on the same press.
    if (e.defaultPrevented) return;
    // The command palette has its own complete keyboard model below.
    if (!$('palette').hidden) return;

    if (e.key === 'Escape') {
        if (!$('pf-manager').hidden) { closePortForwardManager(); return; }
        if (!$('modal').hidden) { closeModal(); return; }
        if (!$('drawer').hidden) closeDrawer();
        return;
    }
    if (e.key === 'Tab' && !$('modal').hidden) {
        trapOverlayFocus($('modal'), e);
        return;
    }
    if (e.key === 'Tab' && !$('drawer').hidden) {
        trapOverlayFocus($('drawer'), e);
        return;
    }
    // Enter submits the modal only when focus is NOT in something that owns Enter:
    // a textarea, an input that opts out (.no-enter-submit), or the YAML editor —
    // whose editable element is a contenteditable div, not a textarea, so checking
    // the tag name alone would let Enter apply a half-typed manifest.
    if (e.key === 'Enter' && !$('modal').hidden
        && e.target.tagName !== 'TEXTAREA'
        && !e.target.closest('button')
        && !(e.target.closest && e.target.closest('.cm-editor'))
        && !e.target.classList.contains('no-enter-submit')) {
        e.preventDefault();
        submitModal();
    }
});

// ============ Health checks: webhooks, certificates, stuck deletions ============
//
// One read-only snapshot (ClusterChecks) behind three tabs. Problems sort first
// in every list, objects open their drawer, and a stuck deletion offers the
// kubectl command that would force it — Kubby never runs it.

const CHECK_TABS = {
    webhooks: { label: 'Admission webhooks', noun: 'webhook', empty: 'No admission webhooks are configured.' },
    certificates: { label: 'Certificates', noun: 'certificate', empty: 'No certificates were found in this scope.' },
    stuck: { label: 'Stuck deletions', noun: 'terminating object', empty: 'Nothing is being deleted in this scope.' },
};
let checksReport = null;
let checksTab = 'webhooks';

function loadChecks(scope) {
    return ClusterChecks(scope.namespace)
        .then((report) => {
            if (!isCurrentViewRequest(scope)) return;
            checksReport = report;
            renderChecks();
        })
        .catch((err) => {
            if (!isCurrentViewRequest(scope)) return;
            checksReport = null;
            $('checks-summary').innerHTML = '';
            for (const tab of Object.keys(CHECK_TABS)) $(`checks-${tab}-list`).innerHTML = '';
            showDashError(err);
        });
}

function checkSections(report) {
    return {
        webhooks: { section: report?.webhooks ?? {}, items: report?.webhooks?.webhooks ?? [], render: webhookCheckHtml },
        certificates: { section: report?.certificates ?? {}, items: report?.certificates?.certificates ?? [], render: certificateCheckHtml },
        stuck: { section: report?.stuck ?? {}, items: report?.stuck?.objects ?? [], render: stuckObjectHtml },
    };
}

function renderChecks() {
    const report = checksReport;
    const sections = checkSections(report);
    $('checks-summary').innerHTML = Object.entries(CHECK_TABS).map(([tab, meta]) => {
        const { section, items } = sections[tab];
        const critical = section.critical ?? 0;
        const warning = section.warning ?? 0;
        return `<button type="button" class="flow-stat checks-stat${critical ? ' flow-stat-bad' : ''}" data-check-tab="${tab}">
            <span class="flow-stat-num">${critical + warning}</span>
            <span class="flow-stat-label">${esc(meta.label)}</span>
            <span class="flow-stat-hint">${critical} critical · ${warning} warning · ${esc(countNoun(items.length, meta.noun))}</span>
        </button>`;
    }).join('');
    $('checks-summary').querySelectorAll('[data-check-tab]').forEach((tile) => {
        tile.addEventListener('click', () => setChecksTab(tile.dataset.checkTab));
    });

    for (const [tab, meta] of Object.entries(CHECK_TABS)) {
        const { section, items, render } = sections[tab];
        const problems = (section.critical ?? 0) + (section.warning ?? 0);
        $(`checks-tab-${tab}`).textContent = problems ? `${meta.label} (${problems})` : meta.label;
        const list = $(`checks-${tab}-list`);
        list.innerHTML = items.map(render).join('');
        list.querySelectorAll('[data-open-kind]').forEach((button) => {
            button.addEventListener('click', () => openDrawer({
                kind: button.dataset.openKind,
                namespace: button.dataset.openNamespace || '',
                name: button.dataset.openName,
                isPod: button.dataset.openKind === 'Pod',
            }));
        });
        list.querySelectorAll('[data-copy-command]').forEach((button) => {
            button.addEventListener('click', () => copyCommand(button, button.dataset.copyCommand));
        });
    }

    const stuck = report?.stuck;
    $('checks-stuck-scanned').textContent = stuck
        ? `${countNoun(stuck.scanned ?? 0, 'resource type')} scanned${stuck.failed ? ` · ${stuck.failed} could not be listed` : ''}`
        : '';
    const warnings = [...(report?.webhooks?.warnings ?? []), ...(report?.certificates?.warnings ?? []), ...(report?.stuck?.warnings ?? [])];
    $('checks-warnings').textContent = warnings.join(' · ');
    $('checks-warnings').hidden = warnings.length === 0;
    $('checks-updated').textContent = report?.checkedAt ? `Checked ${new Date(report.checkedAt).toLocaleTimeString()}` : '';
    setChecksTab(checksTab);
}

function setChecksTab(tab) {
    if (!CHECK_TABS[tab]) return;
    checksTab = tab;
    for (const name of Object.keys(CHECK_TABS)) {
        const active = name === tab;
        const button = $(`checks-tab-${name}`);
        button.classList.toggle('active', active);
        button.setAttribute('aria-selected', String(active));
        button.tabIndex = active ? 0 : -1;
        $(`checks-panel-${name}`).hidden = !active;
    }
    applyChecksFilter();
}

function applyChecksFilter() {
    const onlyProblems = $('checks-only-problems').checked;
    for (const [tab, meta] of Object.entries(CHECK_TABS)) {
        const items = [...$(`checks-${tab}-list`).querySelectorAll('.checks-item')];
        let shown = 0;
        for (const item of items) {
            item.hidden = onlyProblems && item.dataset.problem !== '1';
            if (!item.hidden) shown++;
        }
        const empty = $(`checks-${tab}-empty`);
        empty.textContent = items.length === 0 ? meta.empty : 'No problems found — every item passed its checks.';
        empty.hidden = shown > 0 || !checksReport;
    }
}

document.querySelectorAll('[role="tab"][data-check-tab]').forEach((tab) => {
    tab.addEventListener('click', () => setChecksTab(tab.dataset.checkTab));
    tab.addEventListener('keydown', (event) => {
        if (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight') return;
        event.preventDefault();
        const tabs = Object.keys(CHECK_TABS);
        const next = tabs[(tabs.indexOf(checksTab) + (event.key === 'ArrowRight' ? 1 : -1) + tabs.length) % tabs.length];
        setChecksTab(next);
        $(`checks-tab-${next}`).focus();
    });
});
$('checks-only-problems').addEventListener('change', applyChecksFilter);

function checkSeverityBadge(severity) {
    const tone = { critical: 'status-error', warning: 'status-warn', info: 'status-info', ok: 'status-ok' }[severity] ?? 'status-info';
    const label = { critical: 'Critical', warning: 'Warning', info: 'Info', ok: 'OK' }[severity] ?? severity;
    return `<span class="status-badge ${tone}">${esc(label)}</span>`;
}

function checkFindingsHtml(findings) {
    if (!findings?.length) return '';
    return `<ul class="checks-findings">${findings.map((finding) => `<li class="checks-finding checks-finding-${esc(finding.severity)}">
        <strong>${esc(finding.title)}</strong>${finding.detail ? `<span>${esc(finding.detail)}</span>` : ''}
    </li>`).join('')}</ul>`;
}

function checkObjectButton(kind, namespace, name, label) {
    if (!kind || !name) return `<span class="checks-object-text">${esc(label)}</span>`;
    return `<button type="button" class="checks-object" data-open-kind="${esc(kind)}" data-open-namespace="${esc(namespace ?? '')}" data-open-name="${esc(name)}">${esc(label)}</button>`;
}

function checkChips(values) {
    return values.filter(Boolean).map((value) => `<span class="chip">${esc(value)}</span>`).join('');
}

function checkItem(severity, headHtml, bodyHtml) {
    const problem = severity === 'critical' || severity === 'warning';
    return `<article class="checks-item checks-item-${esc(severity)}" data-problem="${problem ? '1' : '0'}">
        <header class="checks-item-head">${checkSeverityBadge(severity)}${headHtml}</header>
        ${bodyHtml}
    </article>`;
}

function webhookCheckHtml(hook) {
    const target = hook.serviceName
        ? checkObjectButton('Service', hook.serviceNamespace, hook.serviceName, hook.target)
        : `<span>${esc(hook.target || '—')}</span>`;
    return checkItem(hook.severity,
        `${checkObjectButton(hook.configKind, '', hook.configuration, hook.configuration)}<span class="checks-item-sub mono">${esc(hook.webhook)}</span>`,
        `<div class="checks-meta">${checkChips([
            `failurePolicy ${hook.failurePolicy}`, `timeout ${hook.timeoutSeconds}s`, hook.scope,
            hook.readyEndpoints >= 0 ? countNoun(hook.readyEndpoints, 'ready endpoint') : '',
        ])}</div>
        <p class="checks-line"><span class="checks-label">Target</span>${target}</p>
        <p class="checks-line"><span class="checks-label">Rules</span><span class="mono">${esc(hook.rules)}</span></p>
        ${checkFindingsHtml(hook.findings)}`);
}

function certificateCheckHtml(cert) {
    const where = cert.namespace ? `${cert.namespace}/${cert.name}` : cert.name;
    return checkItem(cert.severity,
        `${checkObjectButton(cert.kind, cert.namespace, cert.name, where || cert.source)}<span class="checks-item-sub">${esc(cert.source)}</span>`,
        `<div class="checks-meta">${checkChips([
            cert.hasCertificate ? `expires ${cert.expires}` : '',
            cert.subject ? `CN ${cert.subject}` : '',
            cert.issuer && cert.issuer !== cert.subject ? `issuer ${cert.issuer}` : '',
            cert.managedBy,
        ])}</div>
        ${cert.dnsNames?.length ? `<p class="checks-line"><span class="checks-label">DNS names</span><span class="mono">${esc(cert.dnsNames.join(', '))}</span></p>` : ''}
        ${cert.usedBy?.length ? `<p class="checks-line"><span class="checks-label">Used by</span><span>${esc(cert.usedBy.join(', '))}</span></p>` : ''}
        ${cert.notAfter ? `<p class="checks-line"><span class="checks-label">Not after</span><span class="mono">${esc(cert.notAfter)}</span></p>` : ''}
        ${checkFindingsHtml(cert.findings)}`);
}

function stuckObjectHtml(object) {
    const where = object.namespace ? `${object.namespace}/${object.name}` : object.name;
    return checkItem(object.severity,
        `${checkObjectButton(object.refKind, object.namespace, object.name, where)}<span class="checks-item-sub">${esc(object.kind)}</span>`,
        `<div class="checks-meta">${checkChips([`${object.stuck ? 'Terminating' : 'Deleting'} for ${object.terminating}`])}${
            (object.finalizers ?? []).map((finalizer) => `<span class="chip mono">${esc(finalizer)}</span>`).join('')}</div>
        ${checkFindingsHtml(object.findings)}
        ${object.command ? `<div class="checks-command"><code class="mono">${esc(object.command)}</code>
            <button type="button" class="btn btn-secondary btn-sm" data-copy-command="${esc(object.command)}">Copy</button></div>` : ''}
        ${object.commandNote ? `<p class="checks-note">${esc(object.commandNote)}</p>` : ''}`);
}

// ============ Cleanup: what nothing references any more ============
//
// One read-only scan (ClusterHygiene) shown as groups. Every group states what
// a reference scan cannot see, next to its items — a cleanup list without its
// caveat is how someone deletes a ConfigMap an operator was reading by name.
// Kubby never deletes anything here: the item carries the command to copy.

let hygieneReport = null;

function loadHygiene(scope) {
    return ClusterHygiene(scope.namespace)
        .then((report) => {
            if (!isCurrentViewRequest(scope)) return;
            hygieneReport = report;
            renderHygiene();
        })
        .catch((err) => {
            if (!isCurrentViewRequest(scope)) return;
            hygieneReport = null;
            $('hygiene-summary').innerHTML = '';
            $('hygiene-groups').innerHTML = '';
            $('hygiene-empty').hidden = true;
            showDashError(err);
        });
}

function renderHygiene() {
    const report = hygieneReport;
    const groups = report?.groups ?? [];
    $('hygiene-summary').innerHTML = groups.map((group) => {
        const warnings = (group.items ?? []).filter((item) => item.severity === 'warning' || item.severity === 'critical').length;
        return `<button type="button" class="flow-stat checks-stat${warnings ? ' flow-stat-bad' : ''}" data-hygiene-jump="${esc(group.category)}">
            <span class="flow-stat-num">${group.count ?? 0}</span>
            <span class="flow-stat-label">${esc(group.title)}</span>
            <span class="flow-stat-hint">${warnings ? `${warnings} worth acting on` : 'nothing urgent'}</span>
        </button>`;
    }).join('');
    $('hygiene-summary').querySelectorAll('[data-hygiene-jump]').forEach((tile) => {
        tile.addEventListener('click', () => {
            const section = document.getElementById(`hygiene-group-${tile.dataset.hygieneJump}`);
            section?.scrollIntoView({ block: 'start', behavior: 'smooth' });
            section?.querySelector('.checks-item')?.classList.add('hygiene-jumped');
        });
    });

    $('hygiene-groups').innerHTML = groups.map(hygieneGroupHtml).join('');
    $('hygiene-groups').querySelectorAll('[data-open-kind]').forEach((button) => {
        button.addEventListener('click', () => openDrawer({
            kind: button.dataset.openKind,
            namespace: button.dataset.openNamespace || '',
            name: button.dataset.openName,
            isPod: button.dataset.openKind === 'Pod',
        }));
    });
    $('hygiene-groups').querySelectorAll('[data-copy-command]').forEach((button) => {
        button.addEventListener('click', () => copyCommand(button, button.dataset.copyCommand));
    });

    const warnings = report?.warnings ?? [];
    $('hygiene-warnings').textContent = warnings.join(' · ');
    $('hygiene-warnings').hidden = warnings.length === 0;
    $('hygiene-updated').textContent = report?.checkedAt
        ? `${countNoun(report.total ?? 0, 'item')} · scanned ${new Date(report.checkedAt).toLocaleTimeString()}`
        : '';
    applyHygieneFilter();
}

function hygieneGroupHtml(group) {
    const items = group.items ?? [];
    return `<section class="hygiene-group" id="hygiene-group-${esc(group.category)}" data-hygiene-group="${esc(group.category)}">
        <header class="hygiene-group-head">
            <h3>${esc(group.title)} <span class="hygiene-count">${group.count ?? 0}</span></h3>
            <p class="checks-intro">${esc(group.summary)}</p>
        </header>
        ${group.warning ? `<p class="checks-note hygiene-group-warning">${esc(group.warning)}</p>` : ''}
        <div class="checks-list">${items.map(hygieneItemHtml).join('')}</div>
        <p class="empty-inline hygiene-group-empty" hidden></p>
        <p class="checks-note hygiene-caveat"><strong>Before you delete:</strong> ${esc(group.caveat)}</p>
    </section>`;
}

function hygieneItemHtml(item) {
    const where = item.namespace ? `${item.namespace}/${item.name}` : item.name;
    const search = `${item.namespace ?? ''} ${item.name} ${item.title}`.toLowerCase();
    const problem = item.severity === 'critical' || item.severity === 'warning';
    return `<article class="checks-item checks-item-${esc(item.severity)}" data-problem="${problem ? '1' : '0'}" data-search="${esc(search)}">
        <header class="checks-item-head">${checkSeverityBadge(item.severity)}
            ${checkObjectButton(item.kind, item.namespace, item.name, where)}
            <span class="checks-item-sub">${esc(item.kind)}${item.age ? ` · ${esc(item.age)} old` : ''}</span>
        </header>
        <div class="checks-meta">${checkChips(item.chips ?? [])}</div>
        <p class="checks-line"><span class="checks-label">Why</span><span>${esc(item.title)} — ${esc(item.detail)}</span></p>
        ${item.command ? `<div class="checks-command"><code class="mono">${esc(item.command)}</code>
            <button type="button" class="btn btn-secondary btn-sm" data-copy-command="${esc(item.command)}">Copy</button></div>` : ''}
    </article>`;
}

function applyHygieneFilter() {
    const onlyProblems = $('hygiene-only-problems').checked;
    const term = $('hygiene-filter').value.trim().toLowerCase();
    let shown = 0;
    for (const section of $('hygiene-groups').querySelectorAll('.hygiene-group')) {
        const items = [...section.querySelectorAll('.checks-item')];
        let visible = 0;
        for (const item of items) {
            const hidden = (onlyProblems && item.dataset.problem !== '1')
                || (term !== '' && !item.dataset.search.includes(term));
            item.hidden = hidden;
            if (!hidden) visible++;
        }
        shown += visible;
        const empty = section.querySelector('.hygiene-group-empty');
        empty.textContent = items.length === 0 ? 'Nothing to clean up here.' : 'No item matches the current filter.';
        empty.hidden = visible > 0;
        section.querySelector('.hygiene-caveat').hidden = items.length === 0;
    }
    const empty = $('hygiene-empty');
    empty.textContent = hygieneReport ? 'Nothing matches — this scope is tidy.' : '';
    empty.hidden = !hygieneReport || shown > 0 || (hygieneReport.total ?? 0) === 0;
}

$('hygiene-only-problems').addEventListener('change', applyHygieneFilter);
$('hygiene-filter').addEventListener('input', applyHygieneFilter);

// ============ Why is this Pod Pending? ============
//
// The backend (scheduling.go) re-runs the scheduler's feasibility checks node
// by node. This renders the answer in the order an operator reads it: the
// verdict, what the scheduler itself said, the reason groups with counts, and
// only then the individual nodes.

function openSchedulingModal(ref) {
    let scope;
    const load = () => {
        const box = $('sched-result');
        box.innerHTML = '<p class="modal-hint">Checking every node…</p>';
        return ExplainPodScheduling(ref.namespace, ref.name)
            .then((report) => { if (isCurrentModalRequest(scope)) renderScheduling(box, report); })
            .catch((err) => {
                if (isCurrentModalRequest(scope)) box.innerHTML = `<p class="error">${esc(errMsg(err))}</p>`;
                recordError(err);
            });
    };
    scope = openModal({
        title: 'Why is this Pod Pending?',
        eyebrow: 'Scheduling',
        description: `${ref.namespace}/${ref.name} — every node, and the reason it was ruled out.`,
        ownerKey: modalOwner('scheduling', $('cluster-select').value, ref.namespace, ref.name),
        okText: 'Done',
        cancelText: null,
        extraText: 'Re-check',
        onExtra: load,
        wide: true,
        bodyHtml: '<div id="sched-result" class="sched-result" aria-live="polite"></div>',
        onOpen: load,
    });
}

function renderScheduling(box, report) {
    const tone = { unschedulable: 'err', fits: 'warn', scheduled: 'ok', unknown: 'warn' }[report.verdict] ?? 'warn';
    const label = {
        unschedulable: 'No node fits', fits: 'A node does fit', scheduled: 'Already scheduled', unknown: 'Cannot tell',
    }[report.verdict] ?? report.verdict;
    const requests = (report.requests ?? []).map((request) => `${request.resource} ${request.request}`);
    box.innerHTML = `
        <div class="sched-verdict sched-${tone}">
            <span class="status-badge ${tone === 'err' ? 'status-error' : tone === 'ok' ? 'status-ok' : 'status-warn'}">${esc(label)}</span>
            <p class="sched-headline">${esc(report.headline)}</p>
        </div>
        <div class="checks-meta">${checkChips([
            report.nodeName ? `node ${report.nodeName}` : `${report.nodesFit} of ${report.nodesTotal} nodes fit`,
            ...requests,
        ])}</div>
        ${checkFindingsHtml(report.findings)}
        ${schedulingReasonsHtml(report.reasons ?? [])}
        ${schedulingNodesHtml(report.nodes ?? [])}
        ${schedulingEventsHtml(report.events ?? [])}
        ${(report.warnings ?? []).map((warning) => `<p class="checks-note">${esc(warning)}</p>`).join('')}
        <ul class="sched-limits">${(report.limits ?? []).map((limit) => `<li>${esc(limit)}</li>`).join('')}</ul>`;
}

function schedulingReasonsHtml(reasons) {
    if (!reasons.length) return '';
    return `<h4 class="sched-heading">Why each node was ruled out</h4>
        <ul class="sched-reasons">${reasons.map((reason) => `<li class="sched-reason">
            <span class="sched-reason-count">${reason.count}</span>
            <div><strong>${esc(reason.title)}</strong>
                ${reason.detail ? `<span class="sched-reason-detail">${esc(reason.detail)}</span>` : ''}
                <span class="sched-reason-nodes mono">${esc((reason.nodes ?? []).join(', '))}</span>
            </div>
        </li>`).join('')}</ul>`;
}

function schedulingNodesHtml(nodes) {
    if (!nodes.length) return '';
    return `<h4 class="sched-heading">Nodes</h4>
        <div class="table-wrap"><table class="sched-nodes"><thead><tr>
            <th>Node</th><th>Fits</th><th>CPU free</th><th>Memory free</th><th>Pods</th><th>Why not</th>
        </tr></thead><tbody>${nodes.map((node) => `<tr class="${node.fits ? 'sched-node-fits' : ''}">
            <td class="mono">${esc(node.name)}</td>
            <td>${node.fits ? '<span class="status-badge status-ok">Yes</span>' : '<span class="status-badge status-error">No</span>'}</td>
            <td>${esc(node.cpuFree)}</td><td>${esc(node.memFree)}</td><td>${esc(node.pods)}</td>
            <td>${(node.reasons ?? []).map((reason) => `<span class="chip">${esc(reason.text)}</span>`).join('') || '<span class="dim">—</span>'}</td>
        </tr>`).join('')}</tbody></table></div>`;
}

function schedulingEventsHtml(events) {
    if (!events.length) return '';
    return `<h4 class="sched-heading">What the cluster reported</h4>
        <ul class="sched-events">${events.map((event) => `<li class="${event.isWarn ? 'sched-event-warn' : ''}">
            <span class="chip">${esc(event.reason)}</span><span>${esc(event.message)}</span><span class="dim">${esc(event.age)}</span>
        </li>`).join('')}</ul>`;
}

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
		r.metricsAvailable ? `${r.metricsObserved ?? 0}/${r.metricsExpected ?? 0} container metrics` : '',
    ].filter(Boolean).join(' · ');

    $('sz-bars').innerHTML =
		szTile('CPU', t.cpuRequest, t.cpuUsage, r.allocCpu, r.cpuReservedPct, szCPU, 'cores', r.metricsComplete)
		+ szTile('Memory', t.memRequest, t.memUsage, r.allocMem, r.memReservedPct, szMem, '', r.metricsComplete);

    const note = $('sz-note');
    note.textContent = r.note || '';
    note.hidden = !r.note;

    const advice = r.advice ?? [];
    $('sz-advice-card').hidden = advice.length === 0;
    $('sz-advice').innerHTML = advice.map((a) => `<li>${esc(a)}</li>`).join('');

    const namespaces = r.namespaces ?? [];
    $('sz-ns-total').textContent = `${namespaces.length} namespace${namespaces.length === 1 ? '' : 's'} in scope`;
    $('sz-ns-body').innerHTML = namespaces.map((ns) => {
		const namespaceMetricsComplete = r.metricsAvailable
			&& (ns.metricsObserved ?? 0) === (ns.metricsExpected ?? 0);
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
			<td>${szCell(ns.cpuRequest, ns.cpuUsage, szCPU, namespaceMetricsComplete)}</td>
			<td>${szCell(ns.memRequest, ns.memUsage, szMem, namespaceMetricsComplete)}</td>
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
    // Namespace/pod/container in one cell: three separate columns cost width the
    // findings column needs, and they are read as one identity anyway.
    $('sz-cont-body').innerHTML = rows.map((c) => `<tr class="${(c.severity ?? 0) >= 4 ? 'sz-row-bad' : ''}">
        <td><span class="sz-ref">
            <span class="sz-ref-name">${esc(c.container)}</span>
            <span class="sz-ref-sub">${esc(c.namespace)} / ${esc(c.pod)}</span>
        </span></td>
        <td><span class="chip">${esc(c.qos || '—')}</span></td>
		<td>${szCell(c.cpuRequest, c.cpuUsage, szCPU, !!c.metricsObserved)}</td>
		<td>${szCell(c.memRequest, c.memUsage, szMem, !!c.metricsObserved)}</td>
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

function accessKey(kind, namespace, connection = requestScopes.connectionToken()) {
	return `${connection}\0${kind}|${namespace ?? ''}`;
}

function accessSet(kind, namespace) {
	const connection = requestScopes.connectionToken();
	const key = accessKey(kind, namespace, connection);
    if (accessResolved.has(key)) return Promise.resolve(accessResolved.get(key));
    if (!accessPending.has(key)) {
		accessPending.set(key, CanI(kind, namespace ?? '')
			.catch(() => null)
			.then((set) => {
				if (requestScopes.isCurrentConnection(connection)) accessResolved.set(key, set);
				accessPending.delete(key);
				return requestScopes.isCurrentConnection(connection) ? set : null;
			}));
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

function deniedPermissionReasons(plan) {
    return (plan?.requirements ?? [])
        .filter((requirement) => requirement.checked && !requirement.allowed)
        .map((requirement) => requirement.reason || `Permission denied: ${requirement.verb} ${requirement.kind}.`);
}

function ensurePermissionPlan(plan) {
    const denied = deniedPermissionReasons(plan);
    if (denied.length > 0 || plan?.permitted === false) {
        throw new Error(denied.join('\n') || 'The API server explicitly denied a required permission.');
    }
    return plan;
}

function permissionPlanSummary(plan) {
    const requirements = plan?.requirements?.length ?? 0;
    const unknown = plan?.unknown ?? 0;
    if (!requirements) return 'No Kubernetes resource permissions were required.';
    if (unknown) return `${requirements - unknown} permission checks passed · ${unknown} could not be verified and will be decided by the API server.`;
    return `${requirements} permission checks passed.`;
}

function permissionPlanHTML(plan) {
    if (!plan) return '';
    const denied = deniedPermissionReasons(plan);
    const tone = denied.length ? ' permission-plan-denied' : '';
    const title = denied.length ? `${denied.length} required permission${denied.length === 1 ? '' : 's'} denied` : 'Permission preflight';
    const details = denied.length ? denied : (plan.warnings ?? []);
    return `<section class="permission-plan${tone}" aria-label="Permission preflight">
        <strong>${esc(title)}</strong><p>${esc(permissionPlanSummary(plan))}</p>
        ${details.length ? `<ul>${details.map((item) => `<li>${esc(item)}</li>`).join('')}</ul>` : ''}
    </section>`;
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

let rowMenuTrigger = null;

function openRowMenu(btn, ref) {
    if (rowMenuTrigger && rowMenuTrigger !== btn) closeRowMenu();
    rowMenuTrigger = btn;
    btn.setAttribute('aria-expanded', 'true');
    const menu = $('row-menu');
    let actions;
    if (ref.kind === 'HelmRelease') {
        actions = [
            { label: 'Open release', run: () => openHelmDetailModal(ref) },
            { label: 'History & rollback…', run: () => openHelmDetailModal(ref, 'history') },
            { label: 'Upgrade…', run: () => openHelmUpgradeModal(ref) },
            { label: 'Uninstall', danger: true, run: () => uninstallHelm(ref) },
        ];
        renderRowMenu(menu, btn, actions);
        return;
    }
    // `need` is the verb the action requires — see the RBAC gating block above.
    // It is the verb the *API* wants, not the one the label suggests: a rolling
    // restart is a patch, while scaling has its own deployments/scale probe.
    actions = [
        { label: 'Open details', need: 'get', run: () => openDrawer({ ...ref, tab: 'details' }) },
        { label: '⌁ Investigate', need: 'get', run: () => openDrawer({ ...ref, tab: 'investigate' }) },
        { label: 'Edit YAML', need: 'get', run: () => openDrawer({ ...ref, tab: 'yaml' }) },
    ];
    if (ref.isPod) {
        // The first question about a Pod that is not running yet, and the one
        // kubectl answers worst. Read-only, so `get` is all it needs.
        actions.push({ label: '◷ Why Pending?', need: 'get', run: () => openSchedulingModal(ref) });
        actions.push({ label: 'View logs', need: 'logs', run: () => openDrawer({ ...ref, tab: 'logs' }) });
        actions.push({ label: 'Terminal', need: 'exec', run: () => openDrawer({ ...ref, tab: 'terminal' }) });
    }
    if (ref.kind === 'Deployment') {
        actions.push({ label: 'Scale…', need: 'scale', run: () => scaleRef(ref) });
        actions.push({ label: 'Restart', need: 'patch', run: () => restartRef(ref) });
        actions.push({ label: 'Rollout history…', need: 'get', run: () => openRolloutModal(ref) });
        actions.push({ label: 'Pause rollout', need: 'patch', run: () => pauseRef(ref, true) });
        actions.push({ label: 'Resume rollout', need: 'patch', run: () => pauseRef(ref, false) });
    }
    if (ref.kind === 'StatefulSet') {
        actions.push({ label: 'Restart', need: 'patch', run: () => restartWorkload(ref, RestartStatefulSetOwned) });
    }
    if (ref.kind === 'DaemonSet') {
        actions.push({ label: 'Restart', need: 'patch', run: () => restartWorkload(ref, RestartDaemonSetOwned) });
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
        `<button class="row-menu-item${a.danger ? ' danger' : ''}" role="menuitem" data-i="${i}"${
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
    menu.querySelector('.row-menu-item:not(:disabled)')?.focus();
}

function closeRowMenu({ restoreFocus = false } = {}) {
    $('row-menu').hidden = true;
    if (rowMenuTrigger) rowMenuTrigger.setAttribute('aria-expanded', 'false');
    const trigger = rowMenuTrigger;
    rowMenuTrigger = null;
    if (restoreFocus && trigger?.isConnected) trigger.focus();
}

$('row-menu').addEventListener('keydown', (event) => {
    const items = [...$('row-menu').querySelectorAll('.row-menu-item:not(:disabled)')];
    const current = items.indexOf(document.activeElement);
    let next = -1;
    if (event.key === 'ArrowDown') next = (current + 1) % items.length;
    else if (event.key === 'ArrowUp') next = (current - 1 + items.length) % items.length;
    else if (event.key === 'Home') next = 0;
    else if (event.key === 'End') next = items.length - 1;
    else if (event.key === 'Escape') {
        event.preventDefault();
        closeRowMenu({ restoreFocus: true });
        return;
    }
    if (next >= 0) {
        event.preventDefault();
        items[next]?.focus();
    }
});

function deleteRef(ref) {
    const connectionID = $('cluster-select').value;
    showConfirm(`Delete ${ref.kind} “${ref.name}”${ref.namespace ? ` in ${ref.namespace}` : ''}?\nThis cannot be undone.`, { title: `Delete ${ref.kind}`, icon: '🗑', okText: 'Delete', danger: true }).then((ok) => {
        if (!ok) return;
        DeleteResourceOwned(connectionID, ref.kind, ref.namespace, ref.name)
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
            return SetDeploymentPausedOwned(cluster, ref.namespace, ref.name, paused);
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
            return SetNodeSchedulableOwned(cluster, ref.name, schedulable);
        },
    )
        .then((changed) => { if (changed) refreshCurrentView(); })
        .catch((err) => showError(errMsg(err)));
}

function drainRef(ref) {
    const connectionID = $('cluster-select').value;
    Promise.all([
        PlanDrainPermissions(ref.name),
        // The preview informs the decision but must never prevent it.
        DrainImpact(ref.name).catch((err) => ({ error: errMsg(err) })),
    ])
        .then(([plan, impact]) => {
            ensurePermissionPlan(plan);
            return showConfirm(drainConfirmation(ref, plan, impact),
                { title: 'Drain node', icon: '🚰', okText: 'Drain', danger: true });
        })
        .then((ok) => {
            if (!ok) return;
            return DrainNodeOwned(connectionID, ref.name).then(() => refreshCurrentView());
        })
        .catch((err) => showError(errMsg(err)));
}

// Name what kubectl drain would stop and ask about: budgets that will refuse
// evictions, Pods no controller recreates, and emptyDir data that is deleted.
function drainConfirmation(ref, plan, impact) {
    const lines = [`Drain node “${ref.name}”?`];
    if (!impact || impact.error) {
        lines.push('This cordons it and evicts its pods (DaemonSet pods are kept).');
        if (impact?.error) lines.push(`Kubby could not preview the impact: ${impact.error}`);
    } else {
        const kept = [
            impact.daemonSetPods ? countNoun(impact.daemonSetPods, 'DaemonSet pod') : '',
            impact.mirrorPods ? countNoun(impact.mirrorPods, 'static pod') : '',
        ].filter(Boolean).join(' and ');
        const keptTotal = (impact.daemonSetPods ?? 0) + (impact.mirrorPods ?? 0);
        lines.push(`This cordons it and evicts ${countNoun(impact.evict, 'pod')}${kept ? `; ${kept} ${keptTotal === 1 ? 'stays' : 'stay'}` : ''}.`);
        for (const budget of impact.blockingBudgets ?? []) {
            lines.push(`⚠ PodDisruptionBudget ${budget.namespace}/${budget.name} allows ${countNoun(budget.allowedDisruptions, 'disruption')} but covers ${countNoun(budget.podsOnNode, 'pod')} here — the remaining evictions will be refused and the drain will stop partway.`);
        }
        if (impact.unmanaged?.length) {
            lines.push(`⚠ ${countNoun(impact.unmanaged.length, 'pod')} ${impact.unmanaged.length === 1 ? 'has' : 'have'} no controller and will not be recreated: ${listPreview(impact.unmanaged)}.`);
        }
        if (impact.emptyDir?.length) {
            lines.push(`⚠ ${countNoun(impact.emptyDir.length, 'pod')} ${impact.emptyDir.length === 1 ? 'uses' : 'use'} emptyDir; that data is deleted: ${listPreview(impact.emptyDir)}.`);
        }
        for (const warning of impact.warnings ?? []) lines.push(`⚠ ${warning}`);
    }
    lines.push('', permissionPlanSummary(plan));
    return lines.join('\n');
}

function countNoun(count, noun) {
    return `${count} ${noun}${count === 1 ? '' : 's'}`;
}

function listPreview(items, max = 5) {
    return items.length > max ? `${items.slice(0, max).join(', ')} and ${items.length - max} more` : items.join(', ');
}

function runCronNow(ref) {
    const connectionID = $('cluster-select').value;
    showConfirm(`Trigger CronJob “${ref.name}” now (create a Job)?`, { title: 'Trigger CronJob', icon: '⏱', okText: 'Trigger' }).then((ok) => {
        if (!ok) return;
        RunCronJobNowOwned(connectionID, ref.namespace, ref.name)
            .then(() => { loadSidebarCounts(); showAlert('Job created.', { title: 'Triggered', icon: '✅' }); })
            .catch((err) => showError(errMsg(err)));
    });
}

// Rollout history modal with per-revision Rollback.
function openRolloutModal(ref) {
	const connectionID = $('cluster-select').value;
	const scope = openModal({
		title: `Rollout history — ${ref.name}`,
        eyebrow: 'Deployment',
		description: `${ref.namespace || 'cluster-scoped'} · Compare revisions before choosing a rollback target.`,
		ownerKey: modalOwner('rollout', connectionID, ref.kind, ref.namespace, ref.name),
        okText: 'Close',
        okStyle: 'secondary',
        cancelText: null,
        bodyHtml: `<div id="rollout-list" class="rollout-list"><p class="empty-inline">Loading…</p></div>`,
        onOk: () => Promise.resolve(),
    });
	RolloutHistory(ref.namespace, ref.name)
		.then((revs) => {
			if (!isCurrentModalRequest(scope) || $('cluster-select').value !== connectionID) return;
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
					if (!isCurrentModalRequest(scope) || $('cluster-select').value !== connectionID) return;
					const rev = parseInt(btn.dataset.rev, 10);
					showConfirm(`Rollback “${ref.name}” to revision ${rev}?`, { title: 'Rollback deployment', icon: '↩', okText: 'Rollback' }).then((ok) => {
						if (!ok) return;
						if (!isCurrentModalRequest(scope) || $('cluster-select').value !== connectionID) {
							showError('The active cluster changed. Reopen rollout history.');
							return;
						}
						btn.disabled = true;
						RollbackDeploymentOwned(connectionID, ref.namespace, ref.name, rev)
							.then(() => { closeModal(scope); refreshCurrentView(); })
                            .catch((err) => { showError(errMsg(err)); btn.disabled = false; });
                    });
                });
            });
        })
		.catch((err) => {
			if (isCurrentModalRequest(scope)) $('rollout-list').innerHTML = `<p class="error">${esc(errMsg(err))}</p>`;
		});
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
document.querySelectorAll('#content .view:not(#view-overview) table:not(.plain) thead tr').forEach((tr) => {
    tr.insertAdjacentHTML('afterbegin', '<th class="no-sort col-check"><input type="checkbox" class="select-all" aria-label="Select all visible resources"></th>');
});
// Overview cards and Pods define their own columns; plain tables opt out.
document.querySelectorAll('#content .view:not(#view-overview):not(#view-pods) table:not(.plain) thead tr').forEach((tr) => {
    tr.insertAdjacentHTML('beforeend', '<th>Age</th><th class="no-sort col-actions">Actions</th>');
});

// Make every resource-view table header clickable to sort its rows.
document.querySelectorAll('#content .view thead th').forEach((th) => {
    if (th.classList.contains('no-sort')) return;
    th.classList.add('sortable');
    th.setAttribute('aria-sort', 'none');
    const button = document.createElement('button');
    button.type = 'button';
    button.className = 'sort-button';
    button.innerHTML = th.innerHTML;
    th.replaceChildren(button);
    button.addEventListener('click', () => sortTable(th));
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
    clearVirtualSelections();
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
    const body = e.target.closest('table').querySelector('tbody');
    allTableRows(body).map((row) => row.querySelector('.row-check')).filter(Boolean).forEach((cb) => {
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
	const connection = requestScopes.connectionToken();
	const connectionID = $('cluster-select').value;
	Promise.all(refs.map((ref) => ref.kind === 'HelmRelease' ? null : accessSet(ref.kind, ref.namespace)))
		.then((sets) => {
			if (!requestScopes.isCurrentConnection(connection)) return;
			const denied = refs.find((ref, index) => ref.kind !== 'HelmRelease' && !allowed(sets[index], 'delete'));
			if (denied) {
				showError(denyReason(denied.kind, denied.namespace, 'delete'));
				return;
			}
			return showConfirm(`Delete ${refs.length} selected resource(s)?\nThis cannot be undone.`, { title: 'Delete selected', icon: '🗑', okText: `Delete ${refs.length}`, danger: true });
		}).then((ok) => {
		if (!ok) return;
		if (!requestScopes.isCurrentConnection(connection) || $('cluster-select').value !== connectionID) {
			throw new Error('The active cluster changed before the action started.');
		}
        Promise.allSettled(refs.map((r) => r.kind === 'HelmRelease'
            ? HelmUninstallOwned(connectionID, r.namespace, r.name)
            : DeleteResourceOwned(connectionID, r.kind, r.namespace, r.name)))
            .then((results) => {
                const failed = results.filter((r) => r.status === 'rejected').length;
                clearSelection();
                loadSidebarCounts();
                refreshCurrentView();
                if (failed) showError(`${failed} of ${refs.length} could not be deleted.`);
	}).catch((err) => showError(errMsg(err)));
});
});

function sortTable(th) {
    const headRow = th.parentNode;
    const idx = [...headRow.children].indexOf(th);
    const table = th.closest('table');
    const tbody = table.querySelector('tbody');
    const rows = allTableRows(tbody);
    const asc = th.getAttribute('data-sort') !== 'asc';

    // Reset indicators on sibling headers.
    for (const h of headRow.children) {
        h.removeAttribute('data-sort');
        if (h.classList.contains('sortable')) h.setAttribute('aria-sort', 'none');
        const ind = h.querySelector('.sort-ind');
        if (ind) ind.remove();
    }
    th.setAttribute('data-sort', asc ? 'asc' : 'desc');
    th.setAttribute('aria-sort', asc ? 'ascending' : 'descending');
    const ind = document.createElement('span');
    ind.className = 'sort-ind';
    ind.textContent = asc ? ' ↑' : ' ↓';
    (th.querySelector('.sort-button') || th).appendChild(ind);

    const compare = (a, b) => {
        const av = cellSortValue(a, idx), bv = cellSortValue(b, idx);
        if (typeof av === 'number' && typeof bv === 'number') return asc ? av - bv : bv - av;
        return asc ? String(av).localeCompare(String(bv)) : String(bv).localeCompare(String(av));
    };
    if (sortVirtualRows(tbody, compare)) return;
    rows.sort(compare);
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
let paletteReturnFocus = null;

function buildPaletteCommands() {
    const cmds = [];
    for (const [view, title] of Object.entries(PAGE_TITLES)) {
        const label = view === 'structure' ? 'Topology · Dependencies'
            : view === 'traffic' ? 'Topology · Traffic routes' : title;
        cmds.push({ kind: 'Go to', label, run: () => selectView(view) });
    }
    for (const opt of $('namespace-select').options) {
        const label = opt.value === '' ? 'All namespaces' : opt.value;
        cmds.push({ kind: 'Namespace', label, run: () => selectNamespace(opt.value) });
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
    if (!$('palette').hidden) return;
    paletteReturnFocus = document.activeElement;
    paletteAll = buildPaletteCommands();
    paletteHits = [];
    paletteSel = 0;
    $('palette-input').value = '';
    $('palette-backdrop').hidden = false;
    $('palette').hidden = false;
    $('sidebar').inert = true;
    document.querySelector('.main').inert = true;
    if (!$('drawer').hidden) $('drawer').inert = true;
    renderPalette();
    $('palette-input').focus();
}
let paletteAll = [];
let paletteHits = [];          // live resource-search results (async, from SearchResources)
let paletteSearchTimer = null;
let paletteSearchGeneration = 0;

function closePalette({ restoreFocus = true } = {}) {
	paletteSearchGeneration++;
	$('palette').hidden = true;
    $('palette-backdrop').hidden = true;
    $('palette-input').removeAttribute('aria-activedescendant');
    $('drawer').inert = false;
    if ($('drawer').hidden) {
        $('sidebar').inert = false;
        document.querySelector('.main').inert = false;
    }
    clearTimeout(paletteSearchTimer);
    const target = paletteReturnFocus;
    paletteReturnFocus = null;
    if (restoreFocus && target?.isConnected) target.focus();
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
	const generation = ++paletteSearchGeneration;
	const connection = requestScopes.connectionToken();
	const query = term.trim();
	if (query.length < 2) { paletteHits = []; return; }
	paletteSearchTimer = setTimeout(() => {
		SearchResources(query)
			.then((hits) => {
				if (generation !== paletteSearchGeneration
					|| !requestScopes.isCurrentConnection(connection)
					|| $('palette').hidden
					|| $('palette-input').value.trim() !== query) return;
				paletteHits = (hits || []).map((h) => ({
                    kind: `${h.kind}${h.namespace ? ' · ' + h.namespace : ''}`,
                    label: h.name,
                    resource: true,
                    run: () => gotoHit(h),
                }));
                renderPalette();
            })
			.catch(() => {
				if (generation === paletteSearchGeneration && requestScopes.isCurrentConnection(connection)) {
					paletteHits = [];
					renderPalette();
				}
			});
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
        `<div id="palette-option-${i}" class="palette-item${i === paletteSel ? ' sel' : ''}${c.resource ? ' res' : ''}" role="option" aria-selected="${i === paletteSel}" data-i="${i}"><span>${esc(c.label)}</span><span class="p-kind">${esc(c.kind)}</span></div>`).join('');
    list.querySelectorAll('.palette-item').forEach((el) => {
        el.addEventListener('click', () => runPalette(parseInt(el.dataset.i, 10)));
        el.addEventListener('mousemove', () => { paletteSel = parseInt(el.dataset.i, 10); highlightPalette(); });
    });
    highlightPalette();
}

function highlightPalette() {
    $('palette-list').querySelectorAll('.palette-item').forEach((el, i) => {
        const selected = i === paletteSel;
        el.classList.toggle('sel', selected);
        el.setAttribute('aria-selected', String(selected));
    });
    const sel = $('palette-list').querySelector('.palette-item.sel');
    if (sel) {
        $('palette-input').setAttribute('aria-activedescendant', sel.id);
        sel.scrollIntoView({ block: 'nearest' });
    } else {
        $('palette-input').removeAttribute('aria-activedescendant');
    }
}

function runPalette(i) {
    const c = paletteFiltered[i];
    if (!c) return;
    closePalette({ restoreFocus: false });
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
    else if (e.key === 'Tab') { trapOverlayFocus($('palette'), e); }
});

// ============ Helm (release detail / history / upgrade / search / install) ============

// ---- External-link helper (opens the system browser via Wails runtime) ----

function extLink(url, text) {
    if (!url) return '';
    return `<button type="button" class="ext-link mono" data-url="${esc(url)}" title="Open in browser">${esc(text || url)} ↗</button>`;
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

function openHelmDetailModal(ref, initialTab = 'resources') {
    const connectionID = $('cluster-select').value;
    const scope = openModal({
        title: `Release · ${ref.name}`,
        eyebrow: 'Helm release',
        description: `${ref.namespace} · Inspect live resources, configuration and revision history.`,
        ownerKey: modalOwner('helm-detail', connectionID, ref.namespace, ref.name),
        okText: 'Close',
        okStyle: 'secondary',
        cancelText: null,
        wide: true,
        bodyHtml: `<div class="helm-release-hero">
                <div><span class="helm-kicker">${esc(ref.namespace)}</span><div id="helm-meta" class="helm-meta"><span class="helm-loading-inline">Loading release…</span></div></div>
                <div class="helm-release-actions"><button id="helm-upgrade-release" class="btn btn-primary btn-sm">Upgrade</button><button id="helm-run-tests" class="btn btn-secondary btn-sm">Run tests</button><button id="helm-uninstall-release" class="btn btn-quiet-danger btn-sm">Uninstall</button></div>
            </div>
            <div class="helm-tabs" role="tablist" aria-label="Release details">
                <button class="helm-tab" type="button" role="tab" aria-selected="false" data-htab="resources">Resources</button>
                <button class="helm-tab" type="button" role="tab" aria-selected="false" data-htab="values">Values</button>
                <button class="helm-tab" type="button" role="tab" aria-selected="false" data-htab="manifest">Manifest</button>
                <button class="helm-tab" type="button" role="tab" aria-selected="false" data-htab="notes">Notes</button>
                <button class="helm-tab" type="button" role="tab" aria-selected="false" data-htab="history">History</button>
            </div>
            <div id="helm-resources" class="helm-resources">Loading resources…</div>
            <pre id="helm-content" class="helm-content" hidden>Loading…</pre>
            <div id="helm-history" class="rollout-list" hidden><p class="empty-inline">Loading history…</p></div>`,
        onOk: () => Promise.resolve(),
    });
    const metaBox = $('helm-meta');
    const resourcesBox = $('helm-resources');
    const contentBox = $('helm-content');
    const historyBox = $('helm-history');
    const tabs = [...$('modal-body').querySelectorAll('.helm-tab')];
    const runTestsButton = $('helm-run-tests');
    let panes = { values: 'Loading…', manifest: 'Loading…', notes: 'Loading…' };
    let historyLoaded = false;
    const show = (requested) => {
        const tabName = ['resources', 'values', 'manifest', 'notes', 'history'].includes(requested) ? requested : 'resources';
        resourcesBox.hidden = tabName !== 'resources';
        historyBox.hidden = tabName !== 'history';
        contentBox.hidden = tabName === 'resources' || tabName === 'history';
        if (!contentBox.hidden) contentBox.textContent = panes[tabName];
        tabs.forEach((button) => {
            const active = button.dataset.htab === tabName;
            button.classList.toggle('active', active);
            button.setAttribute('aria-selected', String(active));
            button.tabIndex = active ? 0 : -1;
        });
        if (tabName === 'history' && !historyLoaded) {
            historyLoaded = true;
            loadHelmHistory(ref, scope, historyBox);
        }
    };
    tabs.forEach((tab, index) => {
        tab.addEventListener('click', () => show(tab.dataset.htab));
        tab.addEventListener('keydown', (event) => {
            if (!['ArrowLeft', 'ArrowRight'].includes(event.key)) return;
            event.preventDefault();
            const next = event.key === 'ArrowRight' ? (index + 1) % tabs.length : (index - 1 + tabs.length) % tabs.length;
            tabs[next].focus();
            show(tabs[next].dataset.htab);
        });
    });
    show(initialTab);

    HelmSnapshot(ref.namespace, ref.name)
		.then((snapshot) => {
            if (!isCurrentModalRequest(scope)) return;
			const d = snapshot.detail;
            metaBox.innerHTML = `<span class="chip">${esc(d.chart)}</span><span class="chip">app ${esc(d.appVersion || '—')}</span><span class="chip">revision ${d.revision}</span>${helmStatusBadge({ status: d.status })}`;
            panes = { values: d.values || '(no user-supplied values)', manifest: d.manifest || '', notes: d.notes || '(no notes)' };
			renderHelmReleaseResources(snapshot.resources ?? [], scope, resourcesBox);
            const activeTab = tabs.find((button) => button.classList.contains('active'))?.dataset.htab;
            if (activeTab && !['resources', 'history'].includes(activeTab)) contentBox.textContent = panes[activeTab];
        })
        .catch((err) => {
            if (!isCurrentModalRequest(scope)) return;
            const message = esc(errMsg(err));
            metaBox.innerHTML = `<p class="error">${message}</p>`;
            resourcesBox.innerHTML = `<p class="error">${message}</p>`;
        });

    $('helm-upgrade-release').addEventListener('click', () => openHelmUpgradeModal(ref));
    $('helm-uninstall-release').addEventListener('click', () => uninstallHelm(ref, scope));
    runTestsButton.addEventListener('click', async () => {
        const btn = runTestsButton;
        const cluster = $('cluster-select').value;
        const clusterName = $('cluster-select').selectedOptions[0]?.textContent ?? cluster;
        try {
            const plan = await PlanHelmPermissions('test', ref.namespace, ref.name, 0);
            ensurePermissionPlan(plan);
            await confirmedAction(
                () => showConfirm(
                    `Run Helm tests for release “${ref.name}” in namespace “${ref.namespace}” on cluster “${clusterName}”?\n`
                    + `Test hooks may create or delete cluster resources.\n\n${permissionPlanSummary(plan)}`,
                    { title: 'Run Helm tests', icon: '🧪', okText: 'Run tests' },
                ),
                async () => {
                    if (!isCurrentModalRequest(scope) || $('cluster-select').value !== cluster) {
                        throw new Error('The active cluster or release changed before the test started.');
                    }
                    btn.disabled = true; btn.textContent = 'Testing…';
                    const out = await HelmTestOwned(cluster, ref.namespace, ref.name);
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

function renderHelmReleaseResources(list, scope, box) {
	if (!isCurrentModalRequest(scope)) return;
            if (!list || list.length === 0) { box.innerHTML = '<p class="empty-inline">No resources found in the manifest.</p>'; return; }
            box.innerHTML = '';
            for (const r of list) {
                const div = document.createElement('div');
                div.className = 'helm-res-row';
                const openable = r.refKind && r.name;
                const health = r.health || (r.ready ? 'healthy' : 'unknown');
                div.innerHTML = `<div class="helm-res-main">
                        <span class="helm-res-kind">${esc(r.kind)}</span>
                        ${openable ? `<button class="helm-res-name link" type="button">${esc(r.name)}</button>` : `<span class="helm-res-name">${esc(r.name)}</span>`}
                        <span class="chart-repo">${esc(r.namespace || 'cluster-scoped')}</span>
                    </div>
                    <div><span class="helm-resource-health helm-resource-${esc(health)}"><i></i>${esc(r.status)}</span></div>`;
                if (openable) {
                    const nameEl = div.querySelector('button.helm-res-name');
                    nameEl.addEventListener('click', () => {
                        closeModal();
                        openDrawer({ kind: r.refKind, namespace: r.namespace, name: r.name });
                    });
                }
                box.appendChild(div);
            }
}

function loadHelmHistory(ref, scope, historyBox) {
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
                        ${i === 0 ? '' : `<button class="btn btn-secondary btn-sm helm-diff-btn" data-rev="${r.revision}">Preview rollback</button>
                        <button class="btn btn-secondary btn-sm helm-rollback-btn" data-rev="${r.revision}">Rollback to this</button>`}
                    </div>
                </div>`).join('') + '<pre id="helm-hist-diff" class="helm-content diff-view" hidden></pre>';
            box.querySelectorAll('.helm-diff-btn').forEach((btn) => {
                btn.addEventListener('click', () => {
                    const rev = parseInt(btn.dataset.rev, 10);
                    const pre = $('helm-hist-diff');
                    // Toggle: clicking the same revision's Diff again hides the panel.
                    if (!pre.hidden && pre.dataset.rev === String(rev)) {
                        pre.hidden = true;
                        box.querySelectorAll('.helm-diff-btn').forEach((b) => (b.textContent = 'Preview rollback'));
                        return;
                    }
                    pre.dataset.rev = String(rev);
                    pre.hidden = false;
                    pre.textContent = 'Loading diff…';
                    box.querySelectorAll('.helm-diff-btn').forEach((b) =>
                        (b.textContent = b === btn ? '✕ Hide preview' : 'Preview rollback'));
                    Promise.all([
                        HelmGetRevision(ref.namespace, ref.name, rev),
                        HelmGetRevision(ref.namespace, ref.name, currentRev),
                    ])
                        .then(([older, current]) => {
                            if (!isCurrentModalRequest(scope) || pre.dataset.rev !== String(rev)) return;
                            renderDiffInto(pre, current.manifest, older.manifest, { collapse: 3 });
                            pre.scrollIntoView({ block: 'nearest' });
                        })
                        .catch((err) => {
                            if (isCurrentModalRequest(scope) && pre.dataset.rev === String(rev)) pre.textContent = errMsg(err);
                        });
                });
            });
            box.querySelectorAll('.helm-rollback-btn').forEach((btn) => {
                btn.addEventListener('click', async () => {
                    const rev = parseInt(btn.dataset.rev, 10);
                    const cluster = $('cluster-select').value;
                    let plan;
                    try {
                        plan = await PlanHelmPermissions('rollback', ref.namespace, ref.name, rev);
                        ensurePermissionPlan(plan);
                    } catch (err) {
                        if (isCurrentModalRequest(scope)) showError(errMsg(err));
                        return;
                    }
                    showConfirm(`Rollback release “${ref.name}” to revision ${rev}?\n\n${permissionPlanSummary(plan)}`, { title: 'Rollback release', icon: '↩', okText: 'Rollback' }).then((ok) => {
                        if (!ok || !isCurrentModalRequest(scope)) return;
                        if ($('cluster-select').value !== cluster) {
                            showError('The active cluster changed before rollback started. Reopen the release and try again.');
                            return;
                        }
                        btn.disabled = true;
                        HelmRollbackOwned(cluster, ref.namespace, ref.name, rev)
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
    const connectionID = $('cluster-select').value;
    let valuesEditor = null;
    let valuesLoading = true;
    let valuesTouched = false;
    let applyingLoadedValues = false;
    let approvedPreview = null;
    let okButton = null;
    let previewStatus = null;
    const invalidatePreview = (message = '') => {
        approvedPreview = null;
        if (okButton) okButton.disabled = true;
        if (previewStatus && message) previewStatus.textContent = message;
    };
    const scope = openModal({
        title: `Upgrade values — ${ref.name}`,
        eyebrow: 'Helm release',
        description: `${ref.namespace} · Upgrade remains locked until these exact values are previewed.`,
        ownerKey: modalOwner('helm-upgrade', connectionID, ref.namespace, ref.name),
        okText: 'Upgrade',
        okDisabled: true,
        wide: true,
        extraText: '← Release details',
        onExtra: () => { openHelmDetailModal(ref); },
        bodyHtml: `<div class="helm-install-intro"><span class="helm-install-step">1</span><div><strong>Edit release values</strong><p>This reuses the chart currently installed for ${esc(ref.name)}.</p></div></div>
            <div id="helm-values" class="yaml-host yaml-host-modal"></div>
            <div class="helm-install-intro helm-install-intro-values"><span class="helm-install-step">2</span><div><strong>Preview the exact upgrade</strong><p>Any values change invalidates the preview and disables Upgrade.</p></div></div>
            <div class="install-actions">
                <button id="helm-preview-btn" class="btn btn-secondary btn-sm">👁 Preview diff</button>
                <span id="helm-preview-status" class="modal-hint"></span>
            </div>
            <pre id="helm-upg-diff" class="helm-content diff-view" hidden></pre>`,
        onOpen: () => {
            valuesEditor = mountModalEditor('helm-values', {
                placeholder: 'Loading current release values…',
                onChange: () => {
                    if (applyingLoadedValues) return;
                    valuesTouched = true;
                    if (!valuesLoading) invalidatePreview('Values changed — preview again before upgrading.');
                },
            });
        },
        onOk: () => {
            if (!isCurrentModalRequest(scope) || !valuesEditor) return Promise.reject('This Upgrade dialog is stale. Reopen it.');
            const vals = valuesEditor.getValue();
            if (!approvedPreview || approvedPreview.values !== vals) {
                return Promise.reject('Preview these exact values before upgrading.');
            }
            return HelmUpgradeValuesOwned(connectionID, ref.namespace, ref.name, vals, approvedPreview.revision, approvedPreview.valuesDigest)
                .then(() => {
                    refreshCurrentView();
                    setTimeout(() => openHelmDetailModal(ref), 0);
                });
        },
    });
    okButton = $('modal-ok');
    const previewButton = $('helm-preview-btn');
    const previewBox = $('helm-upg-diff');
    previewStatus = $('helm-preview-status');
    previewButton.disabled = true;
    HelmGet(ref.namespace, ref.name)
        .then((d) => {
            if (!isCurrentModalRequest(scope)) return;
            valuesLoading = false;
            if (valuesTouched) {
                invalidatePreview('Current values finished loading; your draft was kept. Preview it before upgrading.');
            } else {
                applyingLoadedValues = true;
                valuesEditor.setValue(d.values || '');
                applyingLoadedValues = false;
                invalidatePreview('Preview is required before Upgrade.');
            }
            previewButton.disabled = false;
        })
        .catch((err) => {
            if (!isCurrentModalRequest(scope)) return;
            valuesLoading = false;
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
        approvedPreview = null;
        okButton.disabled = true;
        pre.hidden = false; pre.textContent = 'Rendering (dry-run)…'; status.textContent = ''; btn.textContent = '✕ Hide preview';
        HelmUpgradePreview(ref.namespace, ref.name, vals)
            .then((diff) => {
                if (!isCurrentModalRequest(scope) || reqId !== previewReqId) return;
                if (valuesEditor.getValue() !== vals) {
                    invalidatePreview('Values changed while previewing — preview again.');
                    return;
                }
                if (!diff.releaseRevision || !diff.valuesDigest) throw new Error('Preview did not return release ownership data.');
                const denied = deniedPermissionReasons(diff.permissions);
                renderDiffInto(pre, diff.current, diff.proposed, { collapse: 3 });
                if (denied.length || diff.permissions?.permitted === false) {
                    approvedPreview = null;
                    okButton.disabled = true;
                    status.textContent = denied.join(' ') || 'A required permission was denied.';
                    return;
                }
                approvedPreview = { values: vals, revision: diff.releaseRevision, valuesDigest: diff.valuesDigest };
                okButton.disabled = false;
                status.textContent = `Preview approved for revision ${diff.releaseRevision}. ${diff.permissions ? permissionPlanSummary(diff.permissions) : ''}`.trim();
            })
            .catch((err) => {
                if (isCurrentModalRequest(scope) && reqId === previewReqId) {
                    approvedPreview = null;
                    okButton.disabled = true;
                    pre.textContent = errMsg(err);
                }
            });
    });
}

function uninstallHelm(ref, modalScope = null) {
    const cluster = $('cluster-select').value;
    const clusterName = $('cluster-select').selectedOptions[0]?.textContent ?? cluster;
    PlanHelmPermissions('uninstall', ref.namespace, ref.name, 0)
        .then((plan) => {
            ensurePermissionPlan(plan);
            return showConfirm(
                `Uninstall Helm release “${ref.name}” in ${ref.namespace} from cluster “${clusterName}”?\nThis removes all its resources.\n\n${permissionPlanSummary(plan)}`,
                { title: 'Uninstall release', icon: '🗑', okText: 'Uninstall', danger: true },
            );
        })
        .then((ok) => {
        if (!ok) return;
        if ($('cluster-select').value !== cluster || (modalScope && !isCurrentModalRequest(modalScope))) {
            showError('The active cluster or release changed before uninstall started. Reopen the release and try again.');
            return;
        }
        if (modalScope && isCurrentModalRequest(modalScope)) closeModal(modalScope);
        HelmUninstallOwned(cluster, ref.namespace, ref.name)
            .then(() => { loadSidebarCounts(); refreshCurrentView(); })
            .catch((err) => showError(errMsg(err)));
        })
        .catch((err) => showError(errMsg(err)));
}

// ---- Catalog (Artifact Hub + configured repositories) + install ----

let helmCatalogRequestID = 0;
let helmCatalogPreferredSource = '';

function loadHelmCatalogSources(scope) {
    const select = $('helm-catalog-source');
    const selected = helmCatalogPreferredSource || select.value || 'artifacthub';
    return ListHelmRepos()
        .then((repos) => {
            if (!isCurrentViewRequest(scope) || helmSection !== 'catalog') return;
            select.innerHTML = '<option value="artifacthub">Artifact Hub</option>'
                + (repos ?? []).map((repo) => `<option value="repo:${esc(repo.name)}">Repository · ${esc(repo.name)}</option>`).join('');
            select.value = [...select.options].some((option) => option.value === selected) ? selected : 'artifacthub';
            updateHelmCatalogHint();
            if (helmCatalogPreferredSource) {
                helmCatalogPreferredSource = '';
                $('chart-query').value = '';
                runHelmCatalogSearch();
            }
        })
        .catch((err) => viewError(scope, err));
}

function updateHelmCatalogHint() {
    const sourceID = $('helm-catalog-source').value;
    $('helm-catalog-hint').textContent = sourceID === 'artifacthub'
        ? 'Artifact Hub search requires internet access.'
        : 'Browsing the cached repository index. Update repositories if versions look stale.';
    $('chart-query').placeholder = sourceID === 'artifacthub'
        ? 'nginx, redis, prometheus…'
        : 'Filter charts, or leave empty to show all';
}

function runHelmCatalogSearch() {
    if (currentView !== 'helm' || helmSection !== 'catalog') return;
    const sourceID = $('helm-catalog-source').value;
    const query = $('chart-query').value.trim();
    if (sourceID === 'artifacthub' && !query) {
        $('chart-results').innerHTML = '<div class="helm-empty-state"><strong>Enter a chart name</strong><span>Artifact Hub searches by relevance.</span></div>';
        return;
    }
    const requestID = ++helmCatalogRequestID;
    const connection = requestScopes.connectionToken();
    const resultsBox = $('chart-results');
    resultsBox.innerHTML = '<div class="helm-loading">Searching charts…</div>';
    const request = sourceID === 'artifacthub'
        ? SearchCharts(query)
        : BrowseHelmRepo(sourceID.slice('repo:'.length)).then((charts) => {
            const term = query.toLowerCase();
            return term ? (charts ?? []).filter((chart) => `${chart.name} ${chart.description || ''}`.toLowerCase().includes(term)) : charts;
        });
    request
        .then((results) => {
            if (requestID !== helmCatalogRequestID || !requestScopes.isCurrentConnection(connection)
                || currentView !== 'helm' || helmSection !== 'catalog') return;
            renderChartResults(results, resultsBox);
        })
        .catch((err) => {
            if (requestID === helmCatalogRequestID && requestScopes.isCurrentConnection(connection)
                && currentView === 'helm' && helmSection === 'catalog') {
                resultsBox.innerHTML = `<div class="helm-empty-state helm-empty-error"><strong>Could not load charts</strong><span>${esc(errMsg(err))}</span><button class="btn btn-secondary btn-sm helm-catalog-retry">Try again</button></div>`;
                resultsBox.querySelector('.helm-catalog-retry')?.addEventListener('click', runHelmCatalogSearch);
            }
        });
}

document.querySelectorAll('.helm-workspace-tab').forEach((tab, index, tabs) => {
    tab.addEventListener('click', () => setHelmSection(tab.dataset.helmSection));
    tab.addEventListener('keydown', (event) => {
        if (!['ArrowLeft', 'ArrowRight'].includes(event.key)) return;
        event.preventDefault();
        const next = event.key === 'ArrowRight' ? (index + 1) % tabs.length : (index - 1 + tabs.length) % tabs.length;
        tabs[next].focus();
        setHelmSection(tabs[next].dataset.helmSection);
    });
});

const openHelmCatalog = () => {
    setHelmSection('catalog');
    setTimeout(() => $('chart-query').focus(), 0);
};
$('btn-helm-search').addEventListener('click', openHelmCatalog);
$('helm-empty-browse').addEventListener('click', openHelmCatalog);
$('helm-release-filter').addEventListener('input', filterHelmReleases);
$('chart-search-go').addEventListener('click', runHelmCatalogSearch);
$('chart-query').addEventListener('keydown', (event) => {
    if (event.key === 'Enter') { event.preventDefault(); runHelmCatalogSearch(); }
});
$('helm-catalog-source').addEventListener('change', () => {
    updateHelmCatalogHint();
    runHelmCatalogSearch();
});

function renderChartResults(results, box) {
    if (!results || results.length === 0) {
        box.innerHTML = '<div class="helm-empty-state"><strong>No charts found</strong><span>Try a broader name or another source.</span></div>';
        return;
    }
    box.innerHTML = '';
    for (const c of results) {
        const div = document.createElement('div');
        div.className = 'chart-item';
        const oci = String(c.repoURL || '').startsWith('oci://');
        div.innerHTML = `<div class="chart-main">
                <div class="chart-name">${esc(c.name)} <span class="chart-repo">${esc(c.repo)}</span>${oci ? ' <span class="chip">OCI</span>' : ''}</div>
                <div class="chart-desc">${esc(c.description || '')}</div>
                <div class="chart-ver mono">Chart ${esc(c.version)}${c.appVersion ? ` · App ${esc(c.appVersion)}` : ''}${c.stars ? ` · ★ ${c.stars}` : ''}</div>
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
    const connectionID = $('cluster-select').value;
    const ns = currentNamespace || 'default';
    const chartName = chart.normName || chart.name;
    const repoName = chart.sourceID || '';
    const namespaceOptions = [...$('namespace-select').options]
        .map((option) => option.value).filter(Boolean)
        .map((name) => `<option value="${esc(name)}"></option>`).join('');
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
        eyebrow: 'Chart installation',
        description: `${chart.repo || 'Chart repository'} · Choose a destination, configure values, then preview the exact release.`,
        ownerKey: modalOwner('chart-install', chart.repoURL, chartName),
        okText: 'Install',
        okDisabled: true,
        wide: true,
        bodyHtml: `<div class="helm-install-intro"><span class="helm-install-step">1</span><div><strong>Choose destination and version</strong><p>The final preview is bound to these exact fields and values.</p></div></div>
            <div class="install-form helm-install-target">
                <label>Release name<input type="text" id="inst-name" class="pf-input" value="${esc(chartName)}" autocomplete="off"></label>
                <label>Namespace<input type="text" id="inst-ns" class="pf-input" list="helm-namespace-options" value="${esc(ns)}" autocomplete="off"><datalist id="helm-namespace-options">${namespaceOptions}</datalist></label>
                <label>Version<select id="inst-ver" class="pf-input"><option value="${esc(chart.version)}">${esc(chart.version)}</option></select></label>
            </div>
            <p class="helm-namespace-note">If the namespace does not exist, Helm will create it. Check spelling before previewing.</p>
            <div id="inst-links" class="install-links"><span class="modal-hint">Repo: </span>${extLink(chart.repoURL)}</div>
            <div class="helm-install-intro helm-install-intro-values"><span class="helm-install-step">2</span><div><strong>Configure and review</strong><p>Load defaults when needed, then preview what Helm will create.</p></div></div>
            <div class="install-tabs" role="tablist" aria-label="Chart configuration">
                <button class="install-tab active" type="button" role="tab" aria-selected="true" data-itab="values">Values</button>
                <button class="install-tab" type="button" role="tab" aria-selected="false" data-itab="readme">README</button>
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
            return HelmInstallOwned(connectionID, nsv, name, chart.repoURL, repoName, chartName, ver, vals, approvedPreview.digest)
                .then(() => {
                    let namespaceOption = [...$('namespace-select').options].find((option) => option.value === nsv);
                    if (!namespaceOption) {
                        namespaceOption = document.createElement('option');
                        namespaceOption.value = nsv;
                        namespaceOption.textContent = nsv;
                        $('namespace-select').appendChild(namespaceOption);
                    }
                    currentNamespace = nsv;
                    $('namespace-select').value = nsv;
                    syncNamespacePicker();
                    helmSection = 'releases';
                    selectView('helm');
                    loadSidebarCounts();
                    setTimeout(() => openHelmDetailModal({ kind: 'HelmRelease', namespace: nsv, name }), 0);
                });
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

    if (repoName && chart.versions?.length) {
        versionSelect.innerHTML = chart.versions
            .map((version) => `<option value="${esc(version)}"${version === chart.version ? ' selected' : ''}>${esc(version)}</option>`).join('');
    }

    // Tabs: Values / README.
    const showInstallTab = (t) => {
        installTabs.forEach((x) => x.classList.toggle('active', x === t));
        installTabs.forEach((x) => x.setAttribute('aria-selected', String(x === t)));
        installTabs.forEach((x) => { x.tabIndex = x === t ? 0 : -1; });
        const readme = t.dataset.itab === 'readme';
        valuesPane.hidden = readme;
        readmeBox.hidden = !readme;
    };
    installTabs.forEach((t, index) => {
        t.addEventListener('click', () => showInstallTab(t));
        t.addEventListener('keydown', (event) => {
            if (!['ArrowLeft', 'ArrowRight'].includes(event.key)) return;
            event.preventDefault();
            const next = event.key === 'ArrowRight' ? (index + 1) % installTabs.length : (index - 1 + installTabs.length) % installTabs.length;
            installTabs[next].focus();
            showInstallTab(installTabs[next]);
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
        ChartDefaultValues(chart.repoURL, repoName, chartName, requestedVersion)
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
            chart.repoURL, repoName, chartName, previewInput.version, previewInput.values)
            .then((diff) => {
                if (!isCurrentModalRequest(scope) || reqId !== previewReqId) return;
                if (!diff.chartDigest) throw new Error('Preview did not return a chart digest.');
                const denied = deniedPermissionReasons(diff.permissions);
                renderDiffInto(pre, diff.current, diff.proposed);
                if (denied.length || diff.permissions?.permitted === false) {
                    approvedPreview = null;
                    okButton.disabled = true;
                    statusBox.textContent = denied.join(' ') || 'A required permission was denied.';
                    return;
                }
                approvedPreview = { ...previewInput, digest: diff.chartDigest };
                okButton.disabled = false;
                statusBox.textContent = `Previewed exact chart ${diff.chartDigest.slice(0, 19)}… ${diff.permissions ? permissionPlanSummary(diff.permissions) : ''}`.trim();
            })
            .catch((err) => {
                if (isCurrentModalRequest(scope) && reqId === previewReqId) {
                    approvedPreview = null;
                    okButton.disabled = true;
                    pre.textContent = errMsg(err);
                }
            });
    });

    // Artifact Hub supplies rich versions/README. A configured repository keeps
    // install functional from its own authenticated source even when it is not
    // published on Artifact Hub.
    if (!repoName) ChartDetails(chart.repo, chartName)
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
    else readmeBox.textContent = 'README metadata is not available from the cached repository index. Values and install still use the configured repository directly.';
}

// ============ Helm repositories ============

function loadHelmRepos(scope) {
    $('helmrepos-loading').hidden = false;
    $('helmrepos-empty').hidden = true;
    return ListHelmRepos()
        .then((repos) => {
            if (!isCurrentViewRequest(scope) || helmSection !== 'repositories') return;
            const body = $('helmrepos-body');
            body.innerHTML = '';
            $('helmrepos-loading').hidden = true;
            $('helmrepos-empty').hidden = (repos?.length ?? 0) > 0;
            for (const r of repos ?? []) {
                const tr = document.createElement('tr');
                tr.innerHTML = `<td>${esc(r.name)}</td>
                    <td>${extLink(r.url)}</td>
                    <td>${r.authenticated ? '<span class="chip">Private</span>' : '<span class="dim">Public</span>'}</td>
                    <td class="col-actions">
                        <button class="btn btn-secondary btn-sm repo-browse" data-name="${esc(r.name)}">Browse</button>
                        <button class="btn btn-secondary btn-sm repo-remove" data-name="${esc(r.name)}">Remove</button>
                    </td>`;
                body.appendChild(tr);
            }
            body.querySelectorAll('.repo-browse').forEach((btn) =>
                btn.addEventListener('click', () => {
                    helmCatalogPreferredSource = `repo:${btn.dataset.name}`;
                    setHelmSection('catalog');
                }));
            body.querySelectorAll('.repo-remove').forEach((btn) =>
                btn.addEventListener('click', () => {
                    showConfirm(`Remove repo “${btn.dataset.name}”?`, { title: 'Remove repository', icon: '🗑', okText: 'Remove', danger: true }).then((ok) => {
                        if (!ok) return;
                        RemoveHelmRepo(btn.dataset.name).then(() => refreshCurrentView()).catch((err) => showError(errMsg(err)));
                    });
                }));
        })
        .catch((err) => {
            if (isCurrentViewRequest(scope) && helmSection === 'repositories') $('helmrepos-loading').hidden = true;
            viewError(scope, err);
        });
}

function openRepoAddModal() {
    openModal({
        title: 'Add Helm repository',
        eyebrow: 'Helm repositories',
        description: 'Add a chart source to the user-level Helm configuration on this machine.',
        ownerKey: 'helm-repo-add',
        okText: 'Add',
        bodyHtml: `<div class="modal-context-card"><div><span class="helm-kicker">Local machine</span><strong>Shared with the Helm CLI</strong><p>This updates your user-level repositories.yaml, not the active cluster.</p></div><span class="chip">Local configuration</span></div>
            <div class="modal-form-grid">
                <label>Name<input type="text" id="repo-name" class="pf-input" placeholder="bitnami" autocomplete="off" autofocus><span class="field-note">A short identifier used in Catalog.</span></label>
                <label>Repository URL<input type="url" id="repo-url" class="pf-input" placeholder="https://charts.bitnami.com/bitnami" autocomplete="url"><span class="field-note">The URL that serves index.yaml.</span></label>
            </div>
            <div class="modal-section-title"><strong>Authentication</strong><p>Optional. Leave both fields empty for a public repository.</p></div>
            <div class="modal-form-grid">
                <label>Username<input type="text" id="repo-user" class="pf-input" autocomplete="username"></label>
                <label>Password<input type="password" id="repo-pass" class="pf-input" autocomplete="new-password"></label>
            </div>
            <p class="modal-inline-note"><span aria-hidden="true">↓</span>The repository index is downloaded once to validate this source, so adding it requires network access.</p>`,
        onOk: () => {
            const name = $('repo-name').value.trim();
            const url = $('repo-url').value.trim();
            if (!name || !url) return Promise.reject('Name and URL are required.');
            return AddHelmRepo(name, url, $('repo-user').value, $('repo-pass').value).then(() => refreshCurrentView());
        },
    });
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
let liveRefreshPending = false;
const LIVE_REFRESH_MS = 5000;
const LIVE_SIDEBAR_REFRESH_MS = 30000;

$('btn-live').addEventListener('click', () => {
    if (liveTimer) {
        clearInterval(liveTimer);
        liveTimer = null;
        $('btn-live').classList.remove('live-on');
        $('btn-live').setAttribute('aria-pressed', 'false');
        $('btn-live').title = 'Turn on auto-refresh';
    } else {
        liveTimer = setInterval(() => {
            if ($('dashboard').hidden) return;
            // Don't disrupt an active selection / open drawer / modal / palette.
            if (selectedRows.size > 0) return;
            if (!$('drawer').hidden || !$('modal').hidden || !$('palette').hidden) return;
			if (liveRefreshPending) return;
			liveRefreshPending = true;
			Promise.resolve(refreshCurrentView())
				.then(() => {
					if (Date.now() - navCountsRefreshedAt >= LIVE_SIDEBAR_REFRESH_MS) return loadSidebarCounts();
				})
				.catch(() => {})
				.finally(() => { liveRefreshPending = false; });
        }, LIVE_REFRESH_MS);
        $('btn-live').classList.add('live-on');
        $('btn-live').setAttribute('aria-pressed', 'true');
        $('btn-live').title = 'Turn off auto-refresh';
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
// a one-shot popup. One redacted snapshot is collected for the thread, shown as
// chips in the header, inspectable in full, and reused byte-for-byte on every send.

let aiThread = [];    // [{ role, content }] — the visible conversation
let aiThreadKey = ''; // kind/ns/name the thread belongs to
let aiBusy = false;
let aiContext = null; // last-fetched AIContext for the chips / "what gets sent"
let aiStatus = null;  // cached GetAIStatus() — keeps rendering synchronous
const aiRequests = createKeyedRequestOwner();

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

function aiKeyFor(ref, connectionID = $('cluster-select').value) {
	return `${connectionID}/${ref.kind}/${ref.namespace}/${ref.name}`;
}

// Called when the drawer opens on a new resource: the old thread belongs to a
// different resource, so it is dropped rather than silently carried over.
function resetAIPanel(ref) {
    const key = aiKeyFor(ref);
    if (!aiRequests.select(key)) return;
    aiThreadKey = key;
    aiThread = [];
    aiContext = null;
    aiBusy = false;
    $('ai-input').value = '';
    $('ai-input').style.height = '';
    $('btn-ai-send').disabled = true;
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
		const scope = activeDrawerScope;
		const key = aiKeyFor(ref);
		$('ai-context-chips').innerHTML = '<span class="ai-chip ai-chip-loading">Collecting evidence…</span>';
		AIResourceContext(ref.kind, ref.namespace, ref.name)
			.then((c) => {
				if (!isCurrentDrawerRequest(scope) || aiThreadKey !== key) return;
				aiContext = c;
				$('btn-ai-send').disabled = false;
				renderAIContextChips();
			})
			.catch(() => {
				if (!isCurrentDrawerRequest(scope) || aiThreadKey !== key) return;
				$('btn-ai-send').disabled = true;
				$('ai-context-chips').innerHTML = '<span class="ai-chip ai-chip-error">Evidence unavailable — reopen this tab to retry</span>';
			});
    }
}

function renderAIContextChips() {
    const c = aiContext;
    if (!c) { $('ai-context-chips').innerHTML = ''; return; }
    const chips = [];
    if (c.events) chips.push(`${c.events} event${c.events === 1 ? '' : 's'}`);
    if (c.logContainers) chips.push(`${c.logLines} log lines from ${c.logContainers} container${c.logContainers === 1 ? '' : 's'}`);
    if (c.previousLogContainers) chips.push(`pre-restart logs from ${c.previousLogContainers} container${c.previousLogContainers === 1 ? '' : 's'}`);
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
        eyebrow: 'AI privacy',
        description: 'Inspect the complete resource context attached to questions in this thread.',
        okText: 'Close',
        okStyle: 'secondary',
        cancelText: null,
        size: 'editor',
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
    if (!q || aiBusy || !drawerRef || !aiContext?.text) return;
	const ref = drawerRef;
	const key = aiKeyFor(ref);
    const request = aiRequests.begin(key);

    aiThread.push({ role: 'user', content: q });
    aiBusy = true;
    $('ai-input').value = '';
    autoGrowAIInput();
    $('btn-ai-send').disabled = true;
    renderAIThread();

	AskAboutResource(ref.kind, ref.namespace, ref.name, aiContext.text, aiThread)
		.then((answer) => {
            if (!aiRequests.isCurrent(request)) return;
            aiThread.push({ role: 'assistant', content: answer });
        })
        .catch((err) => {
            if (!aiRequests.isCurrent(request)) return;
            aiThread.push({ role: 'assistant', content: `⚠️ **${errMsg(err)}**\n\nOpen the ⚙ button above to check the provider settings.` });
        })
        .finally(() => {
            // The thread, not a particular drawer opening, owns the request.
            // Reopening the same resource must not leave its composer stuck;
            // switching resources invalidates the keyed owner and discards this result.
            if (!aiRequests.isCurrent(request)) return;
            aiBusy = false;
            if (drawerRef && aiKeyFor(drawerRef) === key && !$('drawer').hidden) {
                $('btn-ai-send').disabled = !aiContext?.text;
                renderAIThread();
            }
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
    let settingsDirty = false;
    let scope;
    scope = openModal({
        title: 'Settings',
        eyebrow: 'Application',
        description: 'Configure the AI assistant and review build diagnostics stored on this machine.',
        ownerKey: modalOwner('settings'),
        okText: 'Save',
        bodyHtml: `<div class="modal-context-card"><div><span class="helm-kicker">AI assistant</span><strong>Your evidence, your provider</strong><p>Questions include the selected resource's events, recent logs and manifest. The API key stays in <code>%AppData%/kubby/ai.json</code> on this machine.</p></div><span class="chip">Local key storage</span></div>
            <div class="modal-section-title"><strong>Provider</strong><p>Choose where Kubby sends questions and resource evidence.</p></div>
            <div class="settings-ai-form">
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
                    <span class="about-label">Build information</span>
                    <span class="about-version mono" id="about-version">…</span>
                </div>
                <p class="modal-hint settings-diagnostics-note">
                    Reporting a problem? <strong>Copy diagnostics</strong> gathers the version, this machine's
                    platform, the connected cluster's Kubernetes version and capabilities, and a credential-redacted
                    last error. The error may contain server or resource details; review it before sharing.
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
        if (!isCurrentModalRequest(scope)) return;
        const p = $('ai-provider').value;
        $('ai-key-wrap').style.display = p === 'ollama' ? 'none' : '';
        $('ai-hint').textContent = hints[p] || '';
    };
    $('ai-provider').addEventListener('change', applyProviderUI);
    for (const id of ['ai-provider', 'ai-model', 'ai-endpoint', 'ai-key', 'ai-language']) {
        $(id).addEventListener(id === 'ai-provider' || id === 'ai-language' ? 'change' : 'input', () => {
            settingsDirty = true;
        });
    }

    GetAIConfig().then((cfg) => {
        if (!isCurrentModalRequest(scope)) return;
        configuredProvider = cfg.provider || '';
        hasStoredKey = !!cfg.hasApiKey;
        if (!settingsDirty) {
            $('ai-provider').value = cfg.provider || '';
            $('ai-endpoint').value = cfg.endpoint || '';
            $('ai-key').value = '';
            $('ai-model').value = cfg.model || '';
            $('ai-language').value = cfg.language || 'auto';
        }
        $('ai-key').placeholder = hasStoredKey
            ? 'saved key is hidden — leave blank to keep it'
            : 'stored locally, never shown again';
        applyProviderUI();
    }).catch(() => { if (isCurrentModalRequest(scope)) applyProviderUI(); });

    AppVersion().then((v) => {
        if (isCurrentModalRequest(scope)) $('about-version').textContent = v;
    }).catch(() => {
        if (isCurrentModalRequest(scope)) $('about-version').textContent = 'unknown';
    });

    $('btn-copy-diagnostics').addEventListener('click', () => {
        const btn = $('btn-copy-diagnostics');
        btn.disabled = true;
        // Collected in Go, and copied through the Wails runtime rather than
        // navigator.clipboard — the WebView does not always grant the page
        // clipboard permission, and a silent failure here is worse than useless.
        Diagnostics(lastErrorSeen)
            .then((report) => CopyToClipboard(report))
            .then(() => {
                if (!isCurrentModalRequest(scope)) return;
                const flag = $('diag-copied');
                flag.hidden = false;
                setTimeout(() => { if (flag.isConnected) flag.hidden = true; }, 2500);
            })
            .catch((err) => {
                if (isCurrentModalRequest(scope)) showError(errMsg(err), 'Could not copy the diagnostics');
            })
            .finally(() => { if (isCurrentModalRequest(scope)) btn.disabled = false; });
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
	if (welcomeConnectPending) return;
	clearWelcomeError();
	source = { mode: 'path', path: r.path, content: '' };
	setWelcomeConnectPending(true);
	$('connecting-overlay').hidden = false;
    ConnectWithPath(r.path, r.context)
        .then(() => enterDashboard(r.context))
        .catch(showWelcomeError)
		.finally(() => {
			$('connecting-overlay').hidden = true;
			setWelcomeConnectPending(false);
		});
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
