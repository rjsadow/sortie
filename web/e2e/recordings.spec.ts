import { test, expect } from '@playwright/test';
import {
  getAdminToken,
  listRecordings,
  listAdminRecordings,
  getAdminSettings,
  updateAdminSettings,
} from './helpers/api';

test.describe('Recordings', () => {
  const openAdminPanel = async (page: import('@playwright/test').Page) => {
    await page.getByLabel('User menu').click();
    await page.getByRole('button', { name: 'Admin Panel' }).click();
  };

  test('recordings button is visible in header', async ({ page }) => {
    await page.goto('/');
    await expect(page.getByLabel('Manage sessions')).toBeVisible();
    await expect(page.getByLabel('View recordings')).toBeVisible();
  });

  test('opens and closes recordings modal', async ({ page }) => {
    await page.goto('/');
    await expect(page.getByLabel('View recordings')).toBeVisible();

    // Open recordings modal
    await page.getByLabel('View recordings').click();
    await expect(page.getByRole('heading', { name: 'My Recordings' })).toBeVisible();

    // Should show empty state (no recordings yet)
    await expect(page.getByText('No recordings')).toBeVisible();

    // Close modal
    await page.getByLabel('Close').click();
    await expect(page.getByRole('heading', { name: 'My Recordings' })).not.toBeVisible();
  });

  test('recordings modal shows refresh button', async ({ page }) => {
    await page.goto('/');
    await page.getByLabel('View recordings').click();
    await expect(page.getByRole('heading', { name: 'My Recordings' })).toBeVisible();
    await expect(page.getByLabel('Refresh')).toBeVisible();
  });

  test('admin panel shows recordings tab', async ({ page }) => {
    await page.goto('/');
    await expect(page.getByLabel('Manage sessions')).toBeVisible();

    await openAdminPanel(page);
    await expect(page.getByRole('heading', { name: 'Admin Settings' })).toBeVisible();

    // Click Recordings tab
    await page.getByRole('button', { name: 'Recordings', exact: true }).click();
    await expect(page.getByText('All Recordings')).toBeVisible();

    // Should show empty state
    await expect(page.getByText('No recordings found')).toBeVisible();
  });

  test('admin settings tab has auto-record toggle', async ({ page }) => {
    await page.goto('/');
    await expect(page.getByLabel('Manage sessions')).toBeVisible();

    await openAdminPanel(page);
    await expect(page.getByRole('heading', { name: 'Admin Settings' })).toBeVisible();

    // Settings tab should be active by default for system admins
    await expect(page.getByText('Auto-record sessions')).toBeVisible();
    await expect(
      page.getByText('When enabled, recording starts automatically for all new sessions'),
    ).toBeVisible();
  });

  test('auto-record toggle persists setting', async ({ page, request }) => {
    const token = await getAdminToken(request);

    // Ensure auto-record is off initially
    await updateAdminSettings(request, token, { recording_auto_record: 'false' });

    await page.goto('/');
    await expect(page.getByLabel('Manage sessions')).toBeVisible();

    await openAdminPanel(page);
    await expect(page.getByRole('heading', { name: 'Admin Settings' })).toBeVisible();

    // Find the auto-record checkbox and enable it
    const autoRecordLabel = page.getByText('Auto-record sessions');
    const checkbox = autoRecordLabel.locator('..').locator('..').getByRole('checkbox');

    // It should start unchecked
    await expect(checkbox).not.toBeChecked();

    // Enable it
    await checkbox.check();
    await expect(checkbox).toBeChecked();

    // Save settings
    await page.getByRole('button', { name: 'Save Settings' }).click();
    await expect(page.getByText('Settings saved successfully')).toBeVisible();

    // Verify via API that it persisted
    const settingsRes = await getAdminSettings(request, token);
    const settings = await settingsRes.json();
    expect(settings.recording_auto_record).toBe('true');

    // Clean up: disable auto-record
    await updateAdminSettings(request, token, { recording_auto_record: 'false' });
  });

  test('recordings API returns empty list', async ({ request }) => {
    const token = await getAdminToken(request);

    // User recordings endpoint
    const userRes = await listRecordings(request, token);
    expect(userRes.ok()).toBeTruthy();
    const userRecs = await userRes.json();
    expect(Array.isArray(userRecs)).toBeTruthy();

    // Admin recordings endpoint
    const adminRes = await listAdminRecordings(request, token);
    expect(adminRes.ok()).toBeTruthy();
    const adminRecs = await adminRes.json();
    expect(Array.isArray(adminRecs)).toBeTruthy();
  });
});
