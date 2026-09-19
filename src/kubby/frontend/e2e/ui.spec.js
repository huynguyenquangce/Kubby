import { expect, test } from '@playwright/test';
import { collectPageErrors, connectDashboard } from './wails-mock.js';

const zoomMatrix = [
    ['1920×1080 at 80%', 2400, 1350],
    ['1920×1080 at 100%', 1920, 1080],
    ['1920×1080 at 125%', 1536, 864],
    ['1920×1080 at 150%', 1280, 720],
    ['1920×1080 at 175%', 1097, 617],
    ['1920×1080 at 200%', 960, 540],
    ['1366×768 at 100%', 1366, 768],
    ['1366×768 at 150%', 911, 512],
    ['1366×768 at 175%', 781, 438],
    ['1366×768 at 200%', 683, 384],
    ['narrow mobile shell', 390, 844],
];

async function expectViewportBounded(page) {
    await expect.poll(() => page.evaluate(() => {
            const result = [];
            const root = document.documentElement;
            if (root.scrollWidth > window.innerWidth + 1) {
                result.push(`document ${root.scrollWidth} > viewport ${window.innerWidth}`);
            }
            const selectors = [
                '#dashboard', '.main', '.topbar', '.topbar-primary', '.topbar-actions',
                '#btn-command-palette', '.page-heading', '.page-heading-actions',
                '.page-write-actions', '#view-search', '#content', '#drawer', '#modal',
                '.term-toolbar', '.term-frame', '.modal-foot',
            ];
            const verticallyOwned = new Set([
                '#drawer', '#modal', '.term-toolbar', '.term-frame', '.term-footer', '.modal-foot',
            ]);
            for (const selector of selectors) {
                for (const element of document.querySelectorAll(selector)) {
                    const rect = element.getBoundingClientRect();
                    if (element.hidden || (rect.width === 0 && rect.height === 0)) continue;
                    if (rect.left < -1 || rect.right > window.innerWidth + 1) {
                        result.push(`${selector} [${rect.left}, ${rect.right}] outside ${window.innerWidth}`);
                    }
                    if (verticallyOwned.has(selector) && (rect.top < -1 || rect.bottom > window.innerHeight + 1)) {
                        result.push(`${selector} [${rect.top}, ${rect.bottom}] outside ${window.innerHeight}`);
                    }
                }
            }
            return result;
        }), { timeout: 2_500 }).toEqual([]);
}

async function openNavView(page, target) {
    const mobileToggle = page.locator('#btn-mobile-nav');
    if (await mobileToggle.isVisible() && await mobileToggle.getAttribute('aria-expanded') === 'false') {
        await mobileToggle.click();
    }
    const navItem = page.locator(`.nav-item[data-view="${target}"]`);
    const section = navItem.locator('xpath=ancestor::div[contains(@class,"nav-section")]');
    if (await section.evaluate((element) => element.classList.contains('collapsed'))) {
        await section.locator('.nav-group').click();
    }
    await navItem.click();
}

test('FR-28/FR-38: compact Command Palette trigger is bounded and uses keyboard-only focus emphasis', async ({ page }) => {
    await page.setViewportSize({ width: 500, height: 700 });
    await connectDashboard(page);
    const trigger = page.locator('#btn-command-palette');
    await expect(trigger).toHaveAccessibleName('Search resources, actions, or commands');
    await expect(trigger.locator('.command-search-label')).toHaveText('Search Kubby…');
    const restingBorder = await trigger.evaluate((element) => getComputedStyle(element).borderTopColor);
    await expectViewportBounded(page);

    await trigger.click();
    await expect(page.locator('#palette')).toBeVisible();
    await page.locator('#palette-backdrop').click({ position: { x: 4, y: 4 } });
    expect(await trigger.evaluate((element) => element.matches(':focus-visible'))).toBe(false);

    await trigger.press('Enter');
    await page.keyboard.press('Escape');
    expect(await trigger.evaluate((element) => element.matches(':focus-visible'))).toBe(true);
    await expect.poll(() => trigger.evaluate((element) => getComputedStyle(element).borderTopColor))
        .not.toBe(restingBorder);
    await expect(trigger).toHaveCSS('outline-style', 'none');
    await expect(trigger).toHaveCSS('box-shadow', 'none');
    await expectViewportBounded(page);
});

test('NFR-7: Pods uses Kubernetes pages and keeps only a visible row window in the DOM', async ({ page }) => {
    const pods = Array.from({ length: 200 }, (_, index) => ({
        namespace: 'payments',
        name: `pod-${String(index).padStart(3, '0')}`,
        status: 'Running',
        ready: '1/1',
        restarts: 0,
        podIP: `10.244.1.${index + 1}`,
        node: 'kubby-worker',
        age: '1m',
        isError: false,
    }));
    await page.setViewportSize({ width: 1366, height: 768 });
    await connectDashboard(page, {
        overrides: { PodsPage: { pods, metrics: [], page: { continue: 'next-page', remaining: 1 } } },
    });
    await page.locator('.nav-item[data-view="pods"]').click();
    await expect(page.locator('#view-pods table')).toHaveAttribute('aria-rowcount', '201');
    expect(await page.locator('#pods-body tr:not(.virtual-spacer)').count()).toBeLessThan(80);
    await expect(page.locator('#pods-page-status')).toHaveText('200 loaded · 1 remaining');

    await page.evaluate(() => window.__wailsMock.setResponse('PodsPage', {
        pods: [{ namespace: 'payments', name: 'pod-last', status: 'Pending', ready: '0/1', restarts: 0, podIP: '', node: '', age: '1m', isError: true }],
        metrics: [],
        page: { continue: '', remaining: 0 },
    }));
    await page.locator('#pods-load-more').evaluate((button) => button.click());
    await expect(page.locator('#view-pods table')).toHaveAttribute('aria-rowcount', '202');
    await expect(page.locator('#pods-page-status')).toHaveText('201 loaded · complete');
    await page.locator('#view-filter').fill('pod-last');
    await expect(page.locator('#pods-body tr:not(.virtual-spacer)')).toHaveCount(1);
    await expect(page.locator('#pods-body')).toContainText('pod-last');
});

test('FR-35: an explicit multi-resource permission denial blocks Import YAML before dispatch', async ({ page }) => {
    await page.setViewportSize({ width: 1366, height: 768 });
    await connectDashboard(page, {
        overrides: {
            PlanApplyPermissions: {
                operation: 'apply-yaml', permitted: false, denied: 1, unknown: 0,
                requirements: [{ kind: 'Deployment', namespace: 'payments', verb: 'patch', checked: true, allowed: false, reason: 'Your token cannot patch Deployment in namespace payments.' }],
            },
        },
    });
    await page.locator('#btn-import').click();
    await page.locator('#modal-yaml .cm-content').fill('apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: denied\n  namespace: payments\n');
    await page.locator('#modal-ok').click();
    await expect(page.locator('#modal-error')).toContainText('cannot patch Deployment');
    expect(await page.evaluate(() => window.__wailsMock.calls.filter((call) => call.method === 'ApplyYAMLOwned').length)).toBe(0);
});

