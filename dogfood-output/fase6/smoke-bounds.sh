#!/usr/bin/env bash
# Fase 6 boundary regression smoke — guru1-driven (no admin needed).
# Uses known dev seed: guru1@sekolah.id / guru1pass
#
# Asserts (post-UX/QA-fix `<COMMIT>`):
#   T1. Ujian durasi_menit BE cap = 300 (matches DB CHECK).
#   T2. Ujian random.jumlah_soal BE cap = 200.
#   T3. BankSoal poin BE cap = 100.
#   T4. Image upload error code = "payload_too_large" (FE banksoal-api maps this).
set -uo pipefail

BASE="${BASE:-http://127.0.0.1:8200/api/v1}"
GURU_EMAIL="${GURU_EMAIL:-guru1@sekolah.id}"
GURU_PASS="${GURU_PASS:-guru1pass}"

PASS=0
FAIL=0

expect() {
  local label="$1"; local got="$2"; local want="$3"
  if [[ "$got" == "$want" ]]; then
    printf "PASS  %-65s  got=%s\n" "$label" "$got"
    PASS=$((PASS+1))
  else
    printf "FAIL  %-65s  got=%s want=%s\n" "$label" "$got" "$want"
    FAIL=$((FAIL+1))
  fi
}

login() {
  curl -sS -X POST "$BASE/auth/login" -H 'Content-Type: application/json' \
    -d "{\"email\":\"$1\",\"password\":\"$2\"}" \
  | jq -r '.tokens.access_token // empty'
}

codeof() { jq -r '.error.code // .code // "<no-code>"'; }

echo "=== Smoke: Fase 6 boundary regression ==="

GTOK=$(login "$GURU_EMAIL" "$GURU_PASS")
if [[ -z "$GTOK" ]]; then
  echo "FATAL: guru login fail"
  exit 2
fi
GAUTH="Authorization: Bearer $GTOK"

# Find a kelas owned by guru1
KELAS_ID=$(curl -sS "$BASE/kelas?limit=20" -H "$GAUTH" | jq -r '.items[0].id // empty')
if [[ -z "$KELAS_ID" ]]; then
  echo "FATAL: guru has no kelas"
  exit 2
fi
echo "kelas=$KELAS_ID"
echo ""

# === T1: Ujian durasi_menit boundary (DB CHECK = 1..300, BE Go = 1..600) ===
mkU() {
  local d="$1"
  cat <<JSON
{"judul":"smoke-d-$d","deskripsi":"","durasi_menit":$d,"source":{"mode":"random","filter":{},"jumlah_soal":1},"izinkan_review_setelah_submit":true,"status":"draft"}
JSON
}
R300=$(curl -sS -o /tmp/u300.json -w '%{http_code}' -X POST "$BASE/kelas/$KELAS_ID/ujian" \
  -H "$GAUTH" -H 'Content-Type: application/json' --data "$(mkU 300)")
expect "T1a Create durasi=300 (DB CHECK cap = accept)" "$R300" "201"

R301=$(curl -sS -o /tmp/u301.json -w '%{http_code}' -X POST "$BASE/kelas/$KELAS_ID/ujian" \
  -H "$GAUTH" -H 'Content-Type: application/json' --data "$(mkU 301)")
expect "T1b Create durasi=301 (BE Go now caps at 300 → 400 invalid_body)" "$R301" "400"

R601=$(curl -sS -o /tmp/u601.json -w '%{http_code}' -X POST "$BASE/kelas/$KELAS_ID/ujian" \
  -H "$GAUTH" -H 'Content-Type: application/json' --data "$(mkU 601)")
expect "T1c Create durasi=601 (still 400)" "$R601" "400"

R361=$(curl -sS -o /tmp/u361.json -w '%{http_code}' -X POST "$BASE/kelas/$KELAS_ID/ujian" \
  -H "$GAUTH" -H 'Content-Type: application/json' --data "$(mkU 361)")
expect "T1d Create durasi=361 (legacy FE max no longer accepted by BE)" "$R361" "400"

