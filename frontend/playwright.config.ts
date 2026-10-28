import os from 'node:os';
import path from 'node:path';
import { defineConfig, devices } from '@playwright/test';

const outputRoot = process.env.MVFORGE_DUPLA_ROOT ?? process.env.TMPDIR ?? os.tmpdir();
const runOutput = path.join(outputRoot, 'playwright');
const fixtureAPI = 'http://127.0.0.1:8765';

export default defineConfig({
  testDir: './e2e',
  testMatch: '**/*.pw.ts',
  fullyParallel: false,
  workers: 1,
  retries: 0,
  reporter: [
    ['list'],
    ['html', { outputFolder: path.join(runOutput, 'report'), open: 'never' }],
  ],
  outputDir: path.join(runOutput, 'results'),
  use: {
    baseURL: 'http://127.0.0.1:5173',
    screenshot: 'only-on-failure',
    trace: 'off',
    ...devices['Desktop Chrome'],
  },
  webServer: [
    {
      command: 'python3 ../scripts/e2e/ui_fixture_backend.py --host 127.0.0.1 --port 8765',
      url: `${fixtureAPI}/__health`,
      reuseExistingServer: false,
      timeout: 30_000,
    },
    {
      command: `VITE_API_BASE_URL=${fixtureAPI} npm run dev -- --host 127.0.0.1 --port 5173`,
      url: 'http://127.0.0.1:5173',
      reuseExistingServer: false,
      timeout: 60_000,
    },
  ],
});