test('FR-2/FR-13: pasted kubeconfig enters a populated Overview', async ({ page }) => {
    const pageErrors = collectPageErrors(page);
    await connectDashboard(page);

    await expect(page.locator('#page-title')).toHaveText('Overview');
    await expect(page.locator('#cluster-health-title')).toHaveText('1 pod needs attention');
    await expect(page.locator('#cluster-health-ratio')).toHaveText('5 / 6');
    await expect(page.locator('#stat-nodes')).toHaveText('2');
    await expect(page.locator('#infrastructure-state')).toHaveText('Ready');
    await expect(page.locator('.overview-node-row').filter({ hasText: 'kubby-control-plane' })).not.toHaveClass(/overview-node-row-bad/);
    await expect(page.locator('#overview-errors-body')).toContainText('checkout-7b8d9f-2kw7p');
    const clippedIssueCells = await page.locator('#overview-errors-body tr').evaluate((row) => [...row.cells]
        .map((cell, index) => ({ index, clientWidth: cell.clientWidth, scrollWidth: cell.scrollWidth }))
        // The Pod name deliberately ellipsizes and carries the full value in its title.
        .filter((cell) => cell.index !== 1)
        .filter((cell) => cell.scrollWidth > cell.clientWidth + 1));
    expect(clippedIssueCells).toEqual([]);
    const clippedIssueHeaders = await page.locator('.overview-issues-table thead tr').evaluate((row) => [...row.cells]
        .map((cell, index) => ({ index, clientWidth: cell.clientWidth, scrollWidth: cell.scrollWidth }))
        .filter((cell) => cell.index !== 1)
        .filter((cell) => cell.scrollWidth > cell.clientWidth + 1));
    expect(clippedIssueHeaders).toEqual([]);
    await expect(page.locator('#overview-toppods-body')).toContainText('api-6df7fdd9f8-4zj8g');
    await expect(page.locator('#overview-events-body')).toContainText('BackOff');
    expect(pageErrors).toEqual([]);

    const calls = await page.evaluate(() => window.__wailsMock.calls.map((call) => call.method));
    expect(calls).toEqual(expect.arrayContaining(['ContextsFromContent', 'ConnectWithContent', 'OverviewSnapshot']));
});

test('FR-13: healthy Overview keeps the Attention message readable without overflow', async ({ page }) => {
    await page.setViewportSize({ width: 960, height: 540 });
    await connectDashboard(page, {
        healthTitle: 'Cluster healthy',
        overrides: {
            OverviewSnapshot: {
                stats: { nodes: 0, namespaces: 0, pods: 6, deployments: 0, podsAvailable: true },
                failingPods: [],
                nodeMetrics: [],
                nodeStatus: [],
                topPods: [],
                events: [],
                warnings: [],
            },
        },
    });

    const empty = page.locator('#overview-errors-empty');
    await expect(empty).toBeVisible();
    await expect(empty).toContainText('Nothing needs attention');
    const layout = await empty.evaluate((element) => {
        const message = element.children[1];
        const wrapper = element.parentElement;
        return {
            messageWidth: message.getBoundingClientRect().width,
            wrapperClientHeight: wrapper.clientHeight,
            wrapperScrollHeight: wrapper.scrollHeight,
        };
    });
    expect(layout.messageWidth).toBeGreaterThan(150);
    expect(layout.wrapperScrollHeight).toBeLessThanOrEqual(layout.wrapperClientHeight + 1);
});

test('FR-40: Attention opens evidence-first Incident Studio and safe hand-offs', async ({ page }) => {
    const pageErrors = collectPageErrors(page);
    await page.setViewportSize({ width: 1100, height: 760 });
    await connectDashboard(page);

    await page.locator('#overview-errors-body .issue-diagnose').click();
    await expect(page.locator('#drawer')).toBeVisible();
    await expect(page.getByRole('tab', { name: 'Investigate', exact: true })).toHaveAttribute('aria-selected', 'true');
    await expect(page.locator('#incident-summary')).toHaveText('checkout was OOMKilled');
    await expect(page.locator('#incident-findings')).toContainText('reason: OOMKilled');
    await expect(page.locator('#incident-related')).toContainText('Deployment · checkout');
    await expect(page.locator('#incident-timeline')).toContainText('exit code 137');
    await expect(page.locator('#incident-limits-section')).toBeVisible();

    await page.locator('#btn-incident-export').click();
    await expect.poll(() => page.evaluate(() => window.__wailsMock.calls.filter((call) => call.method === 'SaveIncidentReport').length)).toBe(1);

    await page.getByRole('button', { name: /Inspect current and previous logs/ }).click();
    await expect(page.getByRole('tab', { name: 'Logs', exact: true })).toHaveAttribute('aria-selected', 'true');
    expect(pageErrors).toEqual([]);
});

test('FR-6: dashboard namespace picker filters and changes scope from the keyboard', async ({ page }) => {
    const pageErrors = collectPageErrors(page);
    await connectDashboard(page);

    await page.locator('#namespace-toggle').click();
    await expect(page.locator('#namespace-toggle')).toHaveAttribute('aria-expanded', 'true');
    await expect(page.locator('#namespace-search')).toBeFocused();
    await expect(page.locator('#namespace-options .namespace-option')).toHaveCount(3);

    await page.locator('#namespace-search').fill('pay');
    await expect(page.locator('#namespace-options .namespace-option')).toHaveCount(1);
    await expect(page.locator('#namespace-options')).toContainText('payments');
    await page.keyboard.press('Enter');
    await expect(page.locator('#namespace-current')).toHaveText('payments');
    await expect(page.locator('#namespace-popover')).toBeHidden();

    await page.locator('#namespace-toggle').press('ArrowDown');
    await page.locator('#namespace-search').fill('def');
    await page.keyboard.press('Enter');
    await expect(page.locator('#namespace-current')).toHaveText('default');
    expect(pageErrors).toEqual([]);
});

for (const [label, width, height] of zoomMatrix) {
    test(`FR-38: shell stays usable at ${label}`, async ({ page }) => {
        const pageErrors = collectPageErrors(page);
        await page.setViewportSize({ width, height });
        await connectDashboard(page);
        await expectViewportBounded(page);

        const mobile = width <= 820;
        if (mobile) {
            await expect(page.locator('#btn-mobile-nav')).toBeVisible();
            await page.locator('#btn-mobile-nav').click();
            await expect(page.locator('#btn-mobile-nav')).toHaveAttribute('aria-expanded', 'true');
            await expect(page.locator('#sidebar')).toBeInViewport();
            await expect(page.locator('.main')).toHaveAttribute('inert', '');
            await expect.poll(() => page.evaluate(() => document.querySelector('#sidebar')?.contains(document.activeElement))).toBe(true);
            await page.keyboard.press('Escape');
            await expect(page.locator('#btn-mobile-nav')).toHaveAttribute('aria-expanded', 'false');
            await expect(page.locator('.main')).not.toHaveAttribute('inert', '');
            await expect(page.locator('#btn-mobile-nav')).toBeFocused();
        } else {
            await expect(page.locator('#btn-mobile-nav')).toBeHidden();
        }
        expect(pageErrors).toEqual([]);
    });
}

const criticalSurfaceMatrix = [
    ['390 px mobile', 390, 844],
    ['683 px at 200% zoom', 683, 384],
    ['781 px at 175% zoom', 781, 438],
    ['911 px at 150% zoom', 911, 512],
    ['960 px compact desktop', 960, 540],
    ['1097 px at 175% zoom', 1097, 617],
    ['1280 px at 150% zoom', 1280, 720],
    ['1366 px desktop', 1366, 768],
    ['1536 px at 125% zoom', 1536, 864],
    ['1920 px desktop', 1920, 1080],
    ['2400 px at 80% zoom', 2400, 1350],
];

