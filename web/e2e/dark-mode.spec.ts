import { test, expect } from '@playwright/test';

test.describe('Dark Mode', () => {
  test('toggles dark mode on and off', async ({ page }) => {
    await page.goto('/');
    await expect(page.getByPlaceholder('Search applications...')).toBeVisible();

    // Click the dark mode toggle
    const toggle = page.getByRole('button', { name: /Switch to (light|dark) mode/i });
    await toggle.click();

    // Check the html element class
    const htmlClass = await page.locator('html').getAttribute('class');
    const isDark = htmlClass?.includes('dark');

    // Toggle again - should go to the opposite mode
    await toggle.click();
    const htmlClass2 = await page.locator('html').getAttribute('class');
    const isDark2 = htmlClass2?.includes('dark');

    expect(isDark).not.toBe(isDark2);
  });

  test('persists dark mode across page reload', async ({ page }) => {
    await page.goto('/');
    await expect(page.getByPlaceholder('Search applications...')).toBeVisible();

    // Determine current state
    const initialClass = await page.locator('html').getAttribute('class');
    const initiallyDark = initialClass?.includes('dark') ?? false;

    // Toggle to opposite state
    await page.getByRole('button', { name: /Switch to (light|dark) mode/i }).click();

    // Verify it toggled
    const toggledClass = await page.locator('html').getAttribute('class');
    const nowDark = toggledClass?.includes('dark') ?? false;
    expect(nowDark).toBe(!initiallyDark);

    // Reload the page
    await page.reload();
    await expect(page.getByPlaceholder('Search applications...')).toBeVisible();

    // Verify persisted state
    const afterReloadClass = await page.locator('html').getAttribute('class');
    const darkAfterReload = afterReloadClass?.includes('dark') ?? false;
    expect(darkAfterReload).toBe(nowDark);
  });

  test('stores preference in localStorage', async ({ page }) => {
    await page.goto('/');
    await expect(page.getByPlaceholder('Search applications...')).toBeVisible();

    // Toggle dark mode
    await page.getByRole('button', { name: /Switch to (light|dark) mode/i }).click();

    // Check localStorage
    const theme = await page.evaluate(() => localStorage.getItem('sortie-theme'));
    expect(theme).toBeTruthy();
  });
});
