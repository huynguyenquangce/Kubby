const DEFAULT_FIXTURES = {
    RecentConnections: [],
    ContextsFromContent: { contexts: ['kind-kubby-dev'], currentContext: 'kind-kubby-dev' },
    ConnectedClusters: [{ id: 'cluster-1', name: 'kind-kubby-dev', active: true }],
    ListPortForwards: [],
    StartExec: '/bin/bash',
    ListNamespaces: [
        { name: 'default', status: 'Active' },
        { name: 'payments', status: 'Active' },
    ],
    SidebarCounts: [
        { view: 'nodes', count: 2, errors: 0 },
        { view: 'namespaces', count: 2, errors: 0 },
        { view: 'pods', count: 6, errors: 1 },
        { view: 'deployments', count: 2, errors: 1 },
        { view: 'services', count: 2, errors: 0 },
        { view: 'helm', count: 1, errors: 0 },
    ],
    ListHelmReleases: [
        { namespace: 'payments', name: 'checkout', revision: '4', status: 'deployed', updated: '3m', isError: false, isPending: false },
    ],
    HelmGet: {
        name: 'checkout', namespace: 'payments', revision: 4, status: 'deployed', chart: 'checkout-2.4.0', appVersion: '1.8.2',
        values: 'replicaCount: 2\n', manifest: 'apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: checkout\n', notes: 'Checkout is ready.',
    },
    HelmReleaseResources: [
        { apiVersion: 'apps/v1', kind: 'Deployment', refKind: 'Deployment', namespace: 'payments', name: 'checkout', status: '2/2 ready', health: 'healthy', ready: true },
        { apiVersion: 'v1', kind: 'Service', refKind: 'Service', namespace: 'payments', name: 'checkout', status: 'Present', health: 'unknown', ready: false },
    ],
    HelmSnapshot: {
        detail: {
            name: 'checkout', namespace: 'payments', revision: 4, status: 'deployed', chart: 'checkout-2.4.0', appVersion: '1.8.2',
            values: 'replicaCount: 2\n', manifest: 'apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: checkout\n', notes: 'Checkout is ready.',
        },
        resources: [
            { apiVersion: 'apps/v1', kind: 'Deployment', refKind: 'Deployment', namespace: 'payments', name: 'checkout', status: '2/2 ready', health: 'healthy', ready: true },
            { apiVersion: 'v1', kind: 'Service', refKind: 'Service', namespace: 'payments', name: 'checkout', status: 'Present', health: 'unknown', ready: false },
        ],
    },
    HelmHistory: [
        { revision: 4, status: 'deployed', chart: 'checkout-2.4.0', updated: '3m', description: 'Upgrade complete' },
        { revision: 3, status: 'superseded', chart: 'checkout-2.3.0', updated: '2d', description: 'Upgrade complete' },
    ],
    ListHelmRepos: [
        { name: 'team-charts', url: 'https://charts.example.test', authenticated: true },
    ],
    SearchCharts: [
        { name: 'nginx', normalizedName: 'nginx', repo: 'bitnami', repoURL: 'https://charts.bitnami.com/bitnami', sourceID: '', version: '18.2.1', appVersion: '1.27.1', description: 'Web server', stars: 2200 },
    ],
    BrowseHelmRepo: [
        { name: 'checkout', normalizedName: 'checkout', repo: 'team-charts', repoURL: 'https://charts.example.test', sourceID: 'team-charts', version: '2.4.0', versions: ['2.4.0', '2.3.0'], appVersion: '1.8.2', description: 'Checkout service' },
    ],
    ChartDetails: { versions: ['18.2.1', '18.1.0'], readme: '# nginx', homeURL: 'https://nginx.org', maintainers: ['Kubby Test'], links: [] },
    ChartDefaultValues: 'replicaCount: 1\n',
    HelmInstallPreview: { current: '', proposed: 'apiVersion: v1\nkind: Service\n', chartDigest: 'sha256:test-chart', permissions: { permitted: true, denied: 0, unknown: 0, requirements: [] } },
    HelmUpgradePreview: { current: 'replicaCount: 1\n', proposed: 'replicaCount: 2\n', releaseRevision: 4, valuesDigest: 'sha256:test-values', permissions: { permitted: true, denied: 0, unknown: 0, requirements: [] } },
    PlanApplyPermissions: { permitted: true, denied: 0, unknown: 0, requirements: [] },
    PlanDrainPermissions: { permitted: true, denied: 0, unknown: 0, requirements: [] },
    PlanHelmPermissions: { permitted: true, denied: 0, unknown: 0, requirements: [] },
    CustomKinds: { kinds: [], overflow: 0, total: 0 },
    CanI: {
        verbs: {
            get: true,
            create: true,
            update: true,
            patch: true,
            delete: true,
            scale: true,
            logs: true,
            exec: true,
            portforward: true,
        },
    },
    OverviewSnapshot: {
        stats: { nodes: 2, namespaces: 2, pods: 6, deployments: 2, podsAvailable: true },
        failingPods: [
            { namespace: 'payments', name: 'checkout-7b8d9f-2kw7p', status: 'CrashLoopBackOff', restarts: 7 },
        ],
        nodeMetrics: [
            { name: 'kubby-control-plane', cpuMilli: 680, cpuCapacity: 4000, memMi: 2970, memCapacity: 8192 },
            { name: 'kubby-worker', cpuMilli: 940, cpuCapacity: 4000, memMi: 3380, memCapacity: 8192 },
        ],
        nodeStatus: [
            { name: 'kubby-control-plane', ready: true, schedulable: false, pods: 3, pressure: '', version: 'v1.34.0' },
            { name: 'kubby-worker', ready: true, schedulable: true, pods: 3, pressure: '', version: 'v1.34.0' },
        ],
        topPods: [
            { namespace: 'payments', name: 'api-6df7fdd9f8-4zj8g', cpuMilli: 420, memMi: 384 },
            { namespace: 'default', name: 'web-5cf9d8b8d6-hv2lk', cpuMilli: 270, memMi: 211 },
        ],
        events: [
            { type: 'Warning', object: 'Pod/checkout-7b8d9f-2kw7p', reason: 'BackOff', age: '2m', message: 'Back-off restarting failed container', count: 7, isWarn: true },
            { type: 'Normal', object: 'Deployment/web', reason: 'ScalingReplicaSet', age: '8m', message: 'Scaled up replica set web-5cf9d8b8d6 to 2', count: 1, isWarn: false },
        ],
        warnings: [],
    },
    ClusterStructure: {
        scope: 'All namespaces',
        summary: { nodes: 2, namespaces: 2, pods: 3, unhealthy: 1 },
        entries: [
            {
                kind: 'Ingress',
                refKind: 'Ingress',
                name: 'shop',
                namespace: 'payments',
                warning: '',
                services: [
                    {
                        name: 'checkout',
                        namespace: 'payments',
                        type: 'ClusterIP',
                        routes: ['shop.test/checkout → 8080'],
                        warning: '',
                        workloads: [
                            {
                                kind: 'Deployment',
                                name: 'checkout',
                                namespace: 'payments',
                                status: '1 / 2 ready',
                                isError: true,
                                pods: [
                                    { name: 'checkout-7b8d9f-2kw7p', namespace: 'payments', status: 'CrashLoopBackOff', ready: '0/1', restarts: 7, node: 'kubby-worker', isError: true },
                                    { name: 'checkout-7b8d9f-v5lhn', namespace: 'payments', status: 'Running', ready: '1/1', restarts: 0, node: 'kubby-worker', isError: false },
                                ],
                            },
                        ],
                    },
                ],
            },
        ],
        internal: [
            {
                name: 'metrics', namespace: 'default', type: 'ClusterIP', routes: [], warning: '',
                workloads: [{
                    kind: 'Deployment', name: 'metrics', namespace: 'default', status: '1 / 1 ready', isError: false,
                    pods: [{ name: 'metrics-5c947c7b7c-lm2qp', namespace: 'default', status: 'Running', ready: '1/1', restarts: 0, node: 'kubby-control-plane', isError: false }],
                }],
            },
        ],
        unexposed: [],
        warnings: [],
    },
    PodsPage: {
        pods: [
            { namespace: 'payments', name: 'api-6df7fdd9f8-4zj8g', status: 'Running', ready: '1/1', restarts: 0, podIP: '10.244.1.7', node: 'kubby-worker', age: '12m', isError: false },
            { namespace: 'payments', name: 'checkout-7b8d9f-2kw7p', status: 'CrashLoopBackOff', ready: '0/1', restarts: 7, podIP: '10.244.1.8', node: 'kubby-worker', age: '9m', isError: true },
        ],
        metrics: [
            { namespace: 'payments', name: 'api-6df7fdd9f8-4zj8g', cpuMilli: 90, memMi: 128 },
            { namespace: 'payments', name: 'checkout-7b8d9f-2kw7p', cpuMilli: 4, memMi: 32 },
        ],
        page: { continue: '', remaining: 0 },
    },
    ListNodes: [
        { name: 'kubby-control-plane', status: 'Ready', role: 'control-plane', version: 'v1.34.0', age: '3d', isError: false },
        { name: 'kubby-worker', status: 'Ready', role: 'worker', version: 'v1.34.0', age: '3d', isError: false },
    ],
    GetDetail: {
        kind: 'Pod', name: 'api-6df7fdd9f8-4zj8g', namespace: 'payments', created: '2026-08-12T03:20:00Z', age: '12m',
        labels: { app: 'api', tier: 'backend' }, annotations: {},
        info: [{ label: 'Status', value: 'Running' }, { label: 'Node', value: 'kubby-worker' }],
    },
    GetDrawerSnapshotOwned: {
        detail: {
            kind: 'Pod', name: 'api-6df7fdd9f8-4zj8g', namespace: 'payments', created: '2026-08-12T03:20:00Z', age: '12m',
            labels: { app: 'api', tier: 'backend' }, annotations: {},
            info: [{ label: 'Status', value: 'Running' }, { label: 'Node', value: 'kubby-worker' }],
        },
        events: [{ type: 'Normal', reason: 'Started', age: '12m', message: 'Started container api', count: 1 }],
        relation: null,
        nodePods: [],
        namespaceInfo: [],
        sectionErrors: {},
    },
    GetYAML: 'apiVersion: v1\nkind: Pod\nmetadata:\n  name: api-6df7fdd9f8-4zj8g\n  namespace: payments\nspec:\n  containers:\n    - name: api\n      image: example.invalid/api:v1\n',
    ListEvents: [{ type: 'Normal', reason: 'Started', age: '12m', message: 'Started container api', count: 1 }],
    PodContainers: ['api'],
    PodLogs: '2026-08-12T03:20:01Z server listening on :8080\n',
    GetAIConfig: { provider: '', endpoint: '', model: '', language: 'auto', hasApiKey: false },
    GetAIStatus: { configured: false, provider: '', model: '' },
    InvestigateResource: JSON.stringify({
        kind: 'Pod', namespace: 'payments', name: 'checkout-7b8d9f-2kw7p', state: 'critical',
        summary: 'checkout was OOMKilled', observedAt: '2026-08-21T09:30:00Z',
        findings: [{
            id: 'terminated-checkout', severity: 'critical', title: 'checkout was OOMKilled',
            explanation: 'The container exceeded its memory limit and the kernel terminated it.', confidence: 'high',
            evidence: ['reason: OOMKilled', 'restartCount: 7'],
        }],
        timeline: [
            { at: '2026-08-21T09:29:00Z', age: '1m', source: 'Container', title: 'checkout was OOMKilled', detail: 'exit code 137', severity: 'critical' },
            { at: '2026-08-21T09:20:00Z', age: '10m', source: 'Pod', title: 'Pod created', detail: 'Scheduled phase: Running', severity: 'info' },
        ],
        related: [
            { kind: 'Deployment', namespace: 'payments', name: 'checkout', role: 'Rollout owner', status: 'controls ReplicaSet checkout-7b8d9f' },
            { kind: 'Service', namespace: 'payments', name: 'checkout', role: 'Selects this Pod', status: '1 ready endpoints' },
        ],
        actions: [
            { id: 'logs-pod', label: 'Inspect current and previous logs', description: 'Confirm the closest application-level failure.', action: 'logs', kind: 'Pod', namespace: 'payments', name: 'checkout-7b8d9f-2kw7p' },
            { id: 'sizing-pod', label: 'Check memory sizing', description: 'Compare usage, requests and limits.', action: 'sizing', kind: 'Pod', namespace: 'payments', name: 'checkout-7b8d9f-2kw7p' },
        ],
        limitations: ['Kubby is agent-less: the timeline is not a complete historical audit log.'],
    }),
    SaveIncidentReport: '/tmp/kubby-incident-pod-checkout.md',
    AppVersion: 'Kubby 0.2.0-test linux/amd64',
    NetworkFlows: { ingresses: [], services: [], routedCount: 0, endpointCount: 0, brokenCount: 0 },
    Sizing: {
        totals: { pods: 0, containers: 0, cpuRequest: 0, cpuUsage: 0, memRequest: 0, memUsage: 0 },
        nodes: 2, allocCpu: 8000, allocMem: 16384, cpuReservedPct: 0, memReservedPct: 0,
        metricsAvailable: true, namespaces: [], containers: [], advice: [], note: '',
    },
};