for (const [label, width, height] of criticalSurfaceMatrix) {
    test(`FR-38: critical surfaces remain viewport-owned at ${label}`, async ({ page }) => {
        test.slow();
        const pageErrors = collectPageErrors(page);
        await page.setViewportSize({ width, height });
        await connectDashboard(page);

        await openNavView(page, 'pods');
        await expect(page.locator('#view-search')).toBeVisible();
        await expectViewportBounded(page);

        await page.locator('#pods-body tr').first().click();
        await expect(page.locator('#drawer')).toBeVisible();
        await expectViewportBounded(page);
        await page.getByRole('tab', { name: 'Terminal', exact: true }).click();
        await expect(page.locator('#term-status')).toContainText('Attached');
        await expect.poll(async () => (await page.locator('.term-frame').boundingBox())?.height ?? 0)
            .toBeGreaterThan(110);
        await expect.poll(async () => (await page.locator('#term-surface .xterm-screen').boundingBox())?.height ?? 0)
            .toBeGreaterThan(40);
        await expectViewportBounded(page);
        await page.locator('#drawer-close').click();

        await page.locator('#btn-settings').click();
        await expect(page.locator('#modal')).toBeVisible();
        await expectViewportBounded(page);
        await page.keyboard.press('Escape');

        await openNavView(page, 'traffic');
        await expectViewportBounded(page);
        await openNavView(page, 'helm');
        await expectViewportBounded(page);
        expect(pageErrors).toEqual([]);
    });
}

test('FR-37: cluster structure supports filtering and resource inspection', async ({ page }) => {
    const pageErrors = collectPageErrors(page);
    await page.setViewportSize({ width: 1100, height: 760 });
    await connectDashboard(page);
    await page.locator('#btn-cluster-structure').click();

    await expect(page.locator('#page-title')).toHaveText('Topology');
    await expect(page.locator('[data-topology-view="structure"]')).toHaveAttribute('aria-selected', 'true');
    await expect(page.locator('#structure-entries .structure-path')).toHaveCount(1);
    await expect(page.locator('#structure-internal .structure-path')).toHaveCount(1);
    await page.locator('#structure-filter').fill('checkout');
    await expect(page.locator('#structure-entries .structure-path')).toBeVisible();
    await expect(page.locator('#structure-internal .structure-path')).toBeHidden();
    await page.getByRole('button', { name: /checkout-7b8d9f-2kw7p/ }).click();
    await expect(page.locator('#structure-inspector-content')).toContainText('checkout-7b8d9f-2kw7p');
    await expect(page.locator('#structure-open-logs')).toBeVisible();
    await expectViewportBounded(page);
    expect(pageErrors).toEqual([]);
});

test('FR-7/FR-14/FR-38: Pod drawer owns the selected resource and stays in the viewport', async ({ page }) => {
    const pageErrors = collectPageErrors(page);
    await page.setViewportSize({ width: 720, height: 720 });
    await connectDashboard(page);
    await page.locator('#btn-mobile-nav').click();
    await page.locator('.nav-item[data-view="pods"]').click();
    await expect(page.locator('#pods-body')).toContainText('api-6df7fdd9f8-4zj8g');
    await page.locator('#pods-body tr', { hasText: 'api-6df7fdd9f8-4zj8g' }).click();

    await expect(page.locator('#drawer')).toBeVisible();
    await expect(page.locator('#drawer-name')).toHaveText('api-6df7fdd9f8-4zj8g');
    await expect(page.locator('#drawer-ns')).toContainText('payments');
    await expect(page.locator('#drawer-tab-logs')).toBeVisible();
    await page.getByRole('tab', { name: 'YAML', exact: true }).click();
    await expect(page.locator('#dpanel-yaml .cm-editor')).toContainText('api-6df7fdd9f8-4zj8g');
    const detailCalls = await page.evaluate(() => window.__wailsMock.calls
        .filter((call) => ['GetDrawerSnapshotOwned', 'GetDetail', 'ListEvents', 'PodsOnNode', 'NamespaceSummary'].includes(call.method))
        .map((call) => call.method));
    expect(detailCalls).toEqual(['GetDrawerSnapshotOwned']);

    const drawerBounds = await page.locator('#drawer').evaluate((element) => {
        const rect = element.getBoundingClientRect();
        return { left: rect.left, right: rect.right, width: rect.width, viewport: window.innerWidth };
    });
    expect(drawerBounds.left).toBeGreaterThanOrEqual(-1);
    expect(drawerBounds.right).toBeLessThanOrEqual(drawerBounds.viewport + 1);
    await page.locator('#drawer-close').click();
    await expect(page.locator('#drawer')).toBeHidden();
    expect(pageErrors).toEqual([]);
});

test('FR-7/NFR-7: closing a drawer cancels its exact snapshot and stale data cannot remount', async ({ page }) => {
    const pageErrors = collectPageErrors(page);
    await connectDashboard(page);
    await page.evaluate(() => window.__wailsMock.setResponse('GetDrawerSnapshotOwned', { __deferred: 'old-drawer' }));
    await page.locator('.nav-item[data-view="pods"]').click();
    await page.locator('#pods-body tr', { hasText: 'api-6df7fdd9f8-4zj8g' }).click();

    const snapshotCall = await page.evaluate(() => window.__wailsMock.calls
        .find((call) => call.method === 'GetDrawerSnapshotOwned'));
    expect(snapshotCall.args[0]).toBe('cluster-1');
    expect(snapshotCall.args[1]).toBeTruthy();

    await page.locator('#drawer-close').click();
    await expect.poll(() => page.evaluate(() => window.__wailsMock.calls
        .filter((call) => call.method === 'CancelDrawerSnapshot').length)).toBe(1);
    const canceledID = await page.evaluate(() => window.__wailsMock.calls
        .find((call) => call.method === 'CancelDrawerSnapshot')?.args[0]);
    expect(canceledID).toBe(snapshotCall.args[1]);

    await page.evaluate(() => window.__wailsMock.setResponse('GetDrawerSnapshotOwned', {
        detail: {
            kind: 'Pod', name: 'checkout-7b8d9f-2kw7p', namespace: 'payments', created: '2026-08-12T03:23:00Z', age: '9m',
            labels: { app: 'checkout' }, annotations: {}, info: [{ label: 'Status', value: 'CrashLoopBackOff' }],
        },
        events: [], relation: null, nodePods: [], namespaceInfo: [], sectionErrors: {},
    }));
    await page.locator('#pods-body tr', { hasText: 'checkout-7b8d9f-2kw7p' }).click();
    await expect(page.locator('#drawer-name')).toHaveText('checkout-7b8d9f-2kw7p');
    await expect(page.locator('#detail-meta')).toContainText('CrashLoopBackOff');

    await page.evaluate(() => window.__wailsMock.rejectDeferred('old-drawer', 'request canceled'));
    await expect(page.locator('#drawer')).toBeVisible();
    await expect(page.locator('#drawer-name')).toHaveText('checkout-7b8d9f-2kw7p');
    await expect(page.locator('#detail-meta')).toContainText('CrashLoopBackOff');
    expect(pageErrors).toEqual([]);
});

