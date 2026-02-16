import { test, expect } from '@playwright/test';
import {
  getAdminToken,
  createTestUser,
  deleteTestUser,
  createTestSession,
  terminateTestSession,
  waitForSessionRunning,
  loginAs,
  listSessionShares,
} from './helpers/api';

// Sharing tests use fresh browser state — each test logs in as the required user.
test.use({ storageState: { cookies: [], origins: [] } });

test.describe('Session Sharing', () => {
  test.describe.configure({ mode: 'serial' });

  let adminToken: string;
  let ownerUserId: string;
  let viewerUserId: string;
  let ownerToken: string;
  let sessionId: string;

  test.beforeAll(async ({ request }) => {
    adminToken = await getAdminToken(request);

    // Create owner and viewer users
    const ownerRes = await createTestUser(request, adminToken, {
      username: 'pw-share-owner',
      password: 'pass1234',
    });
    ownerUserId = (await ownerRes.json()).id;

    const viewerRes = await createTestUser(request, adminToken, {
      username: 'pw-share-viewer',
      password: 'pass1234',
    });
    viewerUserId = (await viewerRes.json()).id;

    ownerToken = await loginAs(request, 'pw-share-owner', 'pass1234');

    // Create a running session owned by the owner
    const sessRes = await createTestSession(request, ownerToken, 'test-container', ownerUserId);
    sessionId = (await sessRes.json()).id;
    await waitForSessionRunning(request, ownerToken, sessionId);
  });

  test.afterAll(async ({ request }) => {
    // Clean up
    await terminateTestSession(request, adminToken, sessionId).catch(() => {});
    await deleteTestUser(request, adminToken, ownerUserId).catch(() => {});
    await deleteTestUser(request, adminToken, viewerUserId).catch(() => {});
  });

  test('owner sees Share button in session manager', async ({ page }) => {
    // Login as owner
    await page.goto('/');
    await page.getByLabel('Username').fill('pw-share-owner');
    await page.getByLabel('Password').fill('pass1234');
    await page.getByRole('button', { name: 'Sign In' }).click();
    await expect(page.getByLabel('Manage sessions')).toBeVisible();

    // Open session manager
    await page.getByLabel('Manage sessions').click();
    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();

    // Should see the session with a Share button
    await expect(dialog.getByText('Test Container App').first()).toBeVisible();
    await expect(dialog.getByRole('button', { name: 'Share' }).first()).toBeVisible();
  });

  test('owner can open share dialog and invite a user', async ({ page }) => {
    // Login as owner
    await page.goto('/');
    await page.getByLabel('Username').fill('pw-share-owner');
    await page.getByLabel('Password').fill('pass1234');
    await page.getByRole('button', { name: 'Sign In' }).click();
    await expect(page.getByLabel('Manage sessions')).toBeVisible();

    // Open session manager and click Share
    await page.getByLabel('Manage sessions').click();
    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();
    await dialog.getByRole('button', { name: 'Share' }).first().click();

    // Share dialog should appear
    await expect(page.getByText('Share Session')).toBeVisible();

    // Permission selector should be visible
    await expect(page.getByLabel('Permission')).toBeVisible();

    // Invite by username
    await page.getByPlaceholder('Enter username').fill('pw-share-viewer');
    await page.getByRole('button', { name: 'Invite' }).click();

    // Should show the share in "Current Shares" section
    await expect(page.getByText('Current Shares')).toBeVisible({ timeout: 5_000 });
    await expect(page.getByText('pw-share-viewer')).toBeVisible();
    // Use locator scoped to the share entry to avoid matching the <option> in the Permission <select>
    const shareEntry = page.getByText('pw-share-viewer').locator('..');
    await expect(shareEntry.getByText('View Only')).toBeVisible();
  });

  test('viewer sees shared session with correct indicators', async ({ page }) => {
    // Login as viewer
    await page.goto('/');
    await page.getByLabel('Username').fill('pw-share-viewer');
    await page.getByLabel('Password').fill('pass1234');
    await page.getByRole('button', { name: 'Sign In' }).click();
    await expect(page.getByLabel('Manage sessions')).toBeVisible();

    // Open session manager
    await page.getByLabel('Manage sessions').click();
    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();

    // Should see shared session with indicators
    await expect(dialog.getByText('Test Container App').first()).toBeVisible();
    await expect(dialog.getByText('Shared by pw-share-owner')).toBeVisible();
    await expect(dialog.getByText('View Only').first()).toBeVisible();

    // Should NOT see Terminate or Share buttons for shared sessions
    await expect(dialog.getByRole('button', { name: 'Terminate' })).not.toBeVisible();
    await expect(dialog.getByRole('button', { name: 'Share' })).not.toBeVisible();

    // Should see Connect button (not Reconnect) for shared sessions
    await expect(dialog.getByRole('button', { name: 'Connect' }).first()).toBeVisible();
  });

  test('owner can generate a share link', async ({ page }) => {
    // Login as owner
    await page.goto('/');
    await page.getByLabel('Username').fill('pw-share-owner');
    await page.getByLabel('Password').fill('pass1234');
    await page.getByRole('button', { name: 'Sign In' }).click();
    await expect(page.getByLabel('Manage sessions')).toBeVisible();

    // Open session manager and click Share
    await page.getByLabel('Manage sessions').click();
    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();
    await dialog.getByRole('button', { name: 'Share' }).first().click();

    // Share dialog should appear
    await expect(page.getByText('Share Session')).toBeVisible();

    // Generate share link
    await page.getByRole('button', { name: 'Generate Share Link' }).click();

    // Should show the generated link with a Copy button
    await expect(page.getByRole('button', { name: 'Copy' })).toBeVisible({ timeout: 5_000 });

    // The link input should contain the share URL
    const linkInput = page.locator('input[readonly]');
    await expect(linkInput).toBeVisible();
    const linkValue = await linkInput.inputValue();
    expect(linkValue).toContain('/session/');
    expect(linkValue).toContain('share_token=');
  });

  test('owner can revoke a share from the dialog', async ({ page, request }) => {
    // First, verify shares exist
    const sharesRes = await listSessionShares(request, ownerToken, sessionId);
    const shares = await sharesRes.json();
    expect(shares.length).toBeGreaterThan(0);

    // Login as owner
    await page.goto('/');
    await page.getByLabel('Username').fill('pw-share-owner');
    await page.getByLabel('Password').fill('pass1234');
    await page.getByRole('button', { name: 'Sign In' }).click();
    await expect(page.getByLabel('Manage sessions')).toBeVisible();

    // Open session manager and click Share
    await page.getByLabel('Manage sessions').click();
    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();
    await dialog.getByRole('button', { name: 'Share' }).first().click();

    // Share dialog should show current shares
    await expect(page.getByText('Share Session')).toBeVisible();
    await expect(page.getByText('Current Shares')).toBeVisible({ timeout: 5_000 });

    // Revoke the first share
    await page.getByRole('button', { name: 'Revoke' }).first().click();

    // The revoked share should disappear (wait a moment for refresh)
    // Check via API that a share was removed
    const afterRes = await listSessionShares(request, ownerToken, sessionId);
    const afterShares = await afterRes.json();
    expect(afterShares.length).toBeLessThan(shares.length);
  });

  test('viewer no longer sees revoked shared session', async ({ page, request }) => {
    // Revoke all remaining shares via API to ensure clean state
    const sharesRes = await listSessionShares(request, ownerToken, sessionId);
    const shares = await sharesRes.json();
    for (const share of shares) {
      // Only revoke username-based shares (the viewer's share)
      if (share.user_id === viewerUserId) {
        await request.delete(
          `http://localhost:3847/api/sessions/${sessionId}/shares/${share.id}`,
          { headers: { Authorization: `Bearer ${ownerToken}` } },
        );
      }
    }

    // Login as viewer
    await page.goto('/');
    await page.getByLabel('Username').fill('pw-share-viewer');
    await page.getByLabel('Password').fill('pass1234');
    await page.getByRole('button', { name: 'Sign In' }).click();
    await expect(page.getByLabel('Manage sessions')).toBeVisible();

    // Open session manager
    await page.getByLabel('Manage sessions').click();
    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();

    // Should not see the previously shared session
    await expect(dialog.getByText('Shared by pw-share-owner')).not.toBeVisible();
  });
});

