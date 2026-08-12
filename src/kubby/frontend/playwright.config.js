import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
    testDir: './e2e',
    outputDir: './test-results',
    fullyParallel: true,
    forbidOnly: Boolean(process.env.CI),
    retries: process.env.CI ? 2 : 0,
    workers: process.env.CI ? 2 : undefined,
    reporter: process.env.CI
        ? [['line'], ['html', { outputFolder: 'playwright-report', open: 'never' }]]
        : 'line',
    expect: {
        timeout: 7_000,
        toHaveScreenshot: {
            animations: 'disabled',
            maxDiffPixelRatio: 0.01,
        },
    },
    use: {
        baseURL: 'http://127.0.0.1:4175',
        trace: 'retain-on-failure',
        screenshot: 'only-on-failure',
        video: 'retain-on-failure',
    },
    webServer: {
        command: 'npm run dev -- --host 127.0.0.1 --port 4175',
        url: 'http://127.0.0.1:4175',
        reuseExistingServer: !process.env.CI,
        timeout: 120_000,
    },
    projects: [
        {
            name: 'chromium',
            use: { ...devices['Desktop Chrome'] },
        },
    ],
});
