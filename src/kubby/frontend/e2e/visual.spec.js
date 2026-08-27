import { expect, test } from '@playwright/test';
import { connectDashboard } from './wails-mock.js';

test('Compact command bar visual baseline', async ({ page }) => {
    await page.setViewportSize({ width: 500, height: 700 });
    await connectDashboard(page);
    await page.locator('#page-title').hover();
    await expect(page.locator('.topbar')).toHaveScreenshot('command-bar-compact.png');
});

test('Overview light theme visual baseline', async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 1000 });
    await connectDashboard(page, { theme: 'light' });
    await expect(page.locator('#dashboard')).toHaveScreenshot('overview-light.png', {
        mask: [page.locator('#pf-manager')],
    });
});

test('Overview dark theme visual baseline', async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 1000 });
    await connectDashboard(page, { theme: 'dark' });
    await expect(page.locator('#dashboard')).toHaveScreenshot('overview-dark.png', {
        mask: [page.locator('#pf-manager')],
    });
});

test('Cluster structure visual baseline', async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 1000 });
    await connectDashboard(page, { theme: 'light' });
    await page.locator('#btn-cluster-structure').click();
    await expect(page.locator('#structure-entries .structure-path')).toHaveCount(1);
    await page.locator('#structure-updated').evaluate((element) => { element.textContent = 'Updated just now'; });
    await expect(page.locator('#dashboard')).toHaveScreenshot('cluster-structure.png', {
        mask: [page.locator('#pf-manager')],
    });
});

test('Pod Terminal visual baseline', async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 1000 });
    await connectDashboard(page, { theme: 'dark' });
    await page.locator('.nav-item[data-view="pods"]').click();
    await page.locator('#pods-body tr', { hasText: 'api-6df7fdd9f8-4zj8g' }).click();
    await page.getByRole('tab', { name: 'Terminal', exact: true }).click();
    await expect(page.locator('#term-status')).toHaveText('Attached · /bin/bash');
    const sessionId = await page.evaluate(() => window.__wailsMock.calls
        .filter((call) => call.method === 'StartExec').at(-1)?.args[0]);
    await page.evaluate((id) => window.__wailsMock.emit('exec-output', {
        sessionId: id,
        data: '\u001b[36mkubby\u001b[0m@api:/workspace$ printf "ready\\n"\r\nready\r\nkubby@api:/workspace$ ',
    }), sessionId);
    await page.locator('.term-toolbar-copy').click();
    await expect(page.locator('#drawer')).toHaveScreenshot('pod-terminal.png');
});

test('Incident Studio visual baseline', async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 1000 });
    await connectDashboard(page, { theme: 'light' });
    await page.locator('#overview-errors-body .issue-diagnose').click();
    await expect(page.locator('#incident-summary')).toHaveText('checkout was OOMKilled');
    await expect(page.locator('#drawer')).toHaveScreenshot('incident-studio.png');
});
