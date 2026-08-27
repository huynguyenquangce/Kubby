import { expect, test } from '@playwright/test';
import { collectPageErrors, connectDashboard } from './wails-mock.js';

for (const theme of ['light', 'dark']) {
    test(`narrow ${theme} shell keeps accessible names, containment, and modal focus ownership`, async ({ page }) => {
        await page.setViewportSize({ width: 390, height: 844 });
        const pageErrors = collectPageErrors(page);
        await connectDashboard(page, { theme });
        await page.locator('#btn-mobile-nav').click();
        await page.locator('.nav-item[data-view="pods"]').click();
        await expect(page.locator('#pods-body tr')).toHaveCount(2);

        const audit = await page.evaluate(() => {
            const visible = (element) => {
                const style = getComputedStyle(element);
                return style.visibility !== 'hidden' && style.display !== 'none' && element.getClientRects().length > 0;
            };
            const nameOf = (element) => {
                const labelledBy = element.getAttribute('aria-labelledby');
                if (labelledBy) return labelledBy.split(/\s+/).map((id) => document.getElementById(id)?.textContent || '').join(' ').trim();
                const label = element.labels?.[0]?.textContent || element.closest('label')?.textContent || '';
                return (element.getAttribute('aria-label') || label || element.textContent || element.getAttribute('title') || '').trim();
            };
            const controls = [...document.querySelectorAll('button, input, select, textarea, [role="button"], [role="tab"]')]
                .filter(visible)
                .filter((element) => !nameOf(element))
                .map((element) => `${element.tagName.toLowerCase()}#${element.id || '(no-id)'}`);
            const ids = [...document.querySelectorAll('[id]')].map((element) => element.id);
            const duplicates = ids.filter((id, index) => ids.indexOf(id) !== index);
            return {
                controls,
                duplicates: [...new Set(duplicates)],
                overflow: document.documentElement.scrollWidth - window.innerWidth,
                rowCount: document.querySelector('#view-pods table')?.getAttribute('aria-rowcount'),
                theme: document.documentElement.dataset.theme,
            };
        });
        expect(audit.controls).toEqual([]);
        expect(audit.duplicates).toEqual([]);
        expect(audit.overflow).toBeLessThanOrEqual(1);
        expect(audit.rowCount).toBe('3');
        expect(audit.theme).toBe(theme);

        await page.locator('#btn-import').click();
        await expect(page.locator('#modal')).toHaveAttribute('aria-modal', 'true');
        await expect(page.locator('.main')).toHaveAttribute('inert', '');
        for (let i = 0; i < 12; i++) {
            await page.keyboard.press('Tab');
            expect(await page.evaluate(() => document.activeElement?.closest('#modal')?.id)).toBe('modal');
        }
        await page.keyboard.press('Escape');
        await expect(page.locator('#modal')).toBeHidden();
        await expect(page.locator('.main')).not.toHaveAttribute('inert', '');
        expect(pageErrors).toEqual([]);
    });
}
