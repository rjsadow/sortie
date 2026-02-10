import { test, expect } from '@playwright/test';

test.describe('Dark Mode', () => {
  test('toggles dark mode on and off', async ({ page }) => {
    await page.goto('/');
    await expect(page.getByLabel('Manage sessions')).toBeVisible();

    // Open user menu and toggle dark mode
    await page.getByLabel('User menu').click();
    await page.getByRole('button', { name: /Dark Mode/i }).click();

    // Check the html element class
    const htmlClass = await page.locator('html').getAttribute('class');
    const isDark = htmlClass?.includes('dark');

    // Toggle again — reopen menu since it closes after each action
    await page.getByLabel('User menu').click();
    await page.getByRole('button', { name: /Dark Mode/i }).click();
    const htmlClass2 = await page.locator('html').getAttribute('class');
    const isDark2 = htmlClass2?.includes('dark');

    expect(isDark).not.toBe(isDark2);
  });

  test('persists dark mode across page reload', async ({ page }) => {
    await page.goto('/');
    await expect(page.getByLabel('Manage sessions')).toBeVisible();

    // Determine current state
    const initialClass = await page.locator('html').getAttribute('class');
    const initiallyDark = initialClass?.includes('dark') ?? false;

    // Toggle to opposite state via user menu
    await page.getByLabel('User menu').click();
    await page.getByRole('button', { name: /Dark Mode/i }).click();

    // Verify it toggled
    const toggledClass = await page.locator('html').getAttribute('class');
    const nowDark = toggledClass?.includes('dark') ?? false;
    expect(nowDark).toBe(!initiallyDark);

    // Reload the page
    await page.reload();
    await expect(page.getByLabel('Manage sessions')).toBeVisible();

    // Verify persisted state
    const afterReloadClass = await page.locator('html').getAttribute('class');
    const darkAfterReload = afterReloadClass?.includes('dark') ?? false;
    expect(darkAfterReload).toBe(nowDark);
  });

  test('stores preference in localStorage', async ({ page }) => {
    await page.goto('/');
    await expect(page.getByLabel('Manage sessions')).toBeVisible();

    // Toggle dark mode via user menu
    await page.getByLabel('User menu').click();
    await page.getByRole('button', { name: /Dark Mode/i }).click();

    // Check localStorage
    const theme = await page.evaluate(() => localStorage.getItem('sortie-theme'));
    expect(theme).toBeTruthy();
  });
});