# Cleanup
for f in /tmp/u300.json; do
  ID=$(jq -r '.ujian.id // .id // empty' "$f" 2>/dev/null)
  V=$(jq -r '.ujian.version // .version // 1' "$f" 2>/dev/null)
  if [[ -n "$ID" ]]; then
    curl -sS -X DELETE "$BASE/ujian/$ID?version=$V" -H "$GAUTH" > /dev/null
  fi
done

# === T2: jumlah_soal boundary ===
P201=$(cat <<JSON
{"judul":"smoke-js-201","deskripsi":"","durasi_menit":60,"source":{"mode":"random","filter":{},"jumlah_soal":201},"izinkan_review_setelah_submit":true,"status":"draft"}
JSON
)
R201=$(curl -sS -o /tmp/js201.json -w '%{http_code}' -X POST "$BASE/kelas/$KELAS_ID/ujian" \
  -H "$GAUTH" -H 'Content-Type: application/json' --data "$P201")
expect "T2 jumlah_soal=201 (over BE cap → 400)" "$R201" "400"

# === T3: BankSoal poin boundary ===
mkS() {
  local p="$1"
  cat <<JSON
{"mapel":"smoke","tingkat":"X","topik":"poin","pertanyaan":"P","opsi_a":"A","opsi_b":"B","opsi_c":"C","opsi_d":"D","opsi_e":"E","jawaban":"a","poin":$p}
JSON
}
R100=$(curl -sS -o /tmp/p100.json -w '%{http_code}' -X POST "$BASE/bank-soal" \
  -H "$GAUTH" -H 'Content-Type: application/json' --data "$(mkS 100)")
expect "T3a BankSoal poin=100 (BE cap)" "$R100" "201"

R101=$(curl -sS -o /tmp/p101.json -w '%{http_code}' -X POST "$BASE/bank-soal" \
  -H "$GAUTH" -H 'Content-Type: application/json' --data "$(mkS 101)")
expect "T3b BankSoal poin=101 (over → 400)" "$R101" "400"

# === T4: Image upload too-large error code ===
RS=$(curl -sS -o /tmp/p_for_img.json -w '%{http_code}' -X POST "$BASE/bank-soal" \
  -H "$GAUTH" -H 'Content-Type: application/json' --data "$(mkS 1)")
SID2=$(jq -r '.soal.id // .id // empty' /tmp/p_for_img.json 2>/dev/null)
SVER=$(jq -r '.soal.version // .version // 1' /tmp/p_for_img.json 2>/dev/null)
if [[ "$RS" == "201" && -n "$SID2" ]]; then
  dd if=/dev/zero of=/tmp/big.jpg bs=1M count=6 2>/dev/null
  IMG_RESP=$(curl -sS -o /tmp/img_err.json -w '%{http_code}' -X POST \
    "$BASE/bank-soal/$SID2/image?slot=pertanyaan" \
    -H "$GAUTH" \
    -F "file=@/tmp/big.jpg;type=image/jpeg")
  expect "T4a Image >5MB returns 413" "$IMG_RESP" "413"
  CODE=$(jq -r '.code // "<no-code>"' /tmp/img_err.json 2>/dev/null)
  expect "T4b Image too-large code=payload_too_large (FE drift: maps 'image_too_large')" "$CODE" "payload_too_large"
  curl -sS -X DELETE "$BASE/bank-soal/$SID2?version=$SVER" -H "$GAUTH" > /dev/null
  rm -f /tmp/big.jpg
else
  echo "skip: T4 image upload (no soal created, status=$RS)"
fi

# Cleanup
SID=$(jq -r '.soal.id // .id // empty' /tmp/p100.json 2>/dev/null)
SV=$(jq -r '.soal.version // .version // 1' /tmp/p100.json 2>/dev/null)
if [[ -n "$SID" ]]; then
  curl -sS -X DELETE "$BASE/bank-soal/$SID?version=$SV" -H "$GAUTH" > /dev/null
fi

echo ""
echo "=== Result: $PASS pass, $FAIL fail ==="
exit $FAIL
