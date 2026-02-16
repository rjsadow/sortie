import { test, expect } from '@playwright/test';
import {
  getAdminToken,
  createTestSession,
  terminateTestSession,
  waitForSessionRunning,
  listSessions,
} from './helpers/api';

test.describe('SSE Real-Time Updates', () => {
  test.describe.configure({ mode: 'serial' });

  let token: string;

  test.beforeAll(async ({ request }) => {
    token = await getAdminToken(request);
  });

  test.afterEach(async ({ request }) => {
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

  test('badge updates in real-time when session is created via API (no page reload)', async ({
    page,
    request,
  }) => {
    await page.goto('/');
    await expect(page.getByLabel('Manage sessions')).toBeVisible();

    // Record the initial badge state — should have no green badge or count of 0
    const sessionsButton = page.getByLabel('Manage sessions');
    const badge = sessionsButton.locator('.bg-green-500');

    // Get initial count (may be 0 or badge may not exist yet)
    const initialBadgeCount = await badge.count();
    let initialCount = 0;
    if (initialBadgeCount > 0) {
      const text = await badge.textContent();
      initialCount = parseInt(text ?? '0', 10);
    }

    // Create a session via API (NOT through the UI) — the badge should update
    // automatically via SSE without any page reload or user interaction
    const createRes = await createTestSession(request, token, 'test-container');
    expect(createRes.ok()).toBeTruthy();
    const session = await createRes.json();
    await waitForSessionRunning(request, token, session.id);

    // Wait for the badge to appear/update — SSE should push the event within ~1s
    await expect(badge).toBeVisible({ timeout: 10_000 });
    await expect(badge).toHaveText(/^\d+$/, { timeout: 10_000 });

    // Verify the count increased
    const newText = await badge.textContent();
    const newCount = parseInt(newText ?? '0', 10);
    expect(newCount).toBeGreaterThan(initialCount);
  });

  test('badge updates in real-time when session is terminated via API (no page reload)', async ({
    page,
    request,
  }) => {
    // First create a session so we have something to terminate
    const createRes = await createTestSession(request, token, 'test-container');
    expect(createRes.ok()).toBeTruthy();
    const session = await createRes.json();
    await waitForSessionRunning(request, token, session.id);

    // Load the page — badge should show the active session
    await page.goto('/');
    await expect(page.getByLabel('Manage sessions')).toBeVisible();

    const sessionsButton = page.getByLabel('Manage sessions');
    const badge = sessionsButton.locator('.bg-green-500');
    await expect(badge).toBeVisible({ timeout: 10_000 });

    const beforeText = await badge.textContent();
    const beforeCount = parseInt(beforeText ?? '0', 10);
    expect(beforeCount).toBeGreaterThanOrEqual(1);

    // Terminate via API — badge should update without page reload
    await terminateTestSession(request, token, session.id);

    // The badge should disappear or decrement. If this was the only session,
    // the badge should vanish entirely.
    if (beforeCount === 1) {
      await expect(badge).toBeHidden({ timeout: 10_000 });
    } else {
      // Count should decrease
      await expect(badge).toHaveText(String(beforeCount - 1), { timeout: 10_000 });
    }
  });
});