test.describe('Session Sharing - Share Dialog Validation', () => {

  let adminToken: string;
  let userId: string;
  let userToken: string;
  let sessionId: string;

  test.beforeAll(async ({ request }) => {
    adminToken = await getAdminToken(request);

    const userRes = await createTestUser(request, adminToken, {
      username: 'pw-share-val',
      password: 'pass1234',
    });
    userId = (await userRes.json()).id;
    userToken = await loginAs(request, 'pw-share-val', 'pass1234');

    const sessRes = await createTestSession(request, userToken, 'test-container', userId);
    sessionId = (await sessRes.json()).id;
    await waitForSessionRunning(request, userToken, sessionId);
  });

  test.afterAll(async ({ request }) => {
    await terminateTestSession(request, adminToken, sessionId).catch(() => {});
    await deleteTestUser(request, adminToken, userId).catch(() => {});
  });

  test('shows error when inviting nonexistent user', async ({ page }) => {
    await page.goto('/');
    await page.getByLabel('Username').fill('pw-share-val');
    await page.getByLabel('Password').fill('pass1234');
    await page.getByRole('button', { name: 'Sign In' }).click();
    await expect(page.getByLabel('Manage sessions')).toBeVisible();

    await page.getByLabel('Manage sessions').click();
    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();
    await dialog.getByRole('button', { name: 'Share' }).first().click();
    await expect(page.getByText('Share Session')).toBeVisible();

    // Try to invite a user that doesn't exist
    await page.getByPlaceholder('Enter username').fill('nonexistent-ghost-user');
    await page.getByRole('button', { name: 'Invite' }).click();

    // Should show an error message
    await expect(page.getByText(/not found/i)).toBeVisible({ timeout: 5_000 });
  });

  test('shows error when inviting self', async ({ page }) => {
    await page.goto('/');
    await page.getByLabel('Username').fill('pw-share-val');
    await page.getByLabel('Password').fill('pass1234');
    await page.getByRole('button', { name: 'Sign In' }).click();
    await expect(page.getByLabel('Manage sessions')).toBeVisible();

    await page.getByLabel('Manage sessions').click();
    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();
    await dialog.getByRole('button', { name: 'Share' }).first().click();
    await expect(page.getByText('Share Session')).toBeVisible();

    // Try to invite yourself
    await page.getByPlaceholder('Enter username').fill('pw-share-val');
    await page.getByRole('button', { name: 'Invite' }).click();

    // Should show an error message
    await expect(page.getByText(/cannot share.*yourself/i)).toBeVisible({ timeout: 5_000 });
  });

  test('share dialog closes when clicking close button', async ({ page }) => {
    await page.goto('/');
    await page.getByLabel('Username').fill('pw-share-val');
    await page.getByLabel('Password').fill('pass1234');
    await page.getByRole('button', { name: 'Sign In' }).click();
    await expect(page.getByLabel('Manage sessions')).toBeVisible();

    await page.getByLabel('Manage sessions').click();
    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();
    await dialog.getByRole('button', { name: 'Share' }).first().click();
    await expect(page.getByText('Share Session')).toBeVisible();

    // Close the share dialog (the X button inside the dialog header)
    // The share dialog has its own close button separate from the session manager
    const shareDialogClose = page.locator('.fixed.top-1\\/2 button').first();
    await shareDialogClose.click();

    // Share dialog should close but session manager should still be open
    await expect(page.getByText('Share Session')).not.toBeVisible();
  });
});

test.describe('Session Sharing - Shared Sessions Empty State', () => {

  let adminToken: string;
  let userId: string;

  test.beforeAll(async ({ request }) => {
    adminToken = await getAdminToken(request);

    const userRes = await createTestUser(request, adminToken, {
      username: 'pw-share-empty',
      password: 'pass1234',
    });
    userId = (await userRes.json()).id;
  });

  test.afterAll(async ({ request }) => {
    await deleteTestUser(request, adminToken, userId).catch(() => {});
  });

  test('user with no shared sessions sees empty session manager', async ({ page }) => {
    await page.goto('/');
    await page.getByLabel('Username').fill('pw-share-empty');
    await page.getByLabel('Password').fill('pass1234');
    await page.getByRole('button', { name: 'Sign In' }).click();
    await expect(page.getByLabel('Manage sessions')).toBeVisible();

    // Open session manager
    await page.getByLabel('Manage sessions').click();
    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();

    // Wait for loading spinner to finish, then check empty state
    await expect(dialog.locator('.animate-spin')).toBeHidden({ timeout: 15_000 });
    await expect(dialog.getByText('No active sessions')).toBeVisible();
  });
});