test('FR-14: a completed delete cannot close a newer resource drawer', async ({ page }) => {
    await connectDashboard(page);
    await page.evaluate(() => window.__wailsMock.setResponse('DeleteResourceOwned', { __deferred: 'delete-api' }));
    await page.locator('.nav-item[data-view="pods"]').click();
    await page.locator('#pods-body tr', { hasText: 'api-6df7fdd9f8-4zj8g' }).click();
    await page.locator('#btn-delete').click();
    await page.locator('#dialog-ok').click();
    await expect.poll(() => page.evaluate(() => window.__wailsMock.calls
        .filter((call) => call.method === 'DeleteResourceOwned').length)).toBe(1);

    await page.locator('#drawer-close').click();
    await page.locator('#pods-body tr', { hasText: 'checkout-7b8d9f-2kw7p' }).click();
    await expect(page.locator('#drawer-name')).toHaveText('checkout-7b8d9f-2kw7p');
    await page.evaluate(() => window.__wailsMock.resolveDeferred('delete-api'));

    await expect(page.locator('#drawer')).toBeVisible();
    await expect(page.locator('#drawer-name')).toHaveText('checkout-7b8d9f-2kw7p');
});

test('FR-21/FR-38: Pod Terminal auto-attaches Bash-first and keeps session controls owned', async ({ page }) => {
    const pageErrors = collectPageErrors(page);
    await page.setViewportSize({ width: 720, height: 720 });
    await connectDashboard(page);
    await page.locator('#btn-mobile-nav').click();
    await page.locator('.nav-item[data-view="pods"]').click();
    await page.locator('#pods-body tr', { hasText: 'api-6df7fdd9f8-4zj8g' }).click();
    await page.getByRole('tab', { name: 'Terminal', exact: true }).click();

    await expect(page.locator('#term-status')).toHaveText('Attached · /bin/bash');
    await expect(page.locator('#term-container')).toHaveValue('api');
    await expect(page.locator('#term-shell')).toHaveValue('auto');
    await expect(page.locator('#btn-term-start')).toHaveText('Reconnect');
    await expect(page.locator('#btn-term-stop')).toBeVisible();

    const firstSession = await page.evaluate(() => {
        const calls = window.__wailsMock.calls.filter((call) => call.method === 'StartExec');
        return calls.at(-1)?.args[0];
    });
    expect(firstSession).toBeTruthy();
    await page.evaluate((sessionId) => {
        window.__wailsMock.emit('exec-output', { sessionId, data: '\u001b[36mkubby-test\u001b[0m$ ' });
    }, firstSession);
    await expect(page.locator('#term-surface .xterm-rows')).toContainText('kubby-test$');

    await page.keyboard.type('pwd');
    await expect.poll(() => page.evaluate(() => window.__wailsMock.calls.filter((call) => call.method === 'ExecWrite').length)).toBeGreaterThan(0);

    await page.evaluate(() => { window.__wailsMock.clipboardText = 'printf clipboard-ready'; });
    await page.locator('#btn-term-paste').click();
    await expect.poll(() => page.evaluate(() => window.__wailsMock.calls
        .filter((call) => call.method === 'ExecWrite')
        .some((call) => call.args[0] === 'printf clipboard-ready'))).toBe(true);

    await page.locator('#term-shell').selectOption('/bin/sh');
    await expect.poll(() => page.evaluate(() => window.__wailsMock.calls.filter((call) => call.method === 'StartExec').length)).toBe(2);
    const shellRequests = await page.evaluate(() => window.__wailsMock.calls
        .filter((call) => call.method === 'StartExec')
        .map((call) => call.args[4]));
    expect(shellRequests).toEqual(['auto', '/bin/sh']);

    const bounds = await page.locator('.term-frame').evaluate((element) => {
        const rect = element.getBoundingClientRect();
        return { left: rect.left, right: rect.right, width: innerWidth };
    });
    expect(bounds.left).toBeGreaterThanOrEqual(-1);
    expect(bounds.right).toBeLessThanOrEqual(bounds.width + 1);
    await page.locator('#btn-term-stop').click();
    await expect(page.locator('#term-status')).toHaveText('Disconnected by user');
    expect(pageErrors).toEqual([]);
});

test('FR-29: clicking the Kubby brand returns to Overview', async ({ page }) => {
    await connectDashboard(page);
    await openNavView(page, 'pods');
    await expect(page.locator('#page-title')).toHaveText('Pods');
    await page.locator('#btn-brand-home').click();
    await expect(page.locator('#page-title')).toHaveText('Overview');
    await expect(page.locator('#btn-brand-home')).toHaveAccessibleName('Go to Overview');
});

test('FR-33/FR-38: a long Custom Resources group scrolls to its final kind without clipping', async ({ page }) => {
    const kinds = Array.from({ length: 60 }, (_, index) => ({
        refKind: `Widget${String(index + 1).padStart(2, '0')}.example.test`,
        title: `Widget ${String(index + 1).padStart(2, '0')}`,
        namespaced: true,
    }));
    await page.setViewportSize({ width: 683, height: 384 });
    await connectDashboard(page, {
        overrides: { CustomKinds: { kinds, overflow: 0, total: kinds.length } },
    });

    await page.locator('#btn-mobile-nav').click();
    await page.locator('#nav-section-custom .nav-group').click();
    const nav = page.locator('.nav');
    await nav.hover();
    await page.mouse.wheel(0, 20_000);
    const lastKind = page.locator('#nav-custom-items .nav-item').last();
    await expect(lastKind).toBeVisible();
    const geometry = await lastKind.evaluate((element) => {
        const navRect = element.closest('.nav').getBoundingClientRect();
        const itemRect = element.getBoundingClientRect();
        const items = element.parentElement;
        return {
            navBottom: navRect.bottom,
            itemBottom: itemRect.bottom,
            itemsClientHeight: items.clientHeight,
            itemsScrollHeight: items.scrollHeight,
        };
    });
    expect(geometry.itemBottom).toBeLessThanOrEqual(geometry.navBottom + 1);
    expect(geometry.itemsClientHeight).toBe(geometry.itemsScrollHeight);
    await lastKind.click();
    await expect(page.locator('#page-title')).toHaveText('Widget 60');
    await expect(page.locator('#btn-mobile-nav')).toHaveAttribute('aria-expanded', 'false');
});

test('FR-5/FR-38: a wide resource table scrolls inside content without widening the shell', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await connectDashboard(page);
    await openNavView(page, 'pods');

    const content = page.locator('#content');
    const dimensions = await content.evaluate((element) => ({
        clientWidth: element.clientWidth,
        scrollWidth: element.scrollWidth,
        documentWidth: document.documentElement.scrollWidth,
        viewportWidth: window.innerWidth,
    }));
    expect(dimensions.scrollWidth).toBeGreaterThan(dimensions.clientWidth);
    expect(dimensions.documentWidth).toBeLessThanOrEqual(dimensions.viewportWidth + 1);

    await content.hover();
    await page.mouse.wheel(20_000, 0);
    await expect.poll(() => content.evaluate((element) => element.scrollLeft)).toBeGreaterThan(0);

    const contentRight = await content.evaluate((element) => element.getBoundingClientRect().right);
    const lastCellRight = await page.locator('#pods-body tr').first().locator('td').last()
        .evaluate((element) => element.getBoundingClientRect().right);
    expect(lastCellRight).toBeLessThanOrEqual(contentRight + 1);
    await expectViewportBounded(page);
});

