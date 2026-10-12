import { expect, test } from '@playwright/test';
import { connectDashboard } from './wails-mock.js';

async function calls(page, method) {
    return page.evaluate((name) => window.__wailsMock.calls.filter((call) => call.method === name), method);
}
async function openPod(page) {
    await page.locator('.nav-item[data-view="pods"]').click();
    await page.locator('#pods-body tr', { hasText: 'api-6df7fdd9f8-4zj8g' }).click();
    await expect(page.locator('#drawer')).toBeVisible();
}
async function openRelease(page) {
    const nav = page.locator('.nav-item[data-view="helm"]');
    const group = nav.locator('xpath=ancestor::div[contains(@class,"nav-section")]');
    if (await group.evaluate((element) => element.classList.contains('collapsed'))) await group.locator('.nav-group').click();
    await nav.click();
    await page.locator('.helm-open-release').click();
    await expect(page.locator('#helm-meta')).toContainText('revision 4');
}

test.afterEach(async ({ page }) => {
    expect(await page.evaluate(() => window.__wailsMock?.unexpectedCalls ?? [])).toEqual([]);
});

test('FR-14: owned EOF stops Follow, retains final batch and ignores stale completion', async ({ page }) => {
    await connectDashboard(page);
    await openPod(page);
    await page.getByRole('tab', { name: 'Logs', exact: true }).click();
    await expect(page.locator('#logs-follow')).toBeEnabled();
    await page.locator('#logs-follow').check();
    await expect.poll(async () => (await calls(page, 'StartLogStream')).length).toBe(1);
    const oldID = (await calls(page, 'StartLogStream'))[0].args[3];
    await page.locator('#logs-follow').uncheck();
    await page.locator('#logs-follow').check();
    await expect.poll(async () => (await calls(page, 'StartLogStream')).length).toBe(2);
    const newID = (await calls(page, 'StartLogStream'))[1].args[3];
    expect((await calls(page,'StopLogStream'))[0].args).toEqual([oldID]);
    await page.evaluate(({ oldID, newID }) => {
        window.__wailsMock.emit('logclosed', { streamId: oldID });
        window.__wailsMock.emit('loglines', { streamId: newID, lines: ['final synthetic line'] });
    }, { oldID, newID });
    await expect(page.locator('#logs-follow')).toBeChecked();
    await page.evaluate((streamId) => window.__wailsMock.emit('logclosed', { streamId }), newID);
    await expect(page.locator('#logs-follow')).not.toBeChecked();
    await expect(page.locator('#logs-view')).toContainText('final synthetic line');
    await page.evaluate((streamId) => window.__wailsMock.emit('loglines', { streamId, lines: ['late invalid line'] }), newID);
    await expect(page.locator('#logs-view')).not.toContainText('late invalid line');
});

test('FR-15: reviewed evidence stays exact through answer, follow-up, copy and provider failure', async ({ page }) => {
    const evidence = 'Pod synthetic: missing claim; credentials [REDACTED]';
    await connectDashboard(page, { overrides: {
        GetAIStatus: { configured: true, provider: 'ollama', label: 'Ollama (local)', model: 'synthetic', local: true },
        AIResourceContext: { text: evidence, events: 1, logLines: 0, logContainers: 0, hasYAML: true },
        AskAboutResource: 'Create the missing claim.',
    } });
    await openPod(page);
    await page.getByRole('tab', { name: 'Ask AI', exact: true }).click();
    await page.locator('#ai-view-context').click();
    await expect(page.locator('.ai-context-dump')).toHaveText(evidence);
    await page.locator('#modal-ok').click();
    await page.locator('#ai-input').fill('Why Pending?');
    await page.locator('#btn-ai-send').click();
    await expect(page.locator('.ai-answer')).toContainText('Create the missing claim.');
    await page.locator('.ai-copy').click();
    await expect.poll(async () => (await calls(page, 'ClipboardSetText')).at(-1)?.args[0]).toBe('Create the missing claim.');
    await page.evaluate(() => window.__wailsMock.setResponse('AskAboutResource', { __error: 'synthetic provider unavailable' }));
    await page.locator('#ai-input').fill('What next?');
    await page.locator('#btn-ai-send').click();
    await expect(page.locator('#ai-thread')).toContainText('synthetic provider unavailable');
    const requests = await calls(page, 'AskAboutResource');
    expect(requests).toHaveLength(2);
    expect(requests[0].args.slice(0, 4)).toEqual(['Pod', 'payments', 'api-6df7fdd9f8-4zj8g', evidence]);
    expect(requests[1].args[3]).toBe(evidence);
    expect(requests[1].args[4]).toEqual([
        { role: 'user', content: 'Why Pending?' },
        { role: 'assistant', content: 'Create the missing claim.' },
        { role: 'user', content: 'What next?' },
    ]);
    await expect(page.locator('#btn-ai-send')).toBeEnabled();
});

