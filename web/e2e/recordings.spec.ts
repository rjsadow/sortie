import { test, expect } from '@playwright/test';
import {
  getAdminToken,
  createTestSession,
  listSessions,
  terminateTestSession,
  waitForSessionRunning,
  listRecordings,
  listAdminRecordings,
  getAdminSettings,
  updateAdminSettings,
  startRecording,
  stopRecording,
  uploadRecording,
  downloadRecording,
  deleteRecording,
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

test.describe('recording lifecycle', () => {
  test.describe.configure({ mode: 'serial' });

  const openAdminPanel = async (page: import('@playwright/test').Page) => {
    await page.getByLabel('User menu').click();
    await page.getByRole('button', { name: 'Admin Panel' }).click();
  };

  let token: string;

  test.beforeAll(async ({ request }) => {
    token = await getAdminToken(request);
  });

  test.afterEach(async ({ request }) => {
    // Clean up all recordings to avoid polluting other test blocks
    const recRes = await listAdminRecordings(request, token);
    if (recRes.ok()) {
      const recs = await recRes.json();
      for (const r of recs) {
        await deleteRecording(request, token, r.id);
      }
    }
    // Clean up all sessions
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

  test('full recording API lifecycle via request API', async ({ request }) => {
    // Create container session and wait for running
    const createRes = await createTestSession(request, token, 'test-container');
    expect(createRes.ok()).toBeTruthy();
    const session = await createRes.json();
    await waitForSessionRunning(request, token, session.id);

    // Start recording
    const startRes = await startRecording(request, token, session.id);
    expect(startRes.status()).toBe(201);
    const startBody = await startRes.json();
    const recordingId = startBody.recording_id;
    expect(recordingId).toBeTruthy();

    // Stop recording
    const stopRes = await stopRecording(request, token, session.id, recordingId);
    expect(stopRes.status()).toBe(200);

    // Upload recording
    const fileContent = Buffer.from('fake-video-content-for-e2e-test');
    const uploadRes = await uploadRecording(
      request,
      token,
      session.id,
      recordingId,
      fileContent,
    );
    expect(uploadRes.status()).toBe(200);

    // List recordings - verify it's present with status "ready"
    const listRes = await listRecordings(request, token);
    expect(listRes.ok()).toBeTruthy();
    const recs = await listRes.json();
    const found = recs.find((r: { id: string }) => r.id === recordingId);
    expect(found).toBeTruthy();
    expect(found.status).toBe('ready');

    // Download recording - verify content matches
    const dlRes = await downloadRecording(request, token, recordingId);
    expect(dlRes.status()).toBe(200);
    const dlBody = await dlRes.body();
    expect(dlBody.toString()).toBe(fileContent.toString());

    // Delete recording
    const delRes = await deleteRecording(request, token, recordingId);
    expect(delRes.status()).toBe(204);

    // Verify recording is gone
    const listRes2 = await listRecordings(request, token);
    const recs2 = await listRes2.json();
    const gone = recs2.find((r: { id: string }) => r.id === recordingId);
    expect(gone).toBeUndefined();

    // Session cleanup handled by afterEach
  });

  test('recordings modal shows recording after API upload', async ({ page, request }) => {
    // Create session and upload recording via API
    const createRes = await createTestSession(request, token, 'test-container');
    const session = await createRes.json();
    await waitForSessionRunning(request, token, session.id);

    const startRes = await startRecording(request, token, session.id);
    const startBody = await startRes.json();
    const recordingId = startBody.recording_id;

    await stopRecording(request, token, session.id, recordingId);
    await uploadRecording(
      request,
      token,
      session.id,
      recordingId,
      Buffer.from('modal-test-video'),
    );

    // Navigate to dashboard and open recordings modal
    await page.goto('/');
    await expect(page.getByLabel('View recordings')).toBeVisible();
    await page.getByLabel('View recordings').click();
    await expect(page.getByRole('heading', { name: 'My Recordings' })).toBeVisible();

    // Assert recording filename is visible (contains .vncrec)
    await expect(page.getByText('.vncrec').first()).toBeVisible();
    // Assert "ready" status is visible
    await expect(page.getByText('ready').first()).toBeVisible();

    // Cleanup via API (afterEach handles recordings and sessions)
  });

  test('admin recordings tab shows all users recordings', async ({ page, request }) => {
    // Upload recording as admin
    const createRes = await createTestSession(request, token, 'test-container');
    const session = await createRes.json();
    await waitForSessionRunning(request, token, session.id);

    const startRes = await startRecording(request, token, session.id);
    const startBody = await startRes.json();
    const recordingId = startBody.recording_id;

    await stopRecording(request, token, session.id, recordingId);
    await uploadRecording(
      request,
      token,
      session.id,
      recordingId,
      Buffer.from('admin-tab-test-video'),
    );

    // Open admin panel → Recordings tab
    await page.goto('/');
    await expect(page.getByLabel('Manage sessions')).toBeVisible();
    await openAdminPanel(page);
    await expect(page.getByRole('heading', { name: 'Admin Settings' })).toBeVisible();

    await page.getByRole('button', { name: 'Recordings', exact: true }).click();
    await expect(page.getByText('All Recordings')).toBeVisible();

    // Assert recording visible in admin table
    await expect(page.getByText('.vncrec').first()).toBeVisible();

    // Cleanup via API (afterEach handles recordings and sessions)
  });

  test('delete recording from recordings modal', async ({ page, request }) => {
    // Upload recording via API
    const createRes = await createTestSession(request, token, 'test-container');
    const session = await createRes.json();
    await waitForSessionRunning(request, token, session.id);

    const startRes = await startRecording(request, token, session.id);
    const startBody = await startRes.json();
    const recordingId = startBody.recording_id;

    await stopRecording(request, token, session.id, recordingId);
    await uploadRecording(
      request,
      token,
      session.id,
      recordingId,
      Buffer.from('delete-modal-test-video'),
    );

    // Open recordings modal, verify recording visible
    await page.goto('/');
    await page.getByLabel('View recordings').click();
    await expect(page.getByRole('heading', { name: 'My Recordings' })).toBeVisible();
    await expect(page.getByText('.vncrec').first()).toBeVisible();

    // Set up dialog handler to accept confirmation
    page.on('dialog', (d) => d.accept());

    // Click Delete button
    await page.getByRole('button', { name: 'Delete' }).first().click();

    // Assert recording disappears
    await expect(page.getByText('No recordings')).toBeVisible({ timeout: 5000 });

    // Cleanup via API (afterEach handles recordings and sessions)
  });

  test('download recording via API and verify content', async ({ request }) => {
    // Upload recording via API
    const createRes = await createTestSession(request, token, 'test-container');
    const session = await createRes.json();
    await waitForSessionRunning(request, token, session.id);

    const startRes = await startRecording(request, token, session.id);
    const startBody = await startRes.json();
    const recordingId = startBody.recording_id;

    const fileContent = Buffer.from('download-test-video-content');
    await stopRecording(request, token, session.id, recordingId);
    await uploadRecording(request, token, session.id, recordingId, fileContent);

    // Download and verify content matches what was uploaded
    const dlRes = await downloadRecording(request, token, recordingId);
    expect(dlRes.status()).toBe(200);
    expect(dlRes.headers()['content-type']).toBe('application/octet-stream');
    expect(dlRes.headers()['content-disposition']).toContain('.vncrec');
    const dlBody = await dlRes.body();
    expect(dlBody.toString()).toBe(fileContent.toString());

    // Cleanup via API (afterEach handles recordings and sessions)
  });
});