test('FR-21: shell discovery failure stops after one attempt and offers Retry', async ({ page }) => {
    await connectDashboard(page, {
        overrides: { StartExec: { __error: 'no supported shell found; this image may be distroless' } },
    });
    await page.locator('.nav-item[data-view="pods"]').click();
    await page.locator('#pods-body tr', { hasText: 'api-6df7fdd9f8-4zj8g' }).click();
    await page.getByRole('tab', { name: 'Terminal', exact: true }).click();

    await expect(page.locator('#term-status')).toContainText('no supported shell found');
    await expect(page.locator('#term-placeholder-title')).toHaveText('Shell unavailable');
    await expect(page.locator('#btn-term-start')).toHaveText('Retry');
    await page.waitForTimeout(150);
    const starts = await page.evaluate(() => window.__wailsMock.calls.filter((call) => call.method === 'StartExec').length);
    expect(starts).toBe(1);
});

test('FR-16: Ctrl+F searches inside YAML and Escape keeps the drawer open', async ({ page }) => {
    const pageErrors = collectPageErrors(page);
    await connectDashboard(page);
    await page.locator('.nav-item[data-view="pods"]').click();
    await page.locator('#pods-body tr', { hasText: 'api-6df7fdd9f8-4zj8g' }).click();
    await page.getByRole('tab', { name: 'YAML', exact: true }).click();

    await page.locator('#dpanel-yaml .cm-content').click();
    await page.keyboard.press('Control+f');
    const searchPanel = page.locator('#dpanel-yaml .cm-panel.cm-search');
    await expect(searchPanel).toBeVisible();
    await expect(searchPanel.locator('input[name="search"]')).toBeFocused();
    await searchPanel.locator('input[name="search"]').pressSequentially('namespace');
    await expect(page.locator('#dpanel-yaml .cm-searchMatch')).toHaveCount(1);

    const panelIsAboveContent = await page.locator('#dpanel-yaml .cm-editor').evaluate((editor) => {
        const panel = editor.querySelector('.cm-panel.cm-search')?.getBoundingClientRect();
        const scroller = editor.querySelector('.cm-scroller')?.getBoundingClientRect();
        return Boolean(panel && scroller && panel.bottom <= scroller.top + 1);
    });
    expect(panelIsAboveContent).toBe(true);
    await page.keyboard.press('Escape');
    await expect(searchPanel).toBeHidden();
    await expect(page.locator('#drawer')).toBeVisible();
    expect(pageErrors).toEqual([]);
});

test('FR-31/FR-38: Settings modal closes with Escape and remains viewport-bounded', async ({ page }) => {
    const pageErrors = collectPageErrors(page);
    await page.setViewportSize({ width: 390, height: 700 });
    await connectDashboard(page);
    await page.locator('#btn-settings').click();

    await expect(page.locator('#modal')).toBeVisible();
    await expect(page.locator('#modal')).toHaveAttribute('aria-labelledby', 'modal-title');
    await expect(page.locator('#modal-title')).toHaveText('Settings');
    await expect(page.locator('#modal-eyebrow')).toHaveText('Application');
    await expect(page.locator('#about-version')).toContainText('0.2.0-test');
    await expect(page.locator('#ai-provider')).toBeFocused();
    const providerBox = await page.locator('#ai-provider').boundingBox();
    const modelBox = await page.locator('#ai-model').boundingBox();
    expect(Math.abs(providerBox.x - modelBox.x)).toBeLessThanOrEqual(2);
    expect(modelBox.y).toBeGreaterThan(providerBox.y);
    const bounds = await page.locator('#modal').evaluate((element) => {
        const rect = element.getBoundingClientRect();
        return { left: rect.left, right: rect.right, top: rect.top, bottom: rect.bottom, width: innerWidth, height: innerHeight };
    });
    expect(bounds.left).toBeGreaterThanOrEqual(-1);
    expect(bounds.right).toBeLessThanOrEqual(bounds.width + 1);
    expect(bounds.top).toBeGreaterThanOrEqual(-1);
    expect(bounds.bottom).toBeLessThanOrEqual(bounds.height + 1);
    await page.keyboard.press('Escape');
    await expect(page.locator('#modal')).toBeHidden();
    expect(pageErrors).toEqual([]);
});

test('FR-31: late Settings data preserves fields the user already edited', async ({ page }) => {
    await connectDashboard(page);
    await page.evaluate(() => window.__wailsMock.setResponse('GetAIConfig', { __deferred: 'settings-config' }));
    await page.locator('#btn-settings').click();
    await page.locator('#ai-model').fill('my-local-draft');

    await page.evaluate(() => window.__wailsMock.resolveDeferred('settings-config', {
        provider: 'openai', endpoint: 'https://api.openai.com', model: 'server-model', language: 'en', hasApiKey: true,
    }));

    await expect(page.locator('#ai-model')).toHaveValue('my-local-draft');
});

test('FR-5: every built-in navigation target renders without a browser exception', async ({ page }) => {
    const pageErrors = collectPageErrors(page);
    await page.setViewportSize({ width: 1440, height: 900 });
    await connectDashboard(page);
    const targets = await page.locator('.nav-item[data-view]').evaluateAll((items) => items.map((item) => item.dataset.view));

    for (const target of targets) {
        await openNavView(page, target);
        await expect(page.locator(`#view-${target}`)).toBeVisible();
        await expect(page.locator('#dash-error')).toBeHidden();
    }
    expect(pageErrors).toEqual([]);
});

test('FR-23/24/25: Helm workspace keeps release, catalog, and repository context together', async ({ page }) => {
    const pageErrors = collectPageErrors(page);
    await page.setViewportSize({ width: 1180, height: 780 });
    await connectDashboard(page);
    await openNavView(page, 'helm');

    await expect(page.locator('#helm-total')).toHaveText('1');
    await expect(page.locator('#helm-deployed')).toHaveText('1');
    await expect(page.locator('#helm-body .helm-release-row')).toHaveCount(1);
    await page.locator('#helm-body .helm-open-release').click();
    await expect(page.locator('#modal-title')).toHaveText('Release · checkout');
    await expect(page.locator('#helm-upgrade-release')).toBeVisible();
    await expect(page.locator('#modal-cancel')).toBeHidden();
    await page.locator('#helm-uninstall-release').click();
    await expect(page.locator('#dialog')).toHaveClass(/dialog-danger/);
    await expect(page.locator('#dialog-cancel')).toBeFocused();
    await page.keyboard.press('Escape');
    await expect(page.locator('#modal')).toBeVisible();
    await page.locator('.helm-tab[data-htab="history"]').click();
    await expect(page.locator('#helm-history')).toContainText('Revision 4');
    await page.keyboard.press('Escape');

    await page.locator('.helm-workspace-tab[data-helm-section="catalog"]').click();
    await page.locator('#chart-query').fill('nginx');
    await page.locator('#chart-search-go').click();
    await expect(page.locator('#chart-results .chart-item')).toContainText('nginx');
    await page.locator('#chart-results .chart-item .btn-primary').click();
    await expect(page.locator('#modal-title')).toHaveText('Install nginx');
    await expect(page.locator('.helm-namespace-note')).toContainText('namespace does not exist');
    await page.keyboard.press('Escape');

    await page.locator('.helm-workspace-tab[data-helm-section="repositories"]').click();
    await expect(page.locator('#helm-panel-repositories')).toContainText('Shared with the Helm CLI');
    await expect(page.locator('#helmrepos-body')).toContainText('Private');
    await page.locator('#btn-repo-add').click();
    await expect(page.locator('#modal-title')).toHaveText('Add Helm repository');
    await expect(page.locator('#modal-description')).toContainText('user-level Helm configuration');
    const repoModal = await page.locator('#modal').boundingBox();
    expect(repoModal.width).toBeLessThanOrEqual(700);
    const userBox = await page.locator('#repo-user').boundingBox();
    const passBox = await page.locator('#repo-pass').boundingBox();
    expect(Math.abs(userBox.y - passBox.y)).toBeLessThanOrEqual(2);
    await page.keyboard.press('Escape');
    await page.locator('#helmrepos-body .repo-browse').click();
    await expect(page.locator('#helm-catalog-source')).toHaveValue('repo:team-charts');
    await expect(page.locator('#chart-results')).toContainText('checkout');
    expect(pageErrors).toEqual([]);
});

