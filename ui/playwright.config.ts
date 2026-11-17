import { defineConfig } from '@playwright/test';

// The build host is ARM64 with a system Chromium; set SG_CHROMIUM to use it.
const executablePath = process.env.SG_CHROMIUM || undefined;

export default defineConfig({
  testDir: 'e2e',
  outputDir: 'test-results',
  fullyParallel: true,
  reporter: [['list']],
  use: {
    baseURL: 'http://127.0.0.1:4173',
    launchOptions: { executablePath, args: ['--no-sandbox'] },
  },
  webServer: { command: 'bun run preview', url: 'http://127.0.0.1:4173', reuseExistingServer: true },
});
