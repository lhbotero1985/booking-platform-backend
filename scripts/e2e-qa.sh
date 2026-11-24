#!/usr/bin/env bash
# End-to-end check of the Phase 2 flows against a deployed environment (T3.7). Synthetic data
# only: creates a throwaway user qa-e2e-<random>@example.test.
#
#   scripts/e2e-qa.sh BASE_URL MAILPIT_URL
#   BASE_URL     public API, e.g. https://booking-platform-col.duckdns.org
#   MAILPIT_URL  Mailpit API through an SSH tunnel, e.g. http://127.0.0.1:18025
#                (ssh -N -L 18025:127.0.0.1:8025 deploy@HOST)
#   CURL_EXTRA   optional extra curl options (e.g. --resolve host:443:IP when local DNS fails)
#   SKIP_RATE_LIMIT=1 skips the rate-limit check (it blocks auth endpoints for this IP ~1 min)
#   ADMIN_EMAIL / ADMIN_PASSWORD (optional) run the business onboarding checks as PLATFORM_ADMIN;
#                each run leaves one SUSPENDED business "QA E2E <timestamp>" behind
# Each run also leaves one self-service business "QA E2E propia <timestamp>" whose 7-day demo
# then expires on its own.
#
# Exit status: number of failed checks (0 = all green).
set -uo pipefail

B=${1:?BASE_URL}
M=${2:?MAILPIT_URL}
read -r -a X <<<"${CURL_EXTRA:-}"
J='Content-Type: application/json'
EMAIL="qa-e2e-$(date +%s)-$RANDOM@example.test"
PW1="primera clave de prueba e2e"
PW2="segunda clave de prueba e2e"
PW3="tercera clave de prueba e2e"
fails=0

check() { # name expected(regex) got
  if [[ $3 =~ ^($2)$ ]]; then
    printf 'PASS  %-55s %s\n' "$1" "$3"
  else
    printf 'FAIL  %-55s got %s, want %s\n' "$1" "$3" "$2"
    fails=$((fails + 1))
  fi
}

# status METHOD PATH [JSON] [TOKEN] -> HTTP status. Body and headers go to files because callers
# run it inside $(...) (a subshell); read them with json / hdrs / token.
status() {
  local args=(-s "${X[@]}" -X "$1" "$B$2" -D "$tmp/h" -o "$tmp/b" -w '%{http_code}')
  [[ -n ${3:-} ]] && args+=(-H "$J" -d "$3")
  [[ -n ${4:-} ]] && args+=(-H "Authorization: Bearer $4")
  local code wait
  for _ in 1 2 3 4 5 6; do
    code=$(curl "${args[@]}")
    # The auth endpoints allow 10 requests/min per IP: honour Retry-After instead of failing
    # (except in the rate-limit check itself, NO_RETRY=1).
    [[ $code == 429 && ${NO_RETRY:-0} != 1 ]] || break
    wait=$(tr -d '\r' <"$tmp/h" | sed -n 's/^[Rr]etry-[Aa]fter: *//p')
    sleep "$((${wait:-10} + 1))"
  done
  echo "$code"
}

json() { jq -r "$1" "$tmp/b" 2>/dev/null; }
hdrs() { tr -d '\r' <"$tmp/h"; }
token() { json '.token // empty'; }

login() { # email password -> status (read the token with token right after)
  status POST /api/v1/auth/login "{\"email\":\"$1\",\"password\":\"$2\",\"clientType\":\"MOBILE\"}"
}

# mail_link PATH_FRAGMENT -> newest link in the user's mailbox containing it (waits up to 30 s)
mail_link() {
  local id link
  for _ in $(seq 1 30); do
    for id in $(curl -s "$M/api/v1/search?query=to:$EMAIL" | jq -r '.messages[].ID'); do
      link=$(curl -s "$M/api/v1/message/$id" | jq -r '.Text' |
        grep -o "https://[^ ]*$1?token=[A-Za-z0-9_%.-]*" | head -1)
      [[ -n $link ]] && { echo "$link"; return; }
    done
    sleep 1
  done
}

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
echo "user: $EMAIL"

