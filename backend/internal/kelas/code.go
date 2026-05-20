// Kode invite generator untuk kelas.
//
// Charset sengaja menghindari karakter ambigu (O/0, I/1, L/l, B/8) supaya
// guru/siswa gak salah baca-tulis manual. Generator pakai crypto/rand untuk
// alnum 6-char, lalu cek tabrakan ke repository: kalau sudah ada → retry,
// max 10 percobaan. Kalau setelah 10 retry masih tabrakan, return error
// (kemungkinan keyspace habis — sangat tidak mungkin di skala MVP).
package kelas

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"

	"gorm.io/gorm"
)

// KodeInviteCharset adalah karakter yang dipakai generator. Sengaja skip
// 0/O, 1/I/L, 8/B karena ambigu di tulisan tangan.
const KodeInviteCharset = "ACDEFGHJKMNPQRTUVWXYZ23456789"

// KodeInviteLength adalah panjang kode invite (6 char per spec).
const KodeInviteLength = 6

// MaxKodeInviteRetry adalah batas retry kalau collision.
const MaxKodeInviteRetry = 10

// ErrKodeInviteCollision dikembalikan kalau setelah MaxKodeInviteRetry attempt
// masih nabrak kode existing.
var ErrKodeInviteCollision = errors.New("kelas: failed to generate unique kode invite after max retries")

// kodeInviteFinder adalah subset dari Repo yang dipakai generator. Pakai
// interface kecil supaya bisa di-mock di test tanpa bawa GORM stack.
type kodeInviteFinder interface {
	FindByKodeInvite(ctx context.Context, kode string) (*Kelas, error)
}

// GenerateKodeInvite mengembalikan kode invite 6-char yang dijamin unik
// terhadap repo. Pakai crypto/rand supaya tidak bisa ditebak.
//
// Strategi: generate → cek repo → kalau gorm.ErrRecordNotFound berarti unik
// (return). Kalau ketemu → retry. Error lain langsung dikembalikan.
func GenerateKodeInvite(ctx context.Context, repo kodeInviteFinder) (string, error) {
	for i := 0; i < MaxKodeInviteRetry; i++ {
		kode, err := randomKode(KodeInviteLength)
		if err != nil {
			return "", fmt.Errorf("kelas: random kode: %w", err)
		}

		_, err = repo.FindByKodeInvite(ctx, kode)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return kode, nil
		}
		if err != nil {
			return "", fmt.Errorf("kelas: lookup kode: %w", err)
		}
		// Collision: row found, retry.
	}
	return "", ErrKodeInviteCollision
}

// randomKode mengembalikan string n karakter dari KodeInviteCharset
// menggunakan crypto/rand sebagai sumber.
func randomKode(n int) (string, error) {
	if n <= 0 {
		return "", errors.New("kelas: kode length must be positive")
	}
	max := big.NewInt(int64(len(KodeInviteCharset)))
	buf := make([]byte, n)
	for i := 0; i < n; i++ {
		idx, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		buf[i] = KodeInviteCharset[idx.Int64()]
	}
	return string(buf), nil
}
