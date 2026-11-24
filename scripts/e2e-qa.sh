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
  [[ -n ${5:-} ]] && args+=(-H "$5")
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

# mail_link PATH_FRAGMENT [EMAIL] -> newest link in the mailbox (default: the e2e user)
# containing it (waits up to 30 s)
mail_link() {
  local id link to=${2:-$EMAIL}
  for _ in $(seq 1 30); do
    for id in $(curl -s "$M/api/v1/search?query=to:$to" | jq -r '.messages[].ID'); do
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
check "register" 202 "$(status POST /api/v1/auth/register "{\"email\":\"$EMAIL\",\"password\":\"$PW1\",\"fullName\":\"QA E2E\",\"phone\":\"300 123 4567\"}")"
check "register again (no enumeration)" 202 "$(status POST /api/v1/auth/register "{\"email\":\"$EMAIL\",\"password\":\"$PW1\",\"fullName\":\"QA E2E\",\"phone\":\"300 123 4567\"}")"
check "register weak password" '400|422' "$(status POST /api/v1/auth/register "{\"email\":\"x$EMAIL\",\"password\":\"password\",\"fullName\":\"QA\",\"phone\":\"3001234567\"}")"
check "register without phone" 400 "$(status POST /api/v1/auth/register "{\"email\":\"z$EMAIL\",\"password\":\"$PW1\",\"fullName\":\"QA\"}")"
check "register unknown field" '400|422' "$(status POST /api/v1/auth/register "{\"email\":\"y$EMAIL\",\"password\":\"$PW1\",\"fullName\":\"QA\",\"phone\":\"3001234567\",\"role\":\"PLATFORM_ADMIN\"}")"
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
mine=$(json .id)
myslug=$(json .slug)
myname=$(json .name)

# --- public search (T4.6, T5.7): listed only once it has something to book ------------------
search_mine() { status GET "/api/v1/businesses?cityId=$cali&q=$(jq -rn --arg v "$myname" '$v|@uri')" >/dev/null; json "[.items[] | select(.id==\"$mine\")] | length"; }
check "new business not listed yet (nothing to book)" 0 "$(search_mine)"
check "public detail by slug" 200 "$(status GET "/api/v1/businesses/$myslug")"
check "unknown slug -> 404" 404 "$(status GET /api/v1/businesses/no-existe-e2e)"
check "search without city -> 400" 400 "$(status GET /api/v1/businesses)"
check "owner views the business" 200 "$(status GET "/api/v1/business/$mine" "" "$T1")"
check "  demo running" "TRIALING true" "$(json '"\(.subscription.status) \(.subscription.entitled)"')"
check "owner edits the business" 200 "$(status PATCH "/api/v1/business/$mine" '{"description":"Cortes y barba","bookingPolicy":{"slotIntervalMinutes":15}}' "$T1")"
check "  changes applied" "Cortes y barba 15" "$(json '"\(.description) \(.bookingPolicy.slotIntervalMinutes)"')"
check "another business -> 404" 404 "$(status GET /api/v1/business/01a20aae-0000-7000-8000-000000000000 "" "$T1")"

# --- members (T4.5): the OWNER (person in charge) adds a barber, changes its role, removes it ---
STAFF="staff-$EMAIL"
check "register staff account" 202 "$(status POST /api/v1/auth/register "{\"email\":\"$STAFF\",\"password\":\"$PW1\",\"fullName\":\"Barbero E2E\",\"phone\":\"3007654321\"}")"
slink=$(mail_link verify-email "$STAFF")
check "  staff verified" 200 "$(status POST /api/v1/auth/verify-email "{\"token\":\"${slink##*token=}\"}")"
check "add barber (PROFESSIONAL)" 201 "$(status POST "/api/v1/business/$mine/members" "{\"email\":\"$STAFF\",\"role\":\"PROFESSIONAL\"}" "$T1")"
staff=$(json .userId)
check "members: owner + barber" "OWNER,PROFESSIONAL" "$(status GET "/api/v1/business/$mine/members" "" "$T1" >/dev/null; json '[.items[].role] | join(",")')"
check "promote barber to OWNER" 204 "$(status PATCH "/api/v1/business/$mine/members/$staff" '{"role":"OWNER"}' "$T1")"
check "back to PROFESSIONAL" 204 "$(status PATCH "/api/v1/business/$mine/members/$staff" '{"role":"PROFESSIONAL"}' "$T1")"
check "MANAGER no longer exists" 400 "$(status PATCH "/api/v1/business/$mine/members/$staff" '{"role":"MANAGER"}' "$T1")"
check "remove member" 204 "$(status DELETE "/api/v1/business/$mine/members/$staff" "" "$T1")"
owner_id=$(status GET /api/v1/me "" "$T1" >/dev/null; json .id)
check "last owner cannot leave" 409 "$(status DELETE "/api/v1/business/$mine/members/$owner_id" "" "$T1")"
check "  code LAST_OWNER" LAST_OWNER "$(json .code)"

# --- professionals (T5.2): the OWNER adds the barber by email and itself (it also cuts hair) ---
check "add barber as professional" 201 "$(status POST "/api/v1/business/$mine/professionals" "{\"email\":\"$STAFF\",\"displayName\":\"Barbero E2E\"}" "$T1")"
check "  bookable" true "$(json .bookable)"
pbarber=$(json .id)
check "owner is also a professional" 201 "$(status POST "/api/v1/business/$mine/professionals" "{\"email\":\"$EMAIL\",\"displayName\":\"Dueño E2E\",\"sortOrder\":1}" "$T1")"
powner=$(json .id)
check "one profile per account" 409 "$(status POST "/api/v1/business/$mine/professionals" "{\"email\":\"$STAFF\",\"displayName\":\"Otra vez\"}" "$T1")"
check "list professionals" 2 "$(status GET "/api/v1/business/$mine/professionals" "" "$T1" >/dev/null; json '.items | length')"
check "barber is a PROFESSIONAL member" PROFESSIONAL "$(status GET "/api/v1/business/$mine/members" "" "$T1" >/dev/null; json "[.items[] | select(.userId==\"$staff\") | .role][0]")"

# --- weekly schedule (T5.3): the OWNER sets it; the barber adjusts his own, not the owner's ---
week='{"days":[{"weekday":"MONDAY","workingHours":[{"start":"09:00","end":"13:00"},{"start":"14:00","end":"19:00"}],"breaks":[{"start":"11:00","end":"11:15"}]},{"weekday":"SATURDAY","workingHours":[{"start":"08:00","end":"14:00"}]}]}'
check "owner sets the barber's week" 200 "$(status PUT "/api/v1/business/$mine/professionals/$pbarber/schedule" "$week" "$T1")"
check "  seven days, Monday first" "7 MONDAY 14:00" "$(json '"\(.days | length) \(.days[0].weekday) \(.days[0].workingHours[1].start)"')"
check "break outside working hours" 400 "$(status PUT "/api/v1/business/$mine/professionals/$pbarber/schedule" '{"days":[{"weekday":"MONDAY","workingHours":[{"start":"09:00","end":"13:00"}],"breaks":[{"start":"12:30","end":"13:30"}]}]}' "$T1")"
check "  code BREAK_OUTSIDE_WORKING_HOURS" BREAK_OUTSIDE_WORKING_HOURS "$(json '.errors[0].message')"
check "barber login" 200 "$(login "$STAFF" "$PW1")"
TS=$(token)
check "barber adjusts his own week" 200 "$(status PUT "/api/v1/business/$mine/professionals/$pbarber/schedule" '{"days":[{"weekday":"TUESDAY","workingHours":[{"start":"10:00","end":"20:00"}],"breaks":[{"start":"13:00","end":"14:00"}]}]}' "$TS")"
check "  monday is now off" "0 1" "$(json '"\(.days[0].workingHours | length) \(.days[1].breaks | length)"')"
check "barber cannot edit the owner's week" 403 "$(status PUT "/api/v1/business/$mine/professionals/$powner/schedule" "$week" "$TS")"
check "barber reads the owner's week" 200 "$(status GET "/api/v1/business/$mine/professionals/$powner/schedule" "" "$TS")"

# --- absences (T5.4): the barber registers his own sick leave; overlaps are rejected ---------
abs="/api/v1/business/$mine/professionals/$pbarber/absences"
check "barber registers sick leave" 201 "$(status POST "$abs" '{"start":"2030-12-01T08:00:00-05:00","end":"2030-12-03T19:00:00-05:00","reason":"Incapacidad E2E"}' "$TS")"
check "  stored in UTC" "2030-12-01T13:00:00Z" "$(json .start)"
absence=$(json .id)
check "overlapping absence" 409 "$(status POST "$abs" '{"start":"2030-12-02T00:00:00Z","end":"2030-12-04T00:00:00Z"}' "$T1")"
check "  code ABSENCE_OVERLAP" ABSENCE_OVERLAP "$(json .code)"
check "barber cannot register the owner's absence" 403 "$(status POST "/api/v1/business/$mine/professionals/$powner/absences" '{"start":"2030-12-05T00:00:00Z","end":"2030-12-06T00:00:00Z"}' "$TS")"
check "owner lists the barber's absences" 1 "$(status GET "$abs?from=2030-01-01T00:00:00Z" "" "$T1" >/dev/null; json '.items | length')"
check "barber changes the reason" 200 "$(status PATCH "$abs/$absence" '{"reason":"Cita médica"}' "$TS")"
check "barber deletes it" 204 "$(status DELETE "$abs/$absence" "" "$TS")"

# --- catalog (T5.5): the OWNER sets categories and services; the barber only reads ----------
check "create category" 201 "$(status POST "/api/v1/business/$mine/service-categories" '{"name":"Barba"}' "$T1")"
cat=$(json .id)
check "category name taken (any case)" 409 "$(status POST "/api/v1/business/$mine/service-categories" '{"name":"BARBA"}' "$T1")"
check "create 'Corte + barba'" 201 "$(status POST "/api/v1/business/$mine/services" "{\"name\":\"Corte + barba\",\"categoryId\":\"$cat\",\"durationMinutes\":45,\"priceMinor\":3500000}" "$T1")"
check "  45 min, 35.000 COP" "45 3500000 COP" "$(json '"\(.durationMinutes) \(.priceMinor) \(.currency)"')"
svc=$(json .id)
check "price with cents rejected" 400 "$(status POST "/api/v1/business/$mine/services" '{"name":"Tinte","durationMinutes":30,"priceMinor":2500050}' "$T1")"
check "barber cannot create services" 403 "$(status POST "/api/v1/business/$mine/services" '{"name":"Tinte","durationMinutes":30,"priceMinor":2500000}' "$TS")"
check "barber lists services" 1 "$(status GET "/api/v1/business/$mine/services" "" "$TS" >/dev/null; json '.items | length')"
check "owner changes the price" 4000000 "$(status PATCH "/api/v1/business/$mine/services/$svc" '{"priceMinor":4000000}' "$T1" >/dev/null; json .priceMinor)"

# --- services per professional (T5.6): the OWNER assigns, the barber reads -------------------
ps="/api/v1/business/$mine/professionals/$pbarber/services"
check "owner assigns 'Corte + barba' to the barber" 1 "$(status PUT "$ps" "{\"serviceIds\":[\"$svc\"]}" "$T1" >/dev/null; json '.items | length')"
check "barber sees his services" "Corte \+ barba" "$(status GET "$ps" "" "$TS" >/dev/null; json '.items[0].name')"
check "barber cannot assign services" 403 "$(status PUT "$ps" '{"serviceIds":[]}' "$TS")"
check "unknown service rejected" 400 "$(status PUT "$ps" '{"serviceIds":["01a20aae-0000-7000-8000-000000000000"]}' "$T1")"

# --- public view (T5.7): now bookable, listed and with its public catalog --------------------
check "public search finds it (no auth)" 1 "$(search_mine)"
check "  no internal fields" 0 "$(json '[.items[0] | keys[] | select(. == "status" or . == "reminders" or . == "createdAt")] | length')"
check "public catalog (no auth)" 200 "$(status GET "/api/v1/businesses/$myslug/catalog")"
check "  service, price and barber" "Corte \\+ barba 4000000 Barbero E2E" "$(json '"\(.services[0].name) \(.services[0].priceMinor) \(.professionals[0].displayName)"')"
check "  no internal fields" 0 "$(json '[.professionals[0] | keys[] | select(. == "userId" or . == "status" or . == "email")] | length')"
check "unknown slug catalog -> 404" 404 "$(status GET /api/v1/businesses/no-existe-e2e/catalog)"

# --- public availability (T6.2): the barber works Tuesdays 10:00-20:00, break 13:00-14:00 -----
# (slot interval 15 min since "owner edits the business"; "Corte + barba" lasts 45 min)
now=$(date +%s)
for i in 2 3 4 5 6 7 8; do
  e=$((now + i * 86400))
  if [[ $(date -u -d "@$e" +%u) == 2 ]]; then break; fi
done
tue=$(date -u -d "@$e" +%F)
wed=$(date -u -d "@$((e + 86400))" +%F)
av="/api/v1/businesses/$myslug/availability?serviceId=$svc&professionalId=$pbarber"
check "availability next Tuesday (no auth)" 200 "$(status GET "$av&date=$tue")"
check "  45 min every 15: 10:00-12:15 and 14:00-19:15" "32 ${tue}T10:00:00-05:00 ${tue}T19:15:00-05:00" "$(json '"\(.slots | length) \(.slots[0].startLocal) \(.slots[-1].startLocal)"')"
slot=$(json '.slots[0].startAt')
check "  no internal fields" 0 "$(json '[keys[] | select(. != "date" and . != "timeZone" and . != "serviceId" and . != "professionalId" and . != "durationMinutes" and . != "slots")] | length')"
check "a day he does not work" 0 "$(status GET "$av&date=$wed" >/dev/null; json '.slots | length')"
check "date in the past" 400 "$(status GET "$av&date=2020-01-01")"
check "  code DATE_OUTSIDE_BOOKING_WINDOW" DATE_OUTSIDE_BOOKING_WINDOW "$(json .code)"
check "owner does not perform the service -> 404" 404 "$(status GET "/api/v1/businesses/$myslug/availability?serviceId=$svc&professionalId=$powner&date=$tue")"
check "invalid date -> 400" 400 "$(status GET "$av&date=manana")"

# --- booking (T6.3): the owner books the barber's first Tuesday slot as a customer ------------
book() { # start idempotency-key token
  status POST /api/v1/appointments "{\"businessId\":\"$mine\",\"serviceId\":\"$svc\",\"professionalId\":\"$pbarber\",\"startAt\":\"$1\"}" "$3" "Idempotency-Key: $2"
}
k1="e2e-$(date +%s)-a"
check "phone from the registration (E.164)" "\+573001234567" "$(status GET /api/v1/me "" "$T1" >/dev/null; json .phone)"
check "invalid phone -> 400" 400 "$(status PUT /api/v1/me/phone '{"phone":"12345"}' "$T1")"
check "change phone" "\+573001112233" "$(status PUT /api/v1/me/phone '{"phone":"300 111 2233"}' "$T1" >/dev/null; json .phone)"
check "book without Idempotency-Key -> 400" 400 "$(status POST /api/v1/appointments "{\"businessId\":\"$mine\",\"serviceId\":\"$svc\",\"professionalId\":\"$pbarber\",\"startAt\":\"$slot\"}" "$T1")"
check "book Tuesday 10:00" 201 "$(book "$slot" "$k1" "$T1")"
appt=$(json .id)
check "  pending, snapshot of the service" "PENDING_CONFIRMATION Corte \+ barba 4000000 COP 45" "$(json '"\(.status) \(.serviceName) \(.priceMinor) \(.currency) \(.durationMinutes)"')"
check "same Idempotency-Key -> same appointment" "201 $appt" "$(book "$slot" "$k1" "$T1") $(json .id)"
check "same key, other slot -> 422" 422 "$(book "${tue}T20:00:00Z" "$k1" "$T1")"
check "  code IDEMPOTENCY_KEY_REUSED" IDEMPOTENCY_KEY_REUSED "$(json .code)"
check "slot taken (new key) -> 409" 409 "$(book "$slot" "e2e-$(date +%s)-b" "$T1")"
check "  code SLOT_UNAVAILABLE" SLOT_UNAVAILABLE "$(json .code)"
check "booking needs a session" 401 "$(book "$slot" "e2e-$(date +%s)-c" "")"
check "availability without 10:00-10:45" "29 ${tue}T10:45:00-05:00" "$(status GET "$av&date=$tue" >/dev/null; json '"\(.slots | length) \(.slots[0].startLocal)"')"
status POST /api/v1/auth/logout "" "$TS" >/dev/null
check "  one per account" 409 "$(status POST /api/v1/businesses "$own" "$T1")"
check "  code ALREADY_OWNER" ALREADY_OWNER "$(json .code)"
if [[ -n ${ADMIN_EMAIL:-} && -n ${ADMIN_PASSWORD:-} ]]; then
  check "admin login" 200 "$(login "$ADMIN_EMAIL" "$ADMIN_PASSWORD")"
  TA=$(token)
  check "create business (owner = e2e user)" 201 "$(status POST /api/v1/admin/businesses "{\"name\":\"QA E2E $(date +%s)\",\"contactPhone\":\"300 123 4567\",\"cityId\":\"$cali\",\"addressLine\":\"Calle 5 # 38-25\",\"ownerEmail\":\"$EMAIL\"}" "$TA")"
  biz=$(json .id)
  check "  starts ACTIVE (no activation needed), COP, America/Bogota" "ACTIVE COP America/Bogota \\+573001234567" "$(json '"\(.status) \(.currency) \(.timeZone) \(.contactPhone)"')"
  check "owner sees the membership" OWNER "$(status GET /api/v1/me/memberships "" "$T1" >/dev/null; json ".items[] | select(.businessId==\"$biz\") | .role")"
  check "suspend business (moderation)" SUSPENDED "$(status POST "/api/v1/admin/businesses/$biz/suspend" "" "$TA" >/dev/null; json .status)"
  check "reactivate business" ACTIVE "$(status POST "/api/v1/admin/businesses/$biz/activate" "" "$TA" >/dev/null; json .status)"
  check "suspend again (left suspended)" SUSPENDED "$(status POST "/api/v1/admin/businesses/$biz/suspend" "" "$TA" >/dev/null; json .status)"
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