test('FR-15: resource switch discards a late AI answer', async ({ page }) => {
    await connectDashboard(page, { overrides: {
        GetAIStatus: { configured: true, label: 'Synthetic provider', model: 'synthetic' },
        AskAboutResource: { __deferred: 'old-answer' },
    } });
    await openPod(page);
    await page.getByRole('tab', { name: 'Ask AI', exact: true }).click();
    await expect(page.locator('#btn-ai-send')).toBeEnabled();
    await page.locator('#ai-input').fill('old question');
    await page.locator('#btn-ai-send').click();
    await expect.poll(async () => (await calls(page, 'AskAboutResource')).length).toBe(1);
    await page.keyboard.press('Escape');
    await page.locator('#pods-body tr', { hasText: 'checkout-7b8d9f-2kw7p' }).click();
    await page.getByRole('tab', { name: 'Ask AI', exact: true }).click();
    await page.evaluate(() => window.__wailsMock.resolveDeferred('old-answer', 'wrong resource answer'));
    await expect(page.locator('#ai-thread')).not.toContainText('wrong resource answer');
    await expect(page.locator('#btn-ai-send')).toBeEnabled();
});

test('FR-24: install requires exact preview, then selects namespace and opens release', async ({ page }) => {
    await connectDashboard(page);
    await openRelease(page);
    await page.keyboard.press('Escape');
    await page.locator('[data-helm-section="catalog"]').click();
    await page.locator('#chart-query').fill('nginx');
    await page.locator('#chart-search-go').click();
    await page.locator('#chart-results .chart-item .btn-primary').click();
    await expect(page.locator('#inst-ver')).toHaveValue('18.2.1');
    await page.locator('#inst-name').fill('synthetic-release');
    await page.locator('#inst-ns').fill('synthetic-namespace');
    await expect(page.locator('#modal-ok')).toBeDisabled();
    await page.locator('#inst-preview').click();
    await expect(page.locator('#modal-ok')).toBeEnabled();
    await page.locator('#inst-values .cm-content').fill('replicaCount: 3');
    await expect(page.locator('#modal-ok')).toBeDisabled();
    expect(await calls(page, 'HelmInstallOwned')).toHaveLength(0);
    await page.locator('#inst-preview').click(); // hide the previous preview
    await page.locator('#inst-preview').click();
    await expect(page.locator('#modal-ok')).toBeEnabled();
    await page.locator('#modal-ok').click();
    await expect.poll(async () => (await calls(page, 'HelmInstallOwned')).length).toBe(1);
    expect((await calls(page, 'HelmInstallOwned'))[0].args).toEqual([
        'cluster-1', 'synthetic-namespace', 'synthetic-release', 'https://charts.bitnami.com/bitnami', '', 'nginx', '18.2.1', 'replicaCount: 3', 'sha256:test-chart',
    ]);
    await expect(page.locator('#namespace-select')).toHaveValue('synthetic-namespace');
    await expect(page.locator('#modal-title')).toHaveText('Release · synthetic-release');
});