test('FR-24: late Helm values keep a draft started while the release loads', async ({ page }) => {
    await connectDashboard(page);
    await openNavView(page, 'helm');
    await page.locator('#helm-body .helm-open-release').click();
    await page.evaluate(() => window.__wailsMock.setResponse('HelmGet', { __deferred: 'helm-values' }));
    await page.locator('#helm-upgrade-release').click();
    await expect(page.locator('#modal-title')).toHaveText('Upgrade values — checkout');
    await page.locator('#helm-values .cm-content').fill('replicaCount: 7\ncustomDraft: true');

    await page.evaluate(() => window.__wailsMock.resolveDeferred('helm-values', {
        name: 'checkout', namespace: 'payments', revision: 4, status: 'deployed',
        values: 'replicaCount: 2\n', manifest: '', notes: '',
    }));

    await expect(page.locator('#helm-values .cm-content')).toContainText('replicaCount: 7');
    await expect(page.locator('#helm-values .cm-content')).toContainText('customDraft: true');
    await expect(page.locator('#helm-preview-status')).toContainText('your draft was kept');
    await expect(page.locator('#helm-preview-btn')).toBeEnabled();
});

test('FR-14: a restarted container points at, and reads, the instance before the restart', async ({ page }) => {
    const pageErrors = collectPageErrors(page);
    await page.setViewportSize({ width: 1440, height: 900 });
    await connectDashboard(page, {
        overrides: {
            PodContainerStates: [
                { name: 'sidecar', ready: true, state: 'Running', restartCount: 0, lastTermination: '', lastTerminationAge: '', hasPrevious: false },
                { name: 'api', ready: false, state: 'Waiting: CrashLoopBackOff', restartCount: 7, lastTermination: 'OOMKilled, exit 137', lastTerminationAge: '3m', hasPrevious: true },
            ],
        },
    });
    await openNavView(page, 'pods');
    await page.locator('#pods-body tr', { hasText: 'checkout-7b8d9f-2kw7p' }).click();
    await page.getByRole('tab', { name: 'Logs', exact: true }).click();
    await expect(page.locator('#logs-view')).toContainText('server listening');
    // The drawer opens on the container that restarted and says why it matters.
    await expect(page.locator('#logs-container')).toHaveValue('api');
    const hint = page.locator('#logs-restart-hint');
    await expect(hint).toContainText('api restarted 7 times · last exit OOMKilled, exit 137, 3m ago');

    await page.evaluate(() => window.__wailsMock.setResponse('PodLogs', 'panic: runtime error: out of memory\n'));
    await hint.getByRole('button', { name: 'View logs before restart' }).click();
    await expect(page.locator('#logs-previous')).toBeChecked();
    await expect(page.locator('#logs-view')).toContainText('panic: runtime error');
    await expect(page.locator('#logs-follow')).toBeDisabled();
    await expect(hint).toContainText('Showing the instance before the last restart');
    const logCalls = await page.evaluate(() => window.__wailsMock.calls
        .filter((call) => call.method === 'PodLogs')
        .map((call) => call.args));
    expect(logCalls[0]).toEqual(['payments', 'checkout-7b8d9f-2kw7p', 'api', 500, false]);
    expect(logCalls.at(-1)).toEqual(['payments', 'checkout-7b8d9f-2kw7p', 'api', 500, true]);

    await page.locator('#btn-logs-copy-command').click();
    const copied = await page.evaluate(() => window.__wailsMock.calls.filter((call) => call.method === 'CopyToClipboard').map((call) => call.args[0]));
    expect(copied.at(-1)).toBe('kubectl logs checkout-7b8d9f-2kw7p -n payments -c api --tail=500 --previous');

    // A container that never restarted has no previous instance to offer.
    await page.locator('#logs-container').selectOption('sidecar');
    await expect(page.locator('#logs-previous')).not.toBeChecked();
    await expect(page.locator('#logs-previous')).toBeDisabled();
    await expect(page.locator('#logs-follow')).toBeEnabled();
    await expect(hint).toBeHidden();
    expect(pageErrors).toEqual([]);
});

test('FR-20: Drain names the budgets, unmanaged Pods and emptyDir data it affects before confirming', async ({ page }) => {
    const pageErrors = collectPageErrors(page);
    await page.setViewportSize({ width: 1440, height: 900 });
    await connectDashboard(page, {
        overrides: {
            DrainImpact: {
                node: 'kubby-worker', evict: 4, daemonSetPods: 1, mirrorPods: 0,
                unmanaged: ['payments/debug'], emptyDir: ['payments/cache'],
                blockingBudgets: [{ namespace: 'payments', name: 'checkout', allowedDisruptions: 0, podsOnNode: 2 }],
                warnings: [],
            },
        },
    });
    await openNavView(page, 'nodes');
    await page.getByRole('button', { name: 'Actions for Node kubby-worker' }).click();
    await page.getByRole('menuitem', { name: /Drain/ }).click();
    const message = page.locator('#dialog-message');
    await expect(message).toContainText('evicts 4 pods; 1 DaemonSet pod stays');
    await expect(message).toContainText('PodDisruptionBudget payments/checkout allows 0 disruptions but covers 2 pods here');
    await expect(message).toContainText('1 pod has no controller and will not be recreated: payments/debug');
    await expect(message).toContainText('1 pod uses emptyDir; that data is deleted: payments/cache');
    await page.locator('#dialog-cancel').click();
    const drains = await page.evaluate(() => window.__wailsMock.calls.filter((call) => call.method === 'DrainNodeOwned').length);
    expect(drains).toBe(0);
    expect(pageErrors).toEqual([]);
});

test('FR-19: Scale warns that a HorizontalPodAutoscaler will overwrite a manual scale', async ({ page }) => {
    const pageErrors = collectPageErrors(page);
    await page.setViewportSize({ width: 1440, height: 900 });
    await connectDashboard(page, {
        overrides: {
            ListDeployments: [{ namespace: 'payments', name: 'checkout', ready: '2/2', upToDate: 2, available: 2, age: '3d', isError: false }],
        },
    });
    await openNavView(page, 'deployments');
    await page.locator('#deployments-body tr', { hasText: 'checkout' }).click();
    await page.locator('#btn-scale').click();
    const note = page.locator('#scale-hpa-note');
    await expect(note).toContainText('HorizontalPodAutoscaler checkout manages this Deployment (2–6 replicas)');
    await note.getByRole('button', { name: 'Open autoscaler' }).click();
    await expect(page.locator('#modal')).toBeHidden();
    await expect(page.locator('#drawer-kind')).toHaveText('HorizontalPodAutoscaler');
    await expect(page.locator('#drawer-name')).toHaveText('checkout');
    expect(pageErrors).toEqual([]);
});