# --- platform ----------------------------------------------------------------------------
check "GET /healthz" 200 "$(status GET /healthz)"
echo "      version $(json .version)"
check "GET /readyz" 200 "$(status GET /readyz)"
check "HSTS header" 1 "$(grep -ci '^strict-transport-security: max-age=31536000' <<<"$(hdrs)")"
check "no Server header" 0 "$(grep -ci '^server:' <<<"$(hdrs)")"
check "X-Request-ID header" 1 "$(grep -ci '^x-request-id:' <<<"$(hdrs)")"
check "unknown route -> problem+json 404" 404 "$(status GET /api/v1/does-not-exist)"
check "  content type application/problem+json" 1 "$(grep -ci '^content-type: application/problem+json' <<<"$(hdrs)")"

# --- registration and email verification ---------------------------------------------------
check "register" 202 "$(status POST /api/v1/auth/register "{\"email\":\"$EMAIL\",\"password\":\"$PW1\",\"fullName\":\"QA E2E\"}")"
check "register again (no enumeration)" 202 "$(status POST /api/v1/auth/register "{\"email\":\"$EMAIL\",\"password\":\"$PW1\",\"fullName\":\"QA E2E\"}")"
check "register weak password" '400|422' "$(status POST /api/v1/auth/register "{\"email\":\"x$EMAIL\",\"password\":\"password\",\"fullName\":\"QA\"}")"
check "register unknown field" '400|422' "$(status POST /api/v1/auth/register "{\"email\":\"y$EMAIL\",\"password\":\"$PW1\",\"fullName\":\"QA\",\"role\":\"PLATFORM_ADMIN\"}")"
check "login before verification" '401|403' "$(login "$EMAIL" "$PW1")"
link=$(mail_link verify-email)
check "verification email received" 1 "$([[ -n $link ]] && echo 1 || echo 0)"
check "  link points to PUBLIC_APP_URL" 1 "$([[ $link == "$B"/verify-email* ]] && echo 1 || echo 0)"
vtok=${link##*token=}
check "verify email" 200 "$(status POST /api/v1/auth/verify-email "{\"token\":\"$vtok\"}")"
check "verify email token reuse" '400|401|404|410|422' "$(status POST /api/v1/auth/verify-email "{\"token\":\"$vtok\"}")"
check "resend verification (verified: still 202)" 202 "$(status POST /api/v1/auth/verify-email/resend "{\"email\":\"$EMAIL\"}")"

# --- login, session, me ------------------------------------------------------------------
check "login wrong password" 401 "$(login "$EMAIL" "not the password at all")"
check "login unknown email (same answer)" 401 "$(login "nobody-$EMAIL" "$PW1")"
check "login" 200 "$(login "$EMAIL" "$PW1")"
T1=$(token)
check "  opaque session token bp_s_" 1 "$([[ $T1 == bp_s_* ]] && echo 1 || echo 0)"
check "GET /me" 200 "$(status GET /api/v1/me "" "$T1")"
check "  /me email" "$EMAIL" "$(json .email)"
check "  /me no password hash" 0 "$(grep -ci 'hash\|argon' "$tmp/b")"
check "GET /me without token" 401 "$(status GET /api/v1/me)"
check "GET /me forged token" 401 "$(status GET /api/v1/me "" "bp_s_forged-token-value")"
check "GET /me/memberships" 200 "$(status GET /api/v1/me/memberships "" "$T1")"
check "  no memberships yet" 0 "$(json '.items | length')"

# --- geography (Phase 4): public reference data, writes only for PLATFORM_ADMIN ------------
check "GET /countries" 200 "$(status GET /api/v1/countries)"
check "  Colombia COP +57" "CO COP \+57" "$(json '.items[] | select(.code=="CO") | "\(.code) \(.defaultCurrency) \(.phoneCode)"')"
check "GET /countries/CO/divisions" 200 "$(status GET /api/v1/countries/CO/divisions)"
check "  Antioquia, Valle del Cauca" "CO-ANT,CO-VAC" "$(json '[.items[].code] | join(",")')"
valle=$(json '.items[] | select(.code=="CO-VAC") | .id')
antioquia=$(json '.items[] | select(.code=="CO-ANT") | .id')
check "GET Valle del Cauca cities" 200 "$(status GET "/api/v1/divisions/$valle/cities")"
check "  Cali, Yumbo (America/Bogota)" "Cali,Yumbo America/Bogota" "$(json '[.items[].name] | join(",")') $(json '[.items[].timeZone] | unique | join(",")')"
check "GET Antioquia cities" 200 "$(status GET "/api/v1/divisions/$antioquia/cities")"
check "  Itagüí, Medellín" "Itagüí,Medellín" "$(json '[.items[].name] | join(",")')"
check "unknown country -> 404" 404 "$(status GET /api/v1/countries/ZZ/divisions)"
check "admin write without token" 401 "$(status POST /api/v1/admin/cities "{\"divisionId\":\"$valle\",\"name\":\"Palmira\",\"timeZone\":\"America/Bogota\"}")"
check "admin write as normal user" 403 "$(status POST /api/v1/admin/cities "{\"divisionId\":\"$valle\",\"name\":\"Palmira\",\"timeZone\":\"America/Bogota\"}" "$T1")"

# --- business onboarding (Phase 4, PLATFORM_ADMIN; optional: needs ADMIN_EMAIL/ADMIN_PASSWORD) --
cali=$(curl -s "${X[@]}" "$B/api/v1/divisions/$valle/cities" | jq -r '.items[] | select(.name=="Cali") | .id')
check "business list as normal user" 403 "$(status GET /api/v1/admin/businesses "" "$T1")"

# --- self-service onboarding (ADR-0018): ACTIVE business + 7-day demo, one per account --------
own="{\"name\":\"QA E2E propia $(date +%s)\",\"contactPhone\":\"3001234567\",\"cityId\":\"$cali\",\"addressLine\":\"Calle 1\"}"
check "self-service business" 201 "$(status POST /api/v1/businesses "$own" "$T1")"
check "  ACTIVE at once (demo)" ACTIVE "$(json .status)"
check "  one per account" 409 "$(status POST /api/v1/businesses "$own" "$T1")"
check "  code ALREADY_OWNER" ALREADY_OWNER "$(json .code)"
if [[ -n ${ADMIN_EMAIL:-} && -n ${ADMIN_PASSWORD:-} ]]; then
  check "admin login" 200 "$(login "$ADMIN_EMAIL" "$ADMIN_PASSWORD")"
  TA=$(token)
  check "create business (owner = e2e user)" 201 "$(status POST /api/v1/admin/businesses "{\"name\":\"QA E2E $(date +%s)\",\"contactPhone\":\"300 123 4567\",\"cityId\":\"$cali\",\"addressLine\":\"Calle 5 # 38-25\",\"ownerEmail\":\"$EMAIL\"}" "$TA")"
  biz=$(json .id)
  check "  starts PENDING_REVIEW, COP, America/Bogota" "PENDING_REVIEW COP America/Bogota \\+573001234567" "$(json '"\(.status) \(.currency) \(.timeZone) \(.contactPhone)"')"
  check "owner sees the membership" OWNER "$(status GET /api/v1/me/memberships "" "$T1" >/dev/null; json ".items[] | select(.businessId==\"$biz\") | .role")"
  check "activate business" ACTIVE "$(status POST "/api/v1/admin/businesses/$biz/activate" "" "$TA" >/dev/null; json .status)"
  check "suspend business (left suspended)" SUSPENDED "$(status POST "/api/v1/admin/businesses/$biz/suspend" "" "$TA" >/dev/null; json .status)"
  check "unverified owner rejected" 400 "$(status POST /api/v1/admin/businesses "{\"name\":\"QA E2E bad\",\"contactPhone\":\"3001234567\",\"cityId\":\"$cali\",\"addressLine\":\"Calle 1\",\"ownerEmail\":\"nobody-$EMAIL\"}" "$TA")"
  status POST /api/v1/auth/logout "" "$TA" >/dev/null
else
  echo "SKIP  business onboarding (set ADMIN_EMAIL and ADMIN_PASSWORD)"
fi

# --- change password (current session stays, others revoked) -------------------------------
login "$EMAIL" "$PW1" >/dev/null
T2=$(token)
check "change password wrong current" '400|401|403|422' "$(status POST /api/v1/me/password "{\"currentPassword\":\"wrong wrong wrong\",\"newPassword\":\"$PW2\"}" "$T1")"
check "change password" 204 "$(status POST /api/v1/me/password "{\"currentPassword\":\"$PW1\",\"newPassword\":\"$PW2\"}" "$T1")"
check "  current session still valid" 200 "$(status GET /api/v1/me "" "$T1")"
check "  other session revoked" 401 "$(status GET /api/v1/me "" "$T2")"
check "  old password rejected" 401 "$(login "$EMAIL" "$PW1")"
check "  new password accepted" 200 "$(login "$EMAIL" "$PW2")"

# --- forgot / reset password (all sessions revoked) ----------------------------------------
check "forgot password" 202 "$(status POST /api/v1/auth/password/forgot "{\"email\":\"$EMAIL\"}")"
check "forgot password unknown email (same answer)" 202 "$(status POST /api/v1/auth/password/forgot "{\"email\":\"nobody-$EMAIL\"}")"
link=$(mail_link reset-password)
check "reset email received" 1 "$([[ -n $link ]] && echo 1 || echo 0)"
rtok=${link##*token=}
check "reset with weak password" '400|422' "$(status POST /api/v1/auth/password/reset "{\"token\":\"$rtok\",\"newPassword\":\"12345678\"}")"
check "reset password" 200 "$(status POST /api/v1/auth/password/reset "{\"token\":\"$rtok\",\"newPassword\":\"$PW3\"}")"
check "  reset token reuse" '400|401|404|410|422' "$(status POST /api/v1/auth/password/reset "{\"token\":\"$rtok\",\"newPassword\":\"$PW1\"}")"
check "  sessions revoked after reset" 401 "$(status GET /api/v1/me "" "$T1")"
check "  login with reset password" 200 "$(login "$EMAIL" "$PW3")"

# --- logout / logout-all -------------------------------------------------------------------
T3=$(token)
login "$EMAIL" "$PW3" >/dev/null
T4=$(token)
check "logout" 204 "$(status POST /api/v1/auth/logout "" "$T3")"
check "  token unusable after logout" 401 "$(status GET /api/v1/me "" "$T3")"
check "  other session still valid" 200 "$(status GET /api/v1/me "" "$T4")"
login "$EMAIL" "$PW3" >/dev/null
T5=$(token)
check "logout-all" 200 "$(status POST /api/v1/auth/logout-all "" "$T4")"
check "  revoked count >= 2" 1 "$(( $(json ".revoked // 0") >= 2 ))"
check "  all sessions gone" 401 "$(status GET /api/v1/me "" "$T5")"

# --- rate limiting (last: blocks this IP on auth endpoints ~1 min) ---------------------------
if [[ ${SKIP_RATE_LIMIT:-0} != 1 ]]; then
  got=""
  for _ in $(seq 1 15); do
    got=$(NO_RETRY=1 login "ratelimit-$EMAIL" "whatever whatever")
    [[ $got == 429 ]] && break
  done
  check "auth rate limit reached" 429 "$got"
  check "  Retry-After header" 1 "$(grep -ci '^retry-after: [0-9]' <<<"$(hdrs)")"
  check "  code RATE_LIMITED" RATE_LIMITED "$(json .code)"
fi

echo "user: $EMAIL"
echo "failed: $fails"
exit "$fails"
