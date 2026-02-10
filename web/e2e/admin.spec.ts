import { test, expect } from '@playwright/test';

test.describe('Admin Panel', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/');
    await expect(page.getByPlaceholder('Search applications...')).toBeVisible();
  });

  test('opens admin panel', async ({ page }) => {
    await page.getByRole('button', { name: /Admin settings/i }).click();
    await expect(page.getByRole('heading', { name: 'Admin Settings' })).toBeVisible();
  });

  test('shows all five tabs', async ({ page }) => {
    await page.getByRole('button', { name: /Admin settings/i }).click();
    await expect(page.getByRole('heading', { name: 'Admin Settings' })).toBeVisible();

    for (const tab of ['Settings', 'Users', 'Apps', 'Templates', 'Sessions']) {
      await expect(page.getByRole('button', { name: tab, exact: true })).toBeVisible();
    }
  });

  test('switches between tabs', async ({ page }) => {
    await page.getByRole('button', { name: /Admin settings/i }).click();
    await expect(page.getByRole('heading', { name: 'Admin Settings' })).toBeVisible();

    // Click Users tab
    await page.getByRole('button', { name: 'Users', exact: true }).click();
    await expect(page.getByRole('button', { name: 'Create User' })).toBeVisible();

    // Click Apps tab
    await page.getByRole('button', { name: 'Apps', exact: true }).click();
    await expect(page.getByRole('button', { name: 'Create App' })).toBeVisible();

    // Click Templates tab
    await page.getByRole('button', { name: 'Templates', exact: true }).click();
    await expect(page.getByRole('button', { name: 'Create Template' })).toBeVisible();

    // Click Sessions tab
    await page.getByRole('button', { name: 'Sessions', exact: true }).click();
    await expect(page.getByRole('button', { name: 'Refresh' })).toBeVisible();
  });

  test('creates and deletes a user', async ({ page }) => {
    // Auto-accept confirmation dialogs for delete operations
    page.on('dialog', (dialog) => dialog.accept());

    await page.getByRole('button', { name: /Admin settings/i }).click();
    await page.getByRole('button', { name: 'Users', exact: true }).click();

    // Open create user form
    await page.getByRole('button', { name: 'Create User' }).click();

    // Fill out the form
    const username = `e2e_user_${Date.now()}`;
    await page.getByPlaceholder('Username *').fill(username);
    await page.getByPlaceholder('Password *').fill('testpass123');
    await page.getByPlaceholder('Email (optional)').fill(`${username}@test.com`);

    // Submit - click the green Create User button (inside the form, not the toggle)
    await page.locator('button.bg-green-600').filter({ hasText: 'Create User' }).click();

    // Verify user appears in table
    await expect(page.getByRole('cell', { name: username, exact: true })).toBeVisible();

    // Delete the user
    const row = page.locator('tr', { hasText: username });
    await row.getByRole('button', { name: /Delete/i }).click();

    // Verify user is removed
    await expect(page.getByRole('cell', { name: username, exact: true })).not.toBeVisible();
  });

  test('creates and deletes an app', async ({ page }) => {
    // Auto-accept confirmation dialogs for delete operations
    page.on('dialog', (dialog) => dialog.accept());

    await page.getByRole('button', { name: /Admin settings/i }).click();
    await page.getByRole('button', { name: 'Apps', exact: true }).click();

    // Open create app form
    await page.getByRole('button', { name: 'Create App' }).click();

    const appId = `e2e-app-${Date.now()}`;
    await page.getByPlaceholder('e.g., my-app').fill(appId);
    await page.getByPlaceholder('e.g., My Application').fill('E2E Test App');
    await page.getByPlaceholder('Brief description of the application').fill('Created by E2E test');
    await page.getByPlaceholder('e.g., Development').fill('Testing');

    // URL launch type is default, fill the URL field (exact match avoids icon URL field)
    await page.getByPlaceholder('https://example.com', { exact: true }).fill('https://example.com');

    // Submit
    await page.getByRole('button', { name: 'Create App', exact: true }).last().click();

    // Verify app appears in table (use app ID for unique match)
    await expect(page.getByRole('cell', { name: appId })).toBeVisible();

    // Delete the app using the unique app ID to find the correct row
    const row = page.getByRole('row', { name: new RegExp(appId) });
    await row.getByRole('button', { name: /Delete/i }).click();

    // Verify app is removed
    await expect(page.getByRole('cell', { name: appId })).not.toBeVisible();
  });

  test('closes admin panel', async ({ page }) => {
    await page.getByRole('button', { name: /Admin settings/i }).click();
    await expect(page.getByRole('heading', { name: 'Admin Settings' })).toBeVisible();

    // Close via the X button (first SVG close button in the panel)
    await page.locator('.fixed').getByRole('button').first().click();

    // Panel should be gone
    await expect(page.getByRole('heading', { name: 'Admin Settings' })).not.toBeVisible();
  });
});
