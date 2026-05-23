import { expect, test } from '@playwright/test';

const fakeUser = {
  id: '11111111-1111-4111-8111-111111111111',
  name: 'Admin E2E',
  email: 'admin.e2e@sekolah.id',
  role: 'admin',
  status: 'active',
  must_change_password: false,
  last_login_at: null,
  created_at: '2026-05-23T00:00:00Z',
  updated_at: '2026-05-23T00:00:00Z',
};

test.describe('login smoke', () => {
  test('shows validation errors before submitting', async ({ page }) => {
    await page.goto('/');
    await page.getByRole('link', { name: 'Masuk' }).click();

    await page.getByRole('button', { name: 'Masuk' }).click();

    await expect(page.getByText('Email wajib diisi.')).toBeVisible();
    await expect(page.getByText('Password wajib diisi.')).toBeVisible();
  });

  test('stores admin session and routes to admin dashboard', async ({ page }) => {
    await page.route('**/api/v1/auth/login', async (route) => {
      const request = route.request();
      expect(request.method()).toBe('POST');
      expect(await request.postDataJSON()).toMatchObject({
        email: fakeUser.email,
        password: 'password-e2e',
      });

      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          user: fakeUser,
          tokens: {
            access_token: 'access-token-e2e',
            refresh_token: 'refresh-token-e2e',
          },
        }),
      });
    });

    await page.goto('/');
    await page.getByRole('link', { name: 'Masuk' }).click();
    await page.getByLabel('Email').fill(fakeUser.email);
    await page.getByLabel('Password').fill('password-e2e');
    await page.getByRole('button', { name: 'Masuk' }).click();

    await expect(page).toHaveURL(/\/admin$/);
    await expect(page.getByRole('heading', { name: 'Dashboard' })).toBeVisible();
  });

  test('forces temporary-password users to the security page', async ({ page }) => {
    await page.route('**/api/v1/auth/login', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          user: { ...fakeUser, must_change_password: true },
          tokens: {
            access_token: 'access-token-force-change-e2e',
            refresh_token: 'refresh-token-force-change-e2e',
          },
        }),
      });
    });

    await page.goto('/');
    await page.getByRole('link', { name: 'Masuk' }).click();
    await page.getByLabel('Email').fill(fakeUser.email);
    await page.getByLabel('Password').fill('password-e2e');
    await page.getByRole('button', { name: 'Masuk' }).click();

    await expect(page).toHaveURL(/\/me\/security$/);
    await expect(page.getByRole('heading', { name: 'Keamanan' })).toBeVisible();
    await expect(page.getByText('Wajib ganti password')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Ganti password' })).toBeVisible();
  });
});
