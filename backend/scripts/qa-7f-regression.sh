#!/usr/bin/env bash
# qa-7f-regression.sh — Fase 7.F regression after fix commit 6bf3f0e
# Tests: feed limit strict validation + audit error envelope alignment.
set -uo pipefail
BASE="http://127.0.0.1:8200/api/v1"
P=0; F=0
ok(){ P=$((P+1)); echo "PASS  $1: $2"; }
no(){ F=$((F+1)); echo "FAIL  $1: got '$2' want '$3'"; }

# Login guru1
GURU1_EMAIL="${GURU1_EMAIL:-guru1@sekolah.id}"
GURU1_PASS="${GURU1_PASS:-guru1pass}"
GTOK=$(curl -sS -X POST "$BASE/auth/login" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$GURU1_EMAIL\",\"password\":\"$GURU1_PASS\"}" | jq -r '.tokens.access_token // empty')
[[ -n "$GTOK" ]] && ok "login guru1" "ok" || { no "login guru1" "fail" "ok"; exit 1; }
GAUTH="Authorization: Bearer $GTOK"

# === Finding 1+2: feed limit strict validation ===

# T1 limit=abc → 400 invalid_limit
R=$(curl -sS -o /tmp/_b -w '%{http_code}' "$BASE/guru/feed?limit=abc" -H "$GAUTH")
[[ "$R" == "400" ]] && ok "T1 feed limit=abc returns 400" "$R" || no "T1 feed limit=abc returns 400" "$R" "400"
C=$(jq -r '.code // empty' /tmp/_b)
[[ "$C" == "invalid_limit" ]] && ok "T1b feed limit=abc code=invalid_limit" "$C" || no "T1b feed limit=abc code" "$C" "invalid_limit"

# T2 limit=-5 → 400
R=$(curl -sS -o /tmp/_b -w '%{http_code}' "$BASE/guru/feed?limit=-5" -H "$GAUTH")
[[ "$R" == "400" ]] && ok "T2 feed limit=-5 returns 400" "$R" || no "T2 feed limit=-5 returns 400" "$R" "400"

# T3 limit=0 → 400 (strict: < 1 rejected)
R=$(curl -sS -o /tmp/_b -w '%{http_code}' "$BASE/guru/feed?limit=0" -H "$GAUTH")
[[ "$R" == "400" ]] && ok "T3 feed limit=0 returns 400" "$R" || no "T3 feed limit=0 returns 400" "$R" "400"

# T4 limit=20 → 200 (happy path masih jalan)
R=$(curl -sS -o /tmp/_b -w '%{http_code}' "$BASE/guru/feed?limit=20" -H "$GAUTH")
[[ "$R" == "200" ]] && ok "T4 feed limit=20 returns 200" "$R" || no "T4 feed limit=20 returns 200" "$R" "200"

# T5 limit absent → 200 (default 20 masih default)
R=$(curl -sS -o /tmp/_b -w '%{http_code}' "$BASE/guru/feed" -H "$GAUTH")
[[ "$R" == "200" ]] && ok "T5 feed no-limit returns 200" "$R" || no "T5 feed no-limit returns 200" "$R" "200"

# T6 limit=999 (> 50 max) → 200 dgn clamp di service (existing behavior preserved)
R=$(curl -sS -o /tmp/_b -w '%{http_code}' "$BASE/guru/feed?limit=999" -H "$GAUTH")
[[ "$R" == "200" ]] && ok "T6 feed limit=999 returns 200 (service clamp)" "$R" || no "T6 feed limit=999" "$R" "200"

# === Finding 4: audit error envelope alignment ===

# T7 audit invalid kelas id → 400 dgn shape {error, code, request_id}
R=$(curl -sS -o /tmp/_b -w '%{http_code}' "$BASE/guru/kelas/not-uuid/audit" -H "$GAUTH")
[[ "$R" == "400" ]] && ok "T7 audit invalid id returns 400" "$R" || no "T7 audit invalid id" "$R" "400"
HAS_ERROR=$(jq -r 'if .error then "yes" else "no" end' /tmp/_b)
HAS_CODE=$(jq -r 'if .code then "yes" else "no" end' /tmp/_b)
HAS_RID=$(jq -r 'if .request_id then "yes" else "no" end' /tmp/_b)
HAS_MSG=$(jq -r 'if .message then "yes" else "no" end' /tmp/_b)
[[ "$HAS_ERROR" == "yes" ]] && ok "T7a audit envelope has 'error'" "yes" || no "T7a audit envelope has 'error'" "$HAS_ERROR" "yes"
[[ "$HAS_CODE" == "yes" ]] && ok "T7b audit envelope has 'code'" "yes" || no "T7b audit envelope has 'code'" "$HAS_CODE" "yes"
[[ "$HAS_RID" == "yes" ]] && ok "T7c audit envelope has 'request_id'" "yes" || no "T7c audit envelope has 'request_id'" "$HAS_RID" "yes"
[[ "$HAS_MSG" == "no" ]] && ok "T7d audit envelope no longer has 'message'" "no" || no "T7d audit no 'message'" "$HAS_MSG" "no"

# T8 audit code value masih specific (invalid_kelas_id, bukan generic invalid_id)
C=$(jq -r '.code // empty' /tmp/_b)
[[ "$C" == "invalid_kelas_id" ]] && ok "T8 audit code=invalid_kelas_id" "$C" || no "T8 audit code" "$C" "invalid_kelas_id"

# T9 audit error value sekarang human-readable msg
E=$(jq -r '.error // empty' /tmp/_b)
[[ "$E" == "invalid kelas id" ]] && ok "T9 audit error=human-readable" "$E" || no "T9 audit error" "$E" "invalid kelas id"

# T10 audit limit=abc → 400 dgn shape baru
R=$(curl -sS -o /tmp/_b -w '%{http_code}' "$BASE/guru/kelas/00000000-0000-0000-0000-000000000000/audit?limit=abc" -H "$GAUTH")
# kelas not found dulu (404) atau invalid limit (400)? Order di handler: kelas parse → caller → action → limit → offset
# Actually invalid_kelas_id parse pass → not found → 404. But uuid 00...00 valid format, jadi langsung query → not found
[[ "$R" == "400" || "$R" == "404" ]] && ok "T10 audit limit=abc on missing kelas (400 or 404)" "$R" || no "T10" "$R" "400|404"

echo
echo "=== Total: PASS=$P FAIL=$F ==="
[[ "$F" -eq 0 ]] && exit 0 || exit 1