test('FR-23/35: upgrade rejects denied/failed previews and changed values before exact dispatch', async ({ page }) => {
    await connectDashboard(page);
    await openRelease(page);
    await page.locator('#helm-upgrade-release').click();
    await expect(page.locator('#helm-values .cm-content')).toContainText('replicaCount: 2');
    await page.evaluate(() => window.__wailsMock.setResponse('HelmUpgradePreview', {
        current: '', proposed: '', releaseRevision: 4, valuesDigest: 'sha256:reviewed', permissions: { permitted: false, denied: 1, requirements: [] },
    }));
    await page.locator('#helm-preview-btn').click();
    await expect(page.locator('#helm-preview-status')).toContainText('denied');
    await expect(page.locator('#modal-ok')).toBeDisabled();
    await page.locator('#helm-preview-btn').click();
    await page.evaluate(() => window.__wailsMock.setResponse('HelmUpgradePreview', { __error: 'synthetic preview failed' }));
    await page.locator('#helm-preview-btn').click();
    await expect(page.locator('#helm-upg-diff')).toContainText('synthetic preview failed');
    await expect(page.locator('#modal-ok')).toBeDisabled();
    await page.locator('#helm-preview-btn').click();
    await page.evaluate(() => window.__wailsMock.setResponse('HelmUpgradePreview', {
        current: 'replicaCount: 2', proposed: 'replicaCount: 3', releaseRevision: 4, valuesDigest: 'sha256:reviewed', permissions: { permitted: true, denied: 0, requirements: [] },
    }));
    await page.locator('#helm-preview-btn').click();
    await expect(page.locator('#modal-ok')).toBeEnabled();
    await page.locator('#helm-values .cm-content').fill('replicaCount: 3');
    await expect(page.locator('#modal-ok')).toBeDisabled();
    await page.locator('#helm-preview-btn').click();
    await page.locator('#helm-preview-btn').click();
    await expect(page.locator('#modal-ok')).toBeEnabled();
    await page.locator('#modal-ok').click();
    await expect.poll(async () => (await calls(page, 'HelmUpgradeValuesOwned')).length).toBe(1);
    expect((await calls(page, 'HelmUpgradeValuesOwned'))[0].args).toEqual(['cluster-1', 'payments', 'checkout', 'replicaCount: 3', 4, 'sha256:reviewed']);
    await expect(page.locator('#modal-title')).toHaveText('Release · checkout');
});

for (const [action, button, method] of [
    ['test', '#helm-run-tests', 'HelmTestOwned'],
    ['uninstall', '#helm-uninstall-release', 'HelmUninstallOwned'],
    ['rollback', '.helm-rollback-btn', 'HelmRollbackOwned'],
]) {
    test(`FR-23: ${action} dispatches only after confirmation and surfaces failure`, async ({ page }) => {
        await connectDashboard(page, { overrides: { [method]: { __error: `synthetic ${action} failure` } } });
        await openRelease(page);
        if (action === 'rollback') {
            await page.locator('[data-htab="history"]').click();
            await expect(page.locator('.helm-rollback-btn')).toHaveCount(1);
        }
        await page.locator(button).click();
        await expect(page.locator('#dialog')).toBeVisible();
        await page.locator('#dialog-cancel').click();
        expect(await calls(page, method)).toHaveLength(0);
        await page.locator(button).click();
        await page.locator('#dialog-ok').click();
        await expect.poll(async () => (await calls(page, method)).length).toBe(1);
        expect((await calls(page, method))[0].args.slice(0, 3)).toEqual(['cluster-1', 'payments', 'checkout']);
        await expect(page.locator('#dialog-message')).toContainText(`synthetic ${action} failure`);
    });
}

test('FR-17: Import cancellation dispatches nothing, approved apply preserves editor text and reports failure', async ({ page }) => {
    await connectDashboard(page, { overrides: { ApplyYAMLOwned: { __error: 'synthetic apply conflict' } } });
    await page.locator('#btn-import').click();
    await page.locator('#modal-yaml .cm-content').fill('apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: synthetic');
    await page.locator('#modal-cancel').click();
    expect(await calls(page, 'ApplyYAMLOwned')).toHaveLength(0);
    await page.locator('#btn-import').click();
    const yaml = 'apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: synthetic\n  namespace: payments';
    await page.locator('#modal-yaml .cm-content').fill(yaml);
    await page.locator('#modal-ok').click();
    await expect(page.locator('#modal-error')).toContainText('synthetic apply conflict');
    expect((await calls(page, 'ApplyYAMLOwned'))[0].args).toEqual(['cluster-1', yaml]);
    await expect(page.locator('#modal-yaml .cm-content')).toContainText('name: synthetic');
});

