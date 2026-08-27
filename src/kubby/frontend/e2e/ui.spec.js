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
