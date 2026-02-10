import { test, expect } from '@playwright/test';

// Auth tests run without saved state — they test login/logout flows directly.
test.use({ storageState: { cookies: [], origins: [] } });

test.describe('Authentication', () => {
  test('shows login page when unauthenticated', async ({ page }) => {
    await page.goto('/');
    await expect(page.getByText('Sign in to access your applications')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Sign In' })).toBeVisible();
  });

  test('logs in with valid admin credentials', async ({ page }) => {
    await page.goto('/');
    await page.getByLabel('Username').fill('admin');
    await page.getByLabel('Password').fill('admin123');
    await page.getByRole('button', { name: 'Sign In' }).click();

    // Should reach the dashboard
    await expect(page.getByPlaceholder('Search applications...')).toBeVisible();
    // Admin should see admin button
    await expect(page.getByRole('button', { name: /Admin settings/i })).toBeVisible();
  });

  test('shows error for invalid credentials', async ({ page }) => {
    await page.goto('/');
    await page.getByLabel('Username').fill('admin');
    await page.getByLabel('Password').fill('wrongpassword');
    await page.getByRole('button', { name: 'Sign In' }).click();

    await expect(page.getByText(/invalid/i)).toBeVisible();
  });

  test('shows validation error for empty username', async ({ page }) => {
    await page.goto('/');
    await page.getByLabel('Password').fill('somepassword');
    await page.getByRole('button', { name: 'Sign In' }).click();

    await expect(page.getByText('Username is required')).toBeVisible();
  });

  test('shows validation error for empty password', async ({ page }) => {
    await page.goto('/');
    await page.getByLabel('Username').fill('admin');
    await page.getByRole('button', { name: 'Sign In' }).click();

    await expect(page.getByText('Password is required')).toBeVisible();
  });

  test('logs out and returns to login page', async ({ page }) => {
    // First log in
    await page.goto('/');
    await page.getByLabel('Username').fill('admin');
    await page.getByLabel('Password').fill('admin123');
    await page.getByRole('button', { name: 'Sign In' }).click();
    await expect(page.getByPlaceholder('Search applications...')).toBeVisible();

    // Log out
    await page.getByRole('button', { name: /Sign out/i }).click();

    // Should be back at login
    await expect(page.getByText('Sign in to access your applications')).toBeVisible();
  });

  test('navigates to registration form and back', async ({ page }) => {
    await page.goto('/');

    // Click register link
    await page.getByText("Don't have an account? Register").click();

    // Should see registration form
    await expect(page.getByText('Register to access Sortie')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Create Account' })).toBeVisible();

    // Go back to login
    await page.getByText('Already have an account? Sign in').click();
    await expect(page.getByText('Sign in to access your applications')).toBeVisible();
  });

  test('registers a new user and auto-logs in', async ({ page, request }) => {
    const username = `testuser_${Date.now()}`;

    await page.goto('/');
    await page.getByText("Don't have an account? Register").click();

    await page.getByPlaceholder('Choose a username').fill(username);
    await page.getByPlaceholder('your@email.com').fill(`${username}@test.com`);
    await page.getByPlaceholder('At least 6 characters').fill('testpass123');
    await page.getByPlaceholder('Confirm your password').fill('testpass123');
    await page.getByRole('button', { name: 'Create Account' }).click();

    // Should be auto-logged in to dashboard
    await expect(page.getByPlaceholder('Search applications...')).toBeVisible();
  });
});
