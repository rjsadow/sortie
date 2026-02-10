import { test as setup, expect } from '@playwright/test';

const ADMIN_FILE = 'e2e/.auth/admin.json';

setup('authenticate as admin', async ({ page }) => {
  await page.goto('/');

  // Fill in admin credentials
  await page.getByLabel('Username').fill('admin');
  await page.getByLabel('Password').fill('admin123');
  await page.getByRole('button', { name: 'Sign In' }).click();

  // Wait for dashboard to load
  await expect(page.getByLabel('Manage sessions')).toBeVisible();

  // Save signed-in state
  await page.context().storageState({ path: ADMIN_FILE });
});