test('FR-5: HPA, PDB and NetworkPolicy sections show their own status', async ({ page }) => {
    const pageErrors = collectPageErrors(page);
    await page.setViewportSize({ width: 1440, height: 900 });
    await connectDashboard(page);

    await openNavView(page, 'hpas');
    await expect(page.locator('#hpas-body')).toContainText('Deployment/checkout');
    await expect(page.locator('#hpas-body')).toContainText('cpu 91%/70%');
    await openNavView(page, 'pdbs');
    await expect(page.locator('#pdbs-body tr.error-row')).toContainText('drains will block');
    await openNavView(page, 'networkpolicies');
    await expect(page.locator('#networkpolicies-body')).toContainText('Denies all ingress');
    await expect(page.locator('#btn-create')).toBeVisible();
    expect(pageErrors).toEqual([]);
});

test('FR-41: Topology marks policy-isolated Pods and checks traffic A → B', async ({ page }) => {
    const pageErrors = collectPageErrors(page);
    await page.setViewportSize({ width: 1440, height: 900 });
    await connectDashboard(page, {
        overrides: {
            NetworkFlows: {
                ingresses: [],
                services: [{
                    name: 'checkout', namespace: 'payments', type: 'ClusterIP', clusterIP: '10.96.0.20', ports: ['80→http'], routes: [], readyPods: 1, warning: '',
                    entryPolicy: { verdict: 'allowed', source: 'ingress-nginx controller (2 Pods in ingress-nginx)', blocked: 0, total: 1, policies: ['allow-ingress'] },
                    pods: [{
                        name: 'checkout-7b8d9f-v5lhn', namespace: 'payments', status: 'Running', ready: '1/1', restarts: 0, node: 'kubby-worker', ip: '10.244.1.9',
                        ownerKind: 'ReplicaSet', ownerName: 'checkout-7b8d9f', isError: false, isReady: true,
                    }],
                }],
                routedCount: 0, endpointCount: 1, brokenCount: 0, warnings: [],
                policiesAvailable: true, policyCount: 1, isolatedPods: 1,
                podPolicies: { 'payments/checkout-7b8d9f-v5lhn': { ingress: ['deny-all'], egress: [] } },
            },
        },
    });
    await openNavView(page, 'traffic');
    await expect(page.locator('#flow-summary')).toContainText('Network policies');
    const pod = page.locator('.flow-pod', { hasText: 'checkout-7b8d9f-v5lhn' });
    await expect(pod.locator('.flow-pod-policy')).toBeVisible();
    await expect(pod).toHaveAttribute('title', /Ingress isolated by: deny-all/);

    await expect(page.locator('.flow-policy-ok')).toContainText('NetworkPolicy allow-ingress admits the ingress-nginx controller');

    // Check access starts from the Service it sits on.
    await page.getByRole('button', { name: 'Check access' }).click();
    await expect(page.locator('#modal')).toBeVisible();
    await expect(page.locator('#traffic-dst')).toHaveValue('payments/checkout');
    await page.locator('#traffic-src').fill('default/web-5cf9d8b8d6-hv2lk');
    await page.locator('#traffic-port').fill('80');
    // Enter evaluates in place; it must not close the modal like an OK action.
    await page.locator('#traffic-port').press('Enter');
    const result = page.locator('#traffic-check-result');
    await expect(result).toContainText('Blocked');
    await expect(result).toContainText('isolated for ingress by deny-all');
    await expect(page.locator('#modal')).toBeVisible();
    const checkCalls = await page.evaluate(() => window.__wailsMock.calls
        .filter((call) => call.method === 'CheckTrafficPolicy')
        .map((call) => call.args));
    expect(checkCalls).toEqual([['default', 'web-5cf9d8b8d6-hv2lk', 'Service', 'payments', 'checkout', 80, '']]);

    await result.locator('.traffic-policy', { hasText: 'deny-all' }).click();
    await expect(page.locator('#modal')).toBeHidden();
    await expect(page.locator('#drawer-kind')).toHaveText('NetworkPolicy');
    await expect(page.locator('#drawer-name')).toHaveText('deny-all');
    await page.locator('#btn-copy-kubectl').click();
    const copied = await page.evaluate(() => window.__wailsMock.calls.filter((call) => call.method === 'CopyToClipboard').map((call) => call.args[0]));
    expect(copied.at(-1)).toBe('kubectl describe networkpolicy deny-all -n payments');
    expect(pageErrors).toEqual([]);
});

test('FR-42: Health checks explain broken webhooks, expiring certificates and stuck deletions', async ({ page }) => {
    const pageErrors = collectPageErrors(page);
    await page.setViewportSize({ width: 1440, height: 900 });
    await connectDashboard(page);
    await openNavView(page, 'checks');

    await expect(page.locator('#checks-summary')).toContainText('1 critical · 0 warning · 2 webhooks');
    await expect(page.locator('#checks-tab-webhooks')).toHaveText('Admission webhooks (1)');
    const webhooks = page.locator('#checks-webhooks-list');
    await expect(webhooks.locator('.checks-item').first()).toContainText('No ready endpoints behind the Service');

    // Only problems hides the healthy webhook and the informational certificate.
    await page.locator('#checks-only-problems').check();
    await expect(webhooks.locator('.checks-item', { hasText: 'inject.mesh.dev' })).toBeHidden();
    await page.getByRole('tab', { name: /Certificates/ }).click();
    await expect(page.locator('#checks-panel-certificates')).toBeVisible();
    await expect(page.locator('#checks-certificates-list')).toContainText('Expired 2d ago');
    await expect(page.locator('#checks-certificates-list .checks-item', { hasText: 'kubernetes-admin' })).toBeHidden();

    // Summary tiles switch tabs too.
    await page.locator('#checks-summary [data-check-tab="stuck"]').click();
    await expect(page.locator('#checks-tab-stuck')).toHaveAttribute('aria-selected', 'true');
    const stuck = page.locator('#checks-stuck-list');
    await expect(stuck).toContainText('example.com/cleanup');
    await expect(page.locator('#checks-stuck-scanned')).toHaveText('58 resource types scanned');
    await stuck.getByRole('button', { name: 'Copy' }).click();
    const copied = await page.evaluate(() => window.__wailsMock.calls.filter((call) => call.method === 'CopyToClipboard').map((call) => call.args[0]));
    expect(copied.at(-1)).toBe(`kubectl patch configmap legacy-config -n payments --type=merge -p '{"metadata":{"finalizers":null}}'`);
    const checkCalls = await page.evaluate(() => window.__wailsMock.calls.filter((call) => call.method === 'ClusterChecks').map((call) => call.args));
    expect(checkCalls[0]).toEqual(['']);

    await stuck.getByRole('button', { name: 'payments/legacy-config' }).click();
    await expect(page.locator('#drawer-kind')).toHaveText('ConfigMap');
    await expect(page.locator('#drawer-name')).toHaveText('legacy-config');
    expect(pageErrors).toEqual([]);
});

