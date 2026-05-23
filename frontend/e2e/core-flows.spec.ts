import { expect, test } from '@playwright/test';

const fakeAdminUser = {
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

const fakeGuruUser = {
  id: '22222222-2222-4222-8222-222222222222',
  name: 'Guru E2E',
  email: 'guru.e2e@sekolah.id',
  role: 'guru',
  status: 'active',
  must_change_password: false,
  last_login_at: null,
  created_at: '2026-05-23T00:00:00Z',
  updated_at: '2026-05-23T00:00:00Z',
  kelas: [
    {
      id: 'K123',
      name: 'Kelas 10 IPA 1',
      grade: 10,
      major: 'IPA',
    },
  ],
};

test.describe('Admin create user', () => {
  test.use({ storageState: { cookies: [], origins: [] } });

  test('admin login and create user with must_change_password=true', async ({ page }) => {
    // Mock login sebagai admin
    await page.route('**/api/v1/auth/login', async (route) => {
      const request = route.request();
      expect(request.method()).toBe('POST');
      expect(await request.postDataJSON()).toMatchObject({
        email: fakeAdminUser.email,
        password: 'password-e2e',
      });

      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          user: fakeAdminUser,
          tokens: {
            access_token: 'admin-token',
            refresh_token: 'admin-refresh',
          },
        }),
      });
    });

    // Login
    await page.goto('/');
    await page.getByRole('link', { name: 'Masuk' }).click();
    await page.getByLabel('Email').fill(fakeAdminUser.email);
    await page.getByLabel('Password').fill('password-e2e');
    await page.getByRole('button', { name: 'Masuk' }).click();
    await expect(page).toHaveURL(/\/admin$/);

    // Navigate to users page
    await page.getByRole('link', { name: 'Pengguna' }).click();
    await expect(page.getByRole('heading', { name: 'Pengguna' })).toBeVisible();

    // Mock create user API
    const newUserEmail = 'user.new@sekolah.id';
    const newUserResponse = {
      user: {
        id: '33333333-3333-4333-8333-333333333333',
        name: 'User Baru',
        email: newUserEmail,
        role: 'siswa',
        status: 'active',
        must_change_password: true,
        last_login_at: null,
        created_at: '2026-05-23T12:00:00Z',
        updated_at: '2026-05-23T12:00:00Z',
      },
    };

    await page.route('**/api/v1/admin/pengguna', async (route) => {
      const request = route.request();
      expect(request.method()).toBe('POST');
      const body = JSON.parse(await request.postText());
      expect(body.email).toBe(newUserEmail);
      expect(body.status).toBe('active');
      expect(body.must_change_password).toBe(true);

      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(newUserResponse),
      });
    });

    // Click tombol create user
    await page.getByRole('button', { name: 'Tambah pengguna' }).click();
    await page.getByLabel('Email').fill(newUserEmail);
    await page.getByLabel('Status').selectOption('active');
    await page.getByLabel('Wajib ganti password').check();
    await page.getByRole('button', { name: 'Simpan' }).click();

    // Verify success message
    await expect(page.getByText('User berhasil dibuat')).toBeVisible();
    await expect(page.getByText(newUserEmail)).toBeVisible();
  });
});

test.describe('Guru login and access kelas page', () => {
  test.use({ storageState: { cookies: [], origins: [] } });

  test('guru login and see kelas list', async ({ page }) => {
    // Mock login sebagai guru
    await page.route('**/api/v1/auth/login', async (route) => {
      const request = route.request();
      expect(request.method()).toBe('POST');
      expect(await request.postDataJSON()).toMatchObject({
        email: fakeGuruUser.email,
        password: 'password-e2e',
      });

      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          user: fakeGuruUser,
          tokens: {
            access_token: 'guru-token',
            refresh_token: 'guru-refresh',
          },
        }),
      });
    });

    // Login
    await page.goto('/');
    await page.getByRole('link', { name: 'Masuk' }).click();
    await page.getByLabel('Email').fill(fakeGuruUser.email);
    await page.getByLabel('Password').fill('password-e2e');
    await page.getByRole('button', { name: 'Masuk' }).click();
    await expect(page).toHaveURL(/\/guru$/);

    // Verify guru dashboard
    await expect(page.getByRole('heading', { name: 'Dashboard' })).toBeVisible();
    await expect(page.getByText('Selamat datang, Guru E2E')).toBeVisible();

    // Navigate to kelas page
    await page.getByRole('link', { name: 'Kelas' }).click();
    await expect(page.getByRole('heading', { name: 'Kelas' })).toBeVisible();

    // Verify kelas list shows expected kelas
    await expect(page.getByText('Kelas 10 IPA 1')).toBeVisible();
    await expect(page.getByText('10')).toBeVisible(); // grade
    await expect(page.getByText('IPA')).toBeVisible(); // major
  });
});

test.describe('Guru access task and exam pages', () => {
  test.use({ storageState: { cookies: [], origins: [] } });

  test('guru can navigate to tugas and ulangan pages', async ({ page }) => {
    // Mock login sebagai guru
    await page.route('**/api/v1/auth/login', async (route) => {
      const request = route.request();
      expect(request.method()).toBe('POST');
      expect(await request.postDataJSON()).toMatchObject({
        email: fakeGuruUser.email,
        password: 'password-e2e',
      });

      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          user: fakeGuruUser,
          tokens: {
            access_token: 'guru-token',
            refresh_token: 'guru-refresh',
          },
        }),
      });
    });

    // Login
    await page.goto('/');
    await page.getByRole('link', { name: 'Masuk' }).click();
    await page.getByLabel('Email').fill(fakeGuruUser.email);
    await page.getByLabel('Password').fill('password-e2e');
    await page.getByRole('button', { name: 'Masuk' }).click();
    await expect(page).toHaveURL(/\/guru$/);

    // Navigate to kelas detail (mock)
    await page.route('**/api/v1/guru/kelas/:id', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          kelas: fakeGuruUser.kelas[0],
          active_bab: [],
          pending_counts: {
            submission: 0,
            review_ulangan: 0,
            review_ujian: 0,
          },
        }),
      });
    });

    // Mock menu klik untuk kelas detail
    await page.route('**/api/v1/guru/me/kelas', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          kelas_list: fakeGuruUser.kelas,
        }),
      });
    });

    // Simulasi klik menu Kelas di sidebar (pakai data-testid atau role)
    await page.getByRole('menuitem', { name: 'Kelas' }).click();
    await expect(page.getByRole('heading', { name: 'Kelas' })).toBeVisible();

    // Klik pada kelas untuk buka detail
    await page.getByRole('link', { name: 'Kelas 10 IPA 1' }).click();
    await expect(page.getByRole('heading', { name: 'Kelas 10 IPA 1' })).toBeVisible();

    // Verify menu navigasi ada
    await expect(page.getByRole('link', { name: 'Tugas' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Ulangan Harian' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Bab & Materi' })).toBeVisible();
  });
});
