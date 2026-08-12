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
    const overflow = await page.evaluate(() => {
        const result = [];
        const root = document.documentElement;
        if (root.scrollWidth > window.innerWidth + 1) {
            result.push(`document ${root.scrollWidth} > viewport ${window.innerWidth}`);
        }
        for (const selector of ['#dashboard', '.main', '.topbar', '.page-heading', '#content']) {
            const element = document.querySelector(selector);
            if (!element || element.hidden) continue;
            const rect = element.getBoundingClientRect();
            if (rect.left < -1 || rect.right > window.innerWidth + 1) {
                result.push(`${selector} [${rect.left}, ${rect.right}] outside ${window.innerWidth}`);
            }
        }
        return result;
    });
    expect(overflow).toEqual([]);
}

test('FR-2/FR-13: pasted kubeconfig enters a populated Overview', async ({ page }) => {
    const pageErrors = collectPageErrors(page);
    await connectDashboard(page);

    await expect(page.locator('#page-title')).toHaveText('Overview');
    await expect(page.locator('#cluster-health-summary')).toContainText('5 of 6 pods are healthy');
    await expect(page.locator('#stat-nodes')).toHaveText('2');
    await expect(page.locator('#overview-errors-body')).toContainText('checkout-7b8d9f-2kw7p');
    await expect(page.locator('#overview-toppods-body')).toContainText('api-6df7fdd9f8-4zj8g');
    await expect(page.locator('#overview-events-body')).toContainText('BackOff');
    expect(pageErrors).toEqual([]);

    const calls = await page.evaluate(() => window.__wailsMock.calls.map((call) => call.method));
    expect(calls).toEqual(expect.arrayContaining(['ContextsFromContent', 'ConnectWithContent', 'OverviewSnapshot']));
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

test('FR-37: cluster structure supports filtering and resource inspection', async ({ page }) => {
    const pageErrors = collectPageErrors(page);
    await page.setViewportSize({ width: 1100, height: 760 });
    await connectDashboard(page);
    await page.locator('#btn-cluster-structure').click();

    await expect(page.locator('#page-title')).toHaveText('Cluster structure');
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
    await page.getByRole('button', { name: 'YAML', exact: true }).click();
    await expect(page.locator('#dpanel-yaml .cm-editor')).toContainText('api-6df7fdd9f8-4zj8g');

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

test('FR-31/FR-38: Settings modal closes with Escape and remains viewport-bounded', async ({ page }) => {
    const pageErrors = collectPageErrors(page);
    await page.setViewportSize({ width: 390, height: 700 });
    await connectDashboard(page);
    await page.locator('#btn-settings').click();

    await expect(page.locator('#modal')).toBeVisible();
    await expect(page.locator('#modal')).toHaveAttribute('aria-labelledby', 'modal-title');
    await expect(page.locator('#modal-title')).toHaveText('Settings');
    await expect(page.locator('#about-version')).toContainText('0.1.0-test');
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

test('FR-5: every built-in navigation target renders without a browser exception', async ({ page }) => {
    const pageErrors = collectPageErrors(page);
    await page.setViewportSize({ width: 1440, height: 900 });
    await connectDashboard(page);
    const targets = await page.locator('.nav-item[data-view]').evaluateAll((items) => items.map((item) => item.dataset.view));

    for (const target of targets) {
        await page.locator(`.nav-item[data-view="${target}"]`).click();
        await expect(page.locator(`#view-${target}`)).toBeVisible();
        await expect(page.locator('#dash-error')).toBeHidden();
    }
    expect(pageErrors).toEqual([]);
});