test('FR-43: Why Pending explains the scheduler verdict node by node', async ({ page }) => {
    const pageErrors = collectPageErrors(page);
    await page.setViewportSize({ width: 1440, height: 900 });
    await connectDashboard(page, {
        overrides: {
            OverviewSnapshot: {
                stats: { nodes: 2, namespaces: 2, pods: 6, deployments: 2, podsAvailable: true },
                failingPods: [{ namespace: 'payments', name: 'reporting-0', status: 'Pending', restarts: 0 }],
                nodeMetrics: [], nodeStatus: [], topPods: [], events: [], warnings: [],
            },
        },
    });

    // A Pod with no node yet has no logs and no container state, so the queue
    // offers the scheduling answer instead of Investigate.
    const queue = page.locator('#overview-errors-body');
    await expect(queue.getByRole('button', { name: 'Why Pending?' })).toBeVisible();
    await expect(queue.getByRole('button', { name: 'Investigate' })).toHaveCount(0);
    await queue.getByRole('button', { name: 'Why Pending?' }).click();

    await expect(page.locator('#modal-title')).toHaveText('Why is this Pod Pending?');
    const result = page.locator('#sched-result');
    await expect(result).toContainText('No node fits');
    await expect(result).toContainText('0/3 nodes are available');
    // The scheduler's own message leads; Kubby's explanation follows.
    await expect(result.locator('.checks-finding').first()).toContainText('The scheduler reports: Unschedulable');
    // Reasons are grouped with counts rather than repeated per node.
    const reasons = result.locator('.sched-reason');
    await expect(reasons).toHaveCount(2);
    await expect(reasons.first()).toContainText('Insufficient memory');
    await expect(reasons.first().locator('.sched-reason-count')).toHaveText('2');
    await expect(reasons.first()).toContainText('needs 8.0Gi, 2.0Gi free of 8.0Gi');
    // Each node keeps its own numbers, which is what kubectl never shows.
    const nodes = result.locator('.sched-nodes tbody tr');
    await expect(nodes).toHaveCount(2);
    await expect(nodes.first()).toContainText('kubby-worker');
    await expect(nodes.first()).toContainText('2.0Gi');
    await expect(result).toContainText('FailedScheduling');
    await expect(result.locator('.sched-limits')).toContainText('never schedules or evicts anything');

    const calls = await page.evaluate(() => window.__wailsMock.calls.filter((call) => call.method === 'ExplainPodScheduling').map((call) => call.args));
    expect(calls[0]).toEqual(['payments', 'reporting-0']);
    expect(pageErrors).toEqual([]);
});

test('FR-44: Cleanup groups unused objects and offers the command without running it', async ({ page }) => {
    const pageErrors = collectPageErrors(page);
    await page.setViewportSize({ width: 1440, height: 900 });
    await connectDashboard(page);
    await openNavView(page, 'hygiene');

    await expect(page.locator('#hygiene-summary')).toContainText('Unreferenced ConfigMaps');
    await expect(page.locator('#hygiene-updated')).toContainText('3 items');
    const groups = page.locator('.hygiene-group');
    await expect(groups).toHaveCount(3);
    // Every group carries what a reference scan cannot see, beside its items.
    await expect(groups.first().locator('.hygiene-caveat')).toContainText('reads a ConfigMap by name');

    const claims = page.locator('#hygiene-group-unused-pvc');
    await expect(claims).toContainText('reporting-scratch');
    await expect(claims).toContainText('reclaim policy');

    // "Only warnings" hides the informational ConfigMap but keeps the claim.
    await page.locator('#hygiene-only-problems').check();
    await expect(page.locator('#hygiene-group-unused-configmap .checks-item')).toBeHidden();
    await expect(claims.locator('.checks-item')).toBeVisible();
    await page.locator('#hygiene-only-problems').uncheck();

    await page.locator('#hygiene-filter').fill('legacy');
    await expect(claims.locator('.checks-item')).toBeHidden();
    await expect(page.locator('#hygiene-group-unused-configmap .checks-item')).toBeVisible();
    await page.locator('#hygiene-filter').fill('');

    // The command is copied, never executed.
    await claims.getByRole('button', { name: 'Copy' }).click();
    const copied = await page.evaluate(() => window.__wailsMock.calls.filter((call) => call.method === 'CopyToClipboard').map((call) => call.args[0]));
    expect(copied.at(-1)).toBe('kubectl delete pvc reporting-scratch -n payments');
    const deleted = await page.evaluate(() => window.__wailsMock.calls.filter((call) => call.method.startsWith('Delete')));
    expect(deleted).toEqual([]);

    // An item opens the object it names.
    await claims.getByRole('button', { name: 'payments/reporting-scratch' }).click();
    await expect(page.locator('#drawer-kind')).toHaveText('PersistentVolumeClaim');
    expect(pageErrors).toEqual([]);
});

test('NFR-7: switching view abandons the reads the previous screen still had in flight', async ({ page }) => {
    const pageErrors = collectPageErrors(page);
    await page.setViewportSize({ width: 1440, height: 900 });
    await connectDashboard(page, { overrides: { ClusterChecks: { __deferred: 'checks' } } });

    // Health checks lists every resource type, so it is exactly the read worth
    // abandoning. It never resolves here: the user moves on first.
    await openNavView(page, 'checks');
    await expect(page.locator('#view-status')).toContainText('Loading…');
    await openNavView(page, 'nodes');

    await expect.poll(() => page.evaluate(() => window.__wailsMock.calls.filter((call) => call.method === 'CancelViewReads').length))
        .toBeGreaterThan(0);
    // The screen the user did ask for is rendered, and the abandoned request
    // never reports an error into it.
    await expect(page.locator('#nodes-body')).toContainText('kubby-worker');
    await expect(page.locator('#view-status')).toContainText('Updated just now');
    await expect(page.locator('#dash-error')).toBeHidden();

    // The first load of a screen has nothing to abandon, so it costs no call.
    const beforeFirstLoad = await page.evaluate(() => window.__wailsMock.calls.filter((call) => call.method === 'CancelViewReads').length);
    await page.evaluate(() => window.__wailsMock.resolveDeferred('checks', {
        scope: '', checkedAt: '2026-09-17T05:00:00Z',
        webhooks: { critical: 0, warning: 0, warnings: [], webhooks: [] },
        certificates: { critical: 0, warning: 0, certManagerInstalled: false, warnings: [], certificates: [] },
        stuck: { critical: 0, warning: 0, scanned: 12, failed: 0, warnings: [], objects: [] },
    }));
    expect(beforeFirstLoad).toBeGreaterThan(0);
    expect(pageErrors).toEqual([]);
});

test('FR-6: a namespace created outside Kubby after connecting appears in the picker', async ({ page }) => {
    const pageErrors = collectPageErrors(page);
    await page.setViewportSize({ width: 1440, height: 900 });
    await connectDashboard(page);

    // Created with kubectl while Kubby is connected — Kubby never saw it.
    await page.evaluate(() => window.__wailsMock.setResponse('ListNamespaces', [
        { name: 'default', status: 'Active' },
        { name: 'kubby-demo-sched', status: 'Active' },
        { name: 'payments', status: 'Active' },
    ]));

    // Opening the picker reloads the list.
    await page.locator('#namespace-toggle').click();
    const option = page.locator('#namespace-options .namespace-option', { hasText: 'kubby-demo-sched' });
    await expect(option).toBeVisible();
    await option.click();
    await expect(page.locator('#namespace-current')).toHaveText('kubby-demo-sched');

    // Refresh reloads it too, and a deleted namespace disappears.
    await page.evaluate(() => window.__wailsMock.setResponse('ListNamespaces', [
        { name: 'default', status: 'Active' },
        { name: 'kubby-demo-sched', status: 'Active' },
        { name: 'kubby-demo-hygiene', status: 'Active' },
    ]));
    await page.locator('#btn-refresh').click();
    await page.locator('#namespace-toggle').click();
    await expect(page.locator('#namespace-options')).toContainText('kubby-demo-hygiene');
    await expect(page.locator('#namespace-options')).not.toContainText('payments');
    expect(pageErrors).toEqual([]);
});