test('FR-15: clipboard false result reports failure rather than Copied', async ({ page }) => {
    await connectDashboard(page, { overrides: {
        GetAIStatus: { configured: true, label: 'Synthetic provider', model: 'synthetic' },
        AskAboutResource: 'synthetic answer', ClipboardSetText: false,
    } });
    await openPod(page);
    await page.getByRole('tab', { name: 'Ask AI', exact: true }).click();
    await expect(page.locator('#btn-ai-send')).toBeEnabled();
    await page.locator('#ai-input').fill('question');
    await page.locator('#btn-ai-send').click();
    await page.locator('.ai-copy').click();
    await expect(page.locator('#dialog-message')).toContainText('Could not copy');
    await expect(page.locator('.ai-copy')).toHaveText('Copy');
});

test('FR-24: late install preview cannot approve changed namespace', async ({ page }) => {
    await connectDashboard(page, { overrides: { HelmInstallPreview: { __deferred: 'old-preview' } } });
    await openRelease(page); await page.keyboard.press('Escape');
    await page.locator('[data-helm-section="catalog"]').click();
    await page.locator('#chart-query').fill('nginx'); await page.locator('#chart-search-go').click();
    await page.locator('#chart-results .chart-item .btn-primary').click();
    await page.locator('#inst-name').fill('synthetic');
    await page.locator('#inst-ns').fill('first');
    await page.locator('#inst-preview').click();
    await expect.poll(async () => (await calls(page, 'HelmInstallPreview')).length).toBe(1);
    await page.locator('#inst-ns').fill('second');
    await page.evaluate(() => window.__wailsMock.resolveDeferred('old-preview', {
        current: '', proposed: 'synthetic', chartDigest: 'sha256:old', permissions: { permitted: true, requirements: [] },
    }));
    await expect(page.locator('#inst-status')).toContainText('changed while previewing');
    await expect(page.locator('#modal-ok')).toBeDisabled();
    expect(await calls(page, 'HelmInstallOwned')).toHaveLength(0);
});

test('FR-43: missing PVC headline leads known blocker while node fit remains separate', async ({ page }) => {
    await connectDashboard(page, { overrides: {
        OverviewSnapshot: {stats:{nodes:1,namespaces:1,pods:1,deployments:0,podsAvailable:true},failingPods:[{namespace:'payments',name:'api-6df7fdd9f8-4zj8g',status:'Pending',restarts:0}],nodeMetrics:[],nodeStatus:[],topPods:[],events:[],warnings:[]},
        ExplainPodScheduling: {
        namespace: 'payments', name: 'api-6df7fdd9f8-4zj8g', verdict: 'blocked',
        headline: 'Storage blocks scheduling: PersistentVolumeClaim data does not exist.',
        nodesFit: 1, nodesTotal: 1, nodes: [], reasons: [], findings: [], requests: [], limits: [], warnings: [], events: [],
    } } });
    await page.locator('#overview-errors-body').getByRole('button',{name:'Why Pending?'}).click();
    await expect(page.locator('.sched-verdict')).toContainText('Storage blocks scheduling');
    await expect(page.locator('.sched-headline')).toContainText('data does not exist');
    await expect(page.locator('.checks-meta')).toContainText('1 of 1 nodes fit');
    await expect(page.locator('.sched-verdict')).not.toContainText('A node does fit');
});

test('FR-24: chart links allow web URLs and block custom schemes and URL credentials', async ({ page }) => {
    await connectDashboard(page, { overrides: { ChartDetails: {
        versions: ['18.2.1'], readme: 'synthetic', homeURL: 'custom-handler:untrusted', maintainers: [],
        links: [{ name: 'Unsafe', url: 'https://synthetic:noncredential@example.test' }],
    } } });
    await openRelease(page); await page.keyboard.press('Escape');
    await page.locator('[data-helm-section="catalog"]').click();
    await page.locator('#chart-query').fill('nginx'); await page.locator('#chart-search-go').click();
    await page.locator('#chart-results .chart-item .btn-primary').click();
    await expect(page.locator('#inst-links')).not.toContainText('Unsafe');
    await expect(page.locator('.ext-link[data-url^="custom-handler:"]')).toHaveCount(0);
    await page.evaluate(() => {
        const button = document.createElement('button');button.className = 'ext-link';button.dataset.url = 'custom-handler:untrusted';document.body.append(button);button.click();
        button.dataset.url = 'https://example.test/docs';button.click();button.remove();
    });
    expect(await calls(page, 'BrowserOpenURL')).toEqual([{ method: 'BrowserOpenURL', args: ['https://example.test/docs'] }]);
});
