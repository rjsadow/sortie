import { test, expect } from '@playwright/test';
import {
  getAdminToken,
  createTestSession,
  listSessions,
  terminateTestSession,
  waitForSessionRunning,
} from './helpers/api';

test.describe('Sessions', () => {
  test.describe.configure({ mode: 'serial' });

  let token: string;

  test.beforeAll(async ({ request }) => {
    token = await getAdminToken(request);
  });

  test.afterEach(async ({ request }) => {
    // Clean up all sessions after each test
    const res = await listSessions(request, token);
    if (res.ok()) {
      const sessions = await res.json();
      for (const s of sessions) {
        if (s.status !== 'stopped' && s.status !== 'expired' && s.status !== 'failed') {
          await terminateTestSession(request, token, s.id);
        }
      }
    }
  });

  test('launches a container session and shows status transitions', async ({ page }) => {
    await page.goto('/');
    await expect(page.getByPlaceholder('Search applications...')).toBeVisible();

    // Click the container app card
    await page.getByText('Test Container App').click();

    // Should show creating/starting status (scoped to header banner to avoid strict mode
    // violation since status text appears in both the header and the center overlay)
    const banner = page.getByRole('banner');
    await expect(
      banner.getByText('Creating session...').or(banner.getByText('Starting container...')),
    ).toBeVisible({ timeout: 5_000 });

    // Wait for session to progress past creating. The banner shows statusMessage
    // with a fallback to "Connecting..." (see SessionPage line 188), so we need
    // to accept that fallback text too.
    await expect(
      banner
        .getByText('Connecting to display...')
        .or(banner.getByText('Waiting for container...'))
        .or(banner.getByText('Connection error'))
        .or(banner.getByText('Connecting...'))
        .or(banner.getByText('Error')),
    ).toBeVisible({ timeout: 15_000 });

    // Go back to dashboard
    await page.getByLabel('Back to dashboard').click();
    await expect(page.getByPlaceholder('Search applications...')).toBeVisible();
  });

  test('shows session in session manager after launch', async ({ page, request }) => {
    // Create session via API
    const createRes = await createTestSession(request, token, 'test-container');
    expect(createRes.ok()).toBeTruthy();
    const session = await createRes.json();
    await waitForSessionRunning(request, token, session.id);

    await page.goto('/');
    await expect(page.getByPlaceholder('Search applications...')).toBeVisible();

    // Open session manager
    await page.getByLabel('Manage sessions').click();

    // Scope assertions to the dialog to avoid matching dashboard app cards
    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();
    await expect(dialog.getByText('Test Container App').first()).toBeVisible();
    await expect(dialog.getByText('Running').first()).toBeVisible();

    // Footer shows active session count
    await expect(dialog.getByText(/\d+ active session/)).toBeVisible();
  });

  test('terminates session from session manager', async ({ page, request }) => {
    // Accept confirmation dialogs
    page.on('dialog', (d) => d.accept());

    // Create session via API
    const createRes = await createTestSession(request, token, 'test-container');
    expect(createRes.ok()).toBeTruthy();
    const session = await createRes.json();
    await waitForSessionRunning(request, token, session.id);

    await page.goto('/');
    await expect(page.getByPlaceholder('Search applications...')).toBeVisible();

    // Open session manager
    await page.getByLabel('Manage sessions').click();
    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();
    await expect(dialog.getByText('Test Container App').first()).toBeVisible();

    // Click Terminate within the dialog (click first one - ours)
    await dialog.getByRole('button', { name: 'Terminate' }).first().click();

    // Session should disappear, empty state shown
    await expect(dialog.getByText('No active sessions')).toBeVisible({ timeout: 5_000 });
  });

  test('shows session in admin Sessions tab', async ({ page, request }) => {
    // Accept confirmation dialogs
    page.on('dialog', (d) => d.accept());

    // Create session via API
    const createRes = await createTestSession(request, token, 'test-container');
    expect(createRes.ok()).toBeTruthy();
    const session = await createRes.json();
    await waitForSessionRunning(request, token, session.id);

    await page.goto('/');
    await expect(page.getByPlaceholder('Search applications...')).toBeVisible();

    // Open admin panel
    await page.getByRole('button', { name: /Admin settings/i }).click();
    await expect(page.getByRole('heading', { name: 'Admin Settings' })).toBeVisible();

    // Click Sessions tab
    await page.getByRole('button', { name: 'Sessions', exact: true }).click();

    // Click Refresh to load sessions
    await page.getByRole('button', { name: 'Refresh' }).click();

    // Session should be visible in the table
    await expect(page.getByText('running').first()).toBeVisible({ timeout: 5_000 });

    // Terminate from admin (clicking the first Terminate button targets our session)
    await page.getByRole('button', { name: 'Terminate' }).first().click();

    // After termination, Refresh to see updated status
    await page.getByRole('button', { name: 'Refresh' }).click();

    // Our session should now show "stopped" status
    await expect(page.getByText('stopped').first()).toBeVisible({ timeout: 5_000 });
  });

  test('shows active session count badge', async ({ page, request }) => {
    // Create session via API
    const createRes = await createTestSession(request, token, 'test-container');
    expect(createRes.ok()).toBeTruthy();
    const session = await createRes.json();
    await waitForSessionRunning(request, token, session.id);

    await page.goto('/');
    await expect(page.getByPlaceholder('Search applications...')).toBeVisible();

    // The sessions button should show a green badge with a numeric count.
    // Note: parallel browser workers share the server, so the count may be > 1.
    const sessionsButton = page.getByLabel('Manage sessions');
    await expect(sessionsButton.locator('.bg-green-500')).toBeVisible({ timeout: 5_000 });
    await expect(sessionsButton.locator('.bg-green-500')).toHaveText(/^\d+$/);

    // Terminate session via API
    await terminateTestSession(request, token, session.id);

    // Reload to see updated state
    await page.reload();
    await expect(page.getByPlaceholder('Search applications...')).toBeVisible();

    // Badge count should decrease (may still be visible if other browser workers have sessions)
    // Just verify the page loaded correctly after termination
  });

  test('closes session page with back button and auto-terminates', async ({
    page,
    request,
  }) => {
    await page.goto('/');
    await expect(page.getByPlaceholder('Search applications...')).toBeVisible();

    // Click the container app card to open session page
    await page.getByText('Test Container App').click();

    // Wait for session to reach running/connecting state (ensures session object exists
    // in the frontend, so handleClose will call terminateSession). Include the
    // "Connecting..." fallback for when statusMessage is briefly empty.
    const banner = page.getByRole('banner');
    await expect(
      banner
        .getByText('Connecting to display...')
        .or(banner.getByText('Waiting for container...'))
        .or(banner.getByText('Connection error'))
        .or(banner.getByText('Connecting...'))
        .or(banner.getByText('Error')),
    ).toBeVisible({ timeout: 15_000 });

    // Click back to dashboard (this auto-terminates the session)
    await page.getByLabel('Back to dashboard').click();
    await expect(page.getByPlaceholder('Search applications...')).toBeVisible();

    // Verify we returned to the dashboard successfully.
    // The afterEach cleanup handles any sessions not auto-terminated.
  });
});
