#!/usr/bin/env bash
# Run a list of LinkedIn profile URLs through the running API server and save
# each result. The server caches (CACHE_TTL, default 24h), so re-running is
# instant and does not re-hit LinkedIn.
#
#   1. start the server in another terminal:   go run ./cmd/server
#   2. bash scripts/batch.sh                    # reads scripts/urls.txt
#      bash scripts/batch.sh my-list.txt        # a different list file
#      bash scripts/batch.sh https://www.linkedin.com/in/foo/ bar-slug
#
# Env: API_BASE (default http://localhost:8080), API_KEY (default from .env)
# Output: recon/out/<slug>.json

set -u
cd "$(dirname "$0")/.."

API_BASE="${API_BASE:-http://localhost:8080}"
if [ -z "${API_KEY:-}" ] && [ -f .env ]; then
  API_KEY=$(grep -E '^API_KEY=' .env | cut -d= -f2- | tr -d '"')
fi
OUT_DIR="recon/out"
mkdir -p "$OUT_DIR"

# Collect targets: a file arg, inline args, or the default list.
targets=()
if [ "$#" -gt 0 ] && [ -f "$1" ]; then
  mapfile -t targets < <(grep -vE '^\s*(#|$)' "$1")
elif [ "$#" -gt 0 ]; then
  targets=("$@")
else
  mapfile -t targets < <(grep -vE '^\s*(#|$)' scripts/urls.txt)
fi
[ "${#targets[@]}" -eq 0 ] && { echo "no URLs found" >&2; exit 1; }

# Server up?
if ! curl -sf -o /dev/null "$API_BASE/health"; then
  echo "server not reachable at $API_BASE — start it with:  go run ./cmd/server" >&2
  exit 1
fi

printf '\n%-38s %-6s %s\n' "SLUG" "HTTP" "SUMMARY"
printf '%s\n' "--------------------------------------------------------------------------"

for t in "${targets[@]}"; do
  slug=$(echo "$t" | sed -E 's#.*/in/##; s#/.*##; s#\?.*##')
  case "$t" in http*|*/*) url="$t" ;; *) url="https://www.linkedin.com/in/$t/" ;; esac

  body=$(curl -s -w $'\n%{http_code}' -H "X-API-Key: $API_KEY" \
    --get "$API_BASE/api/profile" --data-urlencode "url=$url")
  code=$(printf '%s' "$body" | tail -n1)
  json=$(printf '%s' "$body" | sed '$d')

  if [ "$code" = "200" ]; then
    printf '%s' "$json" > "$OUT_DIR/$slug.json"
    sum=$(printf '%s' "$json" | node -e '
      let s="";process.stdin.on("data",d=>s+=d).on("end",()=>{
        const j=JSON.parse(s);
        const n=(a)=>Array.isArray(a)?a.length:0;
        console.log(`${j.fullName} | ${j.location?.full||j.location?.countryCode||"?"} `+
          `| exp=${n(j.experience)} edu=${n(j.education)} skl=${n(j.skills)} `+
          `cert=${n(j.certifications)} lang=${n(j.languages)} proj=${n(j.projects)} `+
          `hon=${n(j.honors)} crs=${n(j.courses)}${j.partial?" [PARTIAL]":""}`);
      });' 2>/dev/null)
    printf '%-38s %-6s %s\n' "$slug" "$code" "$sum"
  else
    err=$(printf '%s' "$json" | node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>{try{console.log(JSON.parse(s).error.code)}catch{console.log("?")}})' 2>/dev/null)
    printf '%-38s %-6s %s\n' "$slug" "$code" "$err"
  fi
done

echo
echo "saved to $OUT_DIR/"

exit 0
