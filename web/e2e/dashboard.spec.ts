import { test, expect } from '@playwright/test';
import { getAdminToken, createTestApp, deleteTestApp } from './helpers/api';

const TEST_APPS = [
  { id: 'e2e-app-one', name: 'Test App One', category: 'Development' },
  { id: 'e2e-app-two', name: 'Test App Two', category: 'Productivity' },
  { id: 'e2e-app-three', name: 'Test App Three', category: 'Development' },
];

test.describe('Dashboard', () => {
  test.describe.configure({ mode: 'serial' });

  let token: string;

  test.beforeAll(async ({ request }) => {
    token = await getAdminToken(request);
    for (const app of TEST_APPS) {
      const res = await createTestApp(request, token, app);
      if (!res.ok()) {
        console.error(`Failed to create ${app.id}: ${res.status()} ${await res.text()}`);
      }
    }
  });

  test.afterAll(async ({ request }) => {
    for (const app of TEST_APPS) {
      await deleteTestApp(request, token, app.id);
    }
  });

  test('displays app cards', async ({ page }) => {
    await page.goto('/');
    await expect(page.getByLabel('Manage sessions')).toBeVisible();

    for (const app of TEST_APPS) {
      await expect(page.getByText(app.name)).toBeVisible();
    }
  });

  test('filters apps by search via command palette', async ({ page }) => {
    await page.goto('/');
    await expect(page.getByLabel('Manage sessions')).toBeVisible();

    // Open command palette with keyboard shortcut
    await page.keyboard.press('Control+k');
    const palette = page.locator('[role="listbox"]');
    await expect(palette).toBeVisible();

    // Type search query
    await page.getByPlaceholder('Search apps and actions...').fill('App One');

    // Only matching app should appear in palette results
    await expect(palette.getByText('Test App One')).toBeVisible();
    await expect(palette.getByText('Test App Two')).not.toBeVisible();
    await expect(palette.getByText('Test App Three')).not.toBeVisible();

    // Close palette
    await page.keyboard.press('Escape');
  });

  test('filters apps by category', async ({ page }) => {
    await page.goto('/');
    await expect(page.getByLabel('Manage sessions')).toBeVisible();

    // Click the Productivity category filter pill (rounded-full buttons)
    await page.locator('button.rounded-full', { hasText: /Productivity/ }).click();
    await expect(page.getByText('Test App Two')).toBeVisible();
    await expect(page.getByText('Test App One')).not.toBeVisible();

    // Click All to reset
    await page.locator('button.rounded-full', { hasText: /^All/ }).click();
    await expect(page.getByText('Test App One')).toBeVisible();
    await expect(page.getByText('Test App Two')).toBeVisible();
  });

  test('toggles favorites', async ({ page }) => {
    await page.goto('/');
    await expect(page.getByText('Test App One')).toBeVisible();

    // Find the card containing "Test App One" — walk up to the group container
    const appCard = page.locator('[class*="group"]', { hasText: 'Test App One' }).first();

    // Add to favorites
    await appCard.getByRole('button', { name: /Add to favorites/i }).click();

    // Should now show "Remove from favorites"
    await expect(appCard.getByRole('button', { name: /Remove from favorites/i })).toBeVisible();

    // Remove from favorites
    await appCard.getByRole('button', { name: /Remove from favorites/i }).click();
    await expect(appCard.getByRole('button', { name: /Add to favorites/i })).toBeVisible();
  });

  test('shows empty state for unmatched search', async ({ page }) => {
    await page.goto('/');
    await expect(page.getByLabel('Manage sessions')).toBeVisible();

    // Open command palette
    await page.keyboard.press('Control+k');
    await expect(page.getByPlaceholder('Search apps and actions...')).toBeVisible();

    await page.getByPlaceholder('Search apps and actions...').fill('xyznonexistent');
    await expect(page.getByText('No results found')).toBeVisible();

    // Close palette
    await page.keyboard.press('Escape');
  });
});