export async function installWailsMock(page, overrides = {}) {
    await page.addInitScript(({ fixtures, responseOverrides }) => {
        const clone = (value) => value === undefined ? undefined : JSON.parse(JSON.stringify(value));
        const responses = { ...fixtures, ...responseOverrides };
        const calls = [];
        const listeners = new Map();
        const deferred = new Map();
        const app = new Proxy({}, {
            get(_target, method) {
                return (...args) => {
                    calls.push({ method: String(method), args: clone(args) });
                    const response = Object.prototype.hasOwnProperty.call(responses, method)
                        ? responses[method]
                        : [];
                    if (response && typeof response === 'object' && response.__error) {
                        return Promise.reject(new Error(String(response.__error)));
                    }
                    if (response && typeof response === 'object' && response.__deferred) {
                        return new Promise((resolve, reject) => {
                            deferred.set(String(response.__deferred), { resolve, reject });
                        });
                    }
                    return Promise.resolve(clone(response));
                };
            },
        });

        window.__wailsMock = {
            calls,
            clipboardText: '',
            setResponse(method, value) { responses[method] = clone(value); },
            resolveDeferred(name, value) {
                const pending = deferred.get(name);
                if (!pending) throw new Error(`No deferred Wails call named ${name}`);
                deferred.delete(name);
                pending.resolve(clone(value));
            },
            rejectDeferred(name, message) {
                const pending = deferred.get(name);
                if (!pending) throw new Error(`No deferred Wails call named ${name}`);
                deferred.delete(name);
                pending.reject(new Error(String(message)));
            },
            emit(name, ...args) {
                for (const listener of listeners.get(name) ?? []) listener(...args);
            },
        };
        window.go = { main: { App: app } };
        window.runtime = {
            EventsOnMultiple(name, callback) {
                const group = listeners.get(name) ?? [];
                group.push(callback);
                listeners.set(name, group);
                return () => {
                    listeners.set(name, (listeners.get(name) ?? []).filter((item) => item !== callback));
                };
            },
            EventsOff(...names) { for (const name of names) listeners.delete(name); },
            EventsOffAll() { listeners.clear(); },
            EventsEmit(name, ...args) {
                for (const listener of listeners.get(name) ?? []) listener(...args);
            },
            BrowserOpenURL(url) { calls.push({ method: 'BrowserOpenURL', args: [url] }); },
            ClipboardGetText() {
                calls.push({ method: 'ClipboardGetText', args: [] });
                return Promise.resolve(window.__wailsMock.clipboardText);
            },
            ClipboardSetText(value) {
                calls.push({ method: 'ClipboardSetText', args: [value] });
                window.__wailsMock.clipboardText = String(value);
                return Promise.resolve(true);
            },
            LogPrint() {}, LogTrace() {}, LogDebug() {}, LogInfo() {}, LogWarning() {}, LogError() {}, LogFatal() {},
        };
    }, { fixtures: DEFAULT_FIXTURES, responseOverrides: overrides });
}

export async function connectDashboard(page, options = {}) {
    await installWailsMock(page, options.overrides);
    if (options.theme) {
        await page.addInitScript((theme) => localStorage.setItem('kubby-theme', theme), options.theme);
    }
    await page.goto('/');
    await page.getByRole('button', { name: 'Paste kubeconfig' }).click();
    // Pasted kubeconfig uses the shared CodeMirror YAML editor. Drive the
    // editable surface so E2E follows the same editor-handle path as production.
    await page.locator('#paste-area .cm-content').fill('apiVersion: v1\nkind: Config\ncurrent-context: kind-kubby-dev');
    await page.locator('#btn-load-paste').click();
    await page.locator('#context-row').waitFor({ state: 'visible' });
    await page.locator('#btn-connect').click();
    await page.locator('#dashboard').waitFor({ state: 'visible' });
    await page.locator('#cluster-health-title').filter({ hasText: options.healthTitle ?? '1 pod needs attention' }).waitFor();
}

export function collectPageErrors(page) {
    const errors = [];
    page.on('pageerror', (error) => errors.push(error.message));
    return errors;
}
