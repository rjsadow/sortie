import { defineConfig, devices } from '@playwright/test';

const PORT = 3847;
const BASE_URL = `http://localhost:${PORT}`;

export default defineConfig({
  testDir: './e2e',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 1 : undefined,
  reporter: process.env.CI ? [['github'], ['html', { open: 'never' }]] : [['html', { open: 'never' }]],

  use: {
    baseURL: BASE_URL,
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
    video: 'on-first-retry',
  },

  projects: [
    {
      name: 'setup',
      testMatch: /auth\.setup\.ts/,
    },
    {
      name: 'chromium',
      use: {
        ...devices['Desktop Chrome'],
        storageState: 'e2e/.auth/admin.json',
      },
      dependencies: ['setup'],
    },
    {
      name: 'firefox',
      use: {
        ...devices['Desktop Firefox'],
        storageState: 'e2e/.auth/admin.json',
      },
      dependencies: ['setup'],
    },
    {
      name: 'webkit',
      use: {
        ...devices['Desktop Safari'],
        storageState: 'e2e/.auth/admin.json',
      },
      dependencies: ['setup'],
    },
  ],

  webServer: {
    command: 'rm -f /tmp/sortie-e2e-test.db* && ../sortie --mock-runner --seed ../testdata/e2e-apps.json',
    url: `${BASE_URL}/readyz`,
    reuseExistingServer: !process.env.CI,
    timeout: 30_000,
    env: {
      SORTIE_PORT: String(PORT),
      SORTIE_DB: '/tmp/sortie-e2e-test.db',
      SORTIE_JWT_SECRET: 'test-secret-for-e2e-playwright-runs!',
      SORTIE_ADMIN_PASSWORD: 'admin123',
      SORTIE_ALLOW_REGISTRATION: 'true',
      SORTIE_GATEWAY_RATE_LIMIT: '0',
      SORTIE_VIDEO_RECORDING_ENABLED: 'true',
      SORTIE_RECORDING_STORAGE_PATH: '/tmp/sortie-e2e-recordings',
    },
  },
});
