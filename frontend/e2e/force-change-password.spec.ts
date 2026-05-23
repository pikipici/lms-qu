import { expect, test } from '@playwright/test';

const siswaUser = {
  id: '33333333-3333-4333-8333-333333333333',
  name: 'Siswa E2E',
  email: 'siswa.e2e@sekolah.id',
  role: 'siswa',
  status: 'active',
  must_change_password: true,
  last_login_at: null,
  created_at: '2026-05-23T00:00:00Z',
  updated_at: '2026-05-23T00:00:00Z',
};

test.describe('E2E Flow #2: Force-Change-Password', () => {
  test('redirects to security page on first login', async ({ page }) => {
    await page.route('**/api/v1/auth/login', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          user: { ...siswaUser, must_change_password: true },
          tokens: {
            access_token: 'access-token-force-change-e2e',
            refresh_token: 'refresh-token-force-change-e2e',
          },
        }),
      });
    });

    await page.goto('/');
    await page.getByRole('link', { name: 'Masuk' }).click();
    await page.getByLabel('Email').fill(siswaUser.email);
    await page.getByLabel('Password').fill('password-e2e');
    await page.getByRole('button', { name: 'Masuk' }).click();

    await expect(page).toHaveURL(/\/me\/security$/);
    await expect(page.getByRole('heading', { name: 'Keamanan' })).toBeVisible();
    await expect(page.getByText('Wajib ganti password')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Ganti password' })).toBeVisible();
  });

  test('changes password and redirects to dashboard', async ({ page }) => {
    await page.route('**/api/v1/auth/login', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          user: { ...siswaUser, must_change_password: true },
          tokens: {
            access_token: 'access-token-force-change-e2e',
            refresh_token: 'refresh-token-force-change-e2e',
          },
        }),
      });
    });

    await page.route('**/api/v1/auth/change-password', async (route) => {
      const request = route.request();
      expect(request.method()).toBe('POST');
      const body = JSON.parse(await request.postText());
      expect(body.email).toBe(siswaUser.email);
      expect(body.current_password).toBe('password-e2e');
      expect(body.new_password).toBe('NewPassword123!');

      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          user: { ...siswaUser, must_change_password: false },
          tokens: {
            access_token: 'access-token-new-password-e2e',
            refresh_token: 'refresh-token-new-password-e2e',
          },
        }),
      });
    });

    await page.goto('/');
    await page.getByRole('link', { name: 'Masuk' }).click();
    await page.getByLabel('Email').fill(siswaUser.email);
    await page.getByLabel('Password').fill('password-e2e');
    await page.getByRole('button', { name: 'Masuk' }).click();

    await expect(page).toHaveURL(/\/me\/security$/);
    
    await page.getByRole('button', { name: 'Ganti password' }).click();
    await expect(page.getByPlaceholder('Password lama')).toBeVisible();
    await expect(page.getByPlaceholder('Password baru')).toBeVisible();
    await expect(page.getByPlaceholder('Ulangi password')).toBeVisible();

    await page.getByPlaceholder('Password lama').fill('password-e2e');
    await page.getByPlaceholder('Password baru').fill('NewPassword123!');
    await page.getByPlaceholder('Ulangi password').fill('NewPassword123!');
    await page.getByRole('button', { name: 'Simpan' }).click();

    await expect(page.getByText('Password berhasil diganti')).toBeVisible();
    await expect(page).toHaveURL(/\/siswa$/);
    await expect(page.getByText('Siswa E2E')).toBeVisible();
  });

  test('subsequent login uses new password', async ({ page }) => {
    await page.route('**/api/v1/auth/login', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          user: { ...siswaUser, must_change_password: false },
          tokens: {
            access_token: 'access-token-new-password-e2e',
            refresh_token: 'refresh-token-new-password-e2e',
          },
        }),
      });
    });

    await page.goto('/');
    await page.getByRole('link', { name: 'Masuk' }).click();
    await page.getByLabel('Email').fill(siswaUser.email);
    await page.getByLabel('Password').fill('NewPassword123!');
    await page.getByRole('button', { name: 'Masuk' }).click();

    await expect(page).toHaveURL(/\/siswa$/);
    await expect(page.getByRole('heading', { name: 'Dashboard' })).toBeVisible();
  });
});