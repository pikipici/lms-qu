import { expect, test } from '@playwright/test';

const guruUser = {
  id: '22222222-2222-4222-8222-222222222222',
  name: 'Guru E2E',
  email: 'guru.e2e@sekolah.id',
  role: 'guru',
  status: 'active',
  must_change_password: false,
  last_login_at: null,
  created_at: '2026-05-23T00:00:00Z',
  updated_at: '2026-05-23T00:00:00Z',
};

test.describe('E2E Flow #1: Guru Login → Dashboard', () => {
  test('validates form before login', async ({ page }) => {
    await page.goto('/login');

    await page.getByRole('button', { name: 'Masuk' }).click();

    await expect(page.getByText('Email wajib diisi.')).toBeVisible();
    await expect(page.getByText('Password wajib diisi.')).toBeVisible();
  });

  test('logs in as guru and routes to guru dashboard', async ({ page }) => {
    await page.route('**/api/v1/auth/login', async (route) => {
      const request = route.request();
      expect(request.method()).toBe('POST');
      expect(await request.postDataJSON()).toMatchObject({
        email: guruUser.email,
        password: 'password-guru-e2e',
      });

      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          user: guruUser,
          tokens: {
            access_token: 'access-token-guru-e2e',
            refresh_token: 'refresh-token-guru-e2e',
          },
        }),
      });
    });

    await page.goto('/login');
    await page.getByLabel('Email').fill(guruUser.email);
    await page.getByLabel('Password').fill('password-guru-e2e');
    await page.getByRole('button', { name: 'Masuk' }).click();

    await expect(page).toHaveURL(/\/guru$/);
    await expect(page.getByRole('heading', { name: 'Dashboard' })).toBeVisible();
    await expect(page.getByText('Guru E2E')).toBeVisible();
  });

  test('shows guru dashboard pending counters', async ({ page }) => {
    await page.route('**/api/v1/auth/login', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          user: guruUser,
          tokens: {
            access_token: 'access-token-guru-e2e',
            refresh_token: 'refresh-token-guru-e2e',
          },
        }),
      });
    });

    await page.route('**/api/v1/guru/pending-counts', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          ungraded_submissions: 5,
          pending_review_ulangan: 12,
          pending_review_ujian: 8,
        }),
      });
    });

    await page.goto('/login');
    await page.getByLabel('Email').fill(guruUser.email);
    await page.getByLabel('Password').fill('password-guru-e2e');
    await page.getByRole('button', { name: 'Masuk' }).click();

    await expect(page.getByText('5 Tugas Pending')).toBeVisible();
    await expect(page.getByText('12 Ulangan Pending')).toBeVisible();
    await expect(page.getByText('8 Ujian Pending')).toBeVisible();
  });
});