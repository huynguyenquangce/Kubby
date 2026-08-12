import { expect, test } from '@playwright/test';
import { connectDashboard } from './wails-mock.js';

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
