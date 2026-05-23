import { expect, type Page, test } from '@playwright/test';

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

const guruKelas = {
  id: '33333333-3333-4333-8333-333333333333',
  nama: 'Kelas E2E Guru',
  deskripsi: 'Kelas mocked untuk smoke guru',
  kode_invite: 'E2EGURU',
  guru_id: guruUser.id,
  bobot_soal_ulangan: 60,
  bobot_tugas: 40,
  version: 1,
  archived_at: null,
  created_at: '2026-05-23T00:00:00Z',
  updated_at: '2026-05-23T00:00:00Z',
};

const kelasResponse = {
  items: [guruKelas],
  page: 1,
  page_size: 3,
  total: 1,
  total_pages: 1,
};

const pendingCounts = {
  ungraded_submissions: 5,
  pending_review_ulangan: 12,
  pending_review_ujian: 8,
};

const feedResponse = {
  events: [
    {
      id: 'feed-1',
      kind: 'submission_baru',
      at: '2026-05-23T00:00:00Z',
      kelas_id: guruKelas.id,
      kelas_nama: guruKelas.nama,
      siswa_id: '44444444-4444-4444-8444-444444444444',
      siswa_nama: 'Siswa E2E',
      tugas_id: '55555555-5555-4555-8555-555555555555',
      tugas_judul: 'Tugas Mocked',
      is_late: false,
    },
  ],
  next_cursor: '',
};

async function mockGuruDashboardApis(page: Page) {
  await page.route('**/api/v1/kelas?*', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(kelasResponse),
    });
  });

  await page.route('**/api/v1/guru/pending-counts', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(pendingCounts),
    });
  });

  await page.route('**/api/v1/guru/feed?*', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(feedResponse),
    });
  });
}

async function mockGuruLogin(page: Page) {
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
}

test.describe('guru login smoke', () => {
  test.beforeEach(async ({ context, page }) => {
    await context.clearCookies();
    await page.addInitScript(() => window.localStorage.clear());
  });

  test('validates form before login', async ({ page }) => {
    await page.goto('/login');
    await page.waitForLoadState('networkidle');
    await expect(page.getByLabel('Email')).toBeVisible();
    await expect(page.getByLabel('Password')).toBeVisible();

    await page.getByRole('button', { name: 'Masuk' }).click();

    await expect(page.getByText('Email wajib diisi.')).toBeVisible();
    await expect(page.getByText('Password wajib diisi.')).toBeVisible();
  });

  test('logs in as guru and renders dashboard shell', async ({ page }) => {
    await mockGuruLogin(page);
    await mockGuruDashboardApis(page);

    await page.goto('/login');
    await page.waitForLoadState('networkidle');
    await page.getByLabel('Email').fill(guruUser.email);
    await page.getByLabel('Password').fill('password-guru-e2e');
    await page.waitForTimeout(500);
    await page.getByRole('button', { name: 'Masuk' }).click();

    await expect(page).toHaveURL(/\/guru$/);
    await expect(page.getByRole('heading', { name: /Halo, Guru E2E!/ })).toBeVisible();
    await expect(page.getByText('Total Kelas Aktif')).toBeVisible();
    await expect(page.locator('p').filter({ hasText: /^Kelas E2E Guru$/ })).toBeVisible();
  });

  test('renders guru dashboard counters and feed from mocked APIs', async ({ page }) => {
    await mockGuruLogin(page);
    await mockGuruDashboardApis(page);

    await page.goto('/login');
    await page.waitForLoadState('networkidle');
    await page.getByLabel('Email').fill(guruUser.email);
    await page.getByLabel('Password').fill('password-guru-e2e');
    await page.waitForTimeout(500);
    await page.getByRole('button', { name: 'Masuk' }).click();

    await expect(page.getByText('Tugas perlu dinilai')).toBeVisible();
    await expect(page.getByText('Review ulangan bab')).toBeVisible();
    await expect(page.getByText('Review ujian')).toBeVisible();
    await expect(page.locator('div').filter({ hasText: /Tugas perlu dinilai/ }).getByText('5', { exact: true })).toBeVisible();
    await expect(page.locator('div').filter({ hasText: /Review ulangan bab/ }).getByText('12', { exact: true })).toBeVisible();
    await expect(page.locator('div').filter({ hasText: /Review ujian/ }).getByText('8', { exact: true })).toBeVisible();
    await expect(page.getByText('Siswa E2E')).toBeVisible();
    await expect(page.getByText('Tugas Mocked')).toBeVisible();
  });
});
