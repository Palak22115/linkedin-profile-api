# LinkedIn Profile API

A hosted HTTP API that accepts a LinkedIn profile URL and returns the profile
as structured JSON — name, headline, location, about, experience, education,
skills, certifications, languages, and profile images.

It is **purely reverse‑engineered**: the backend calls LinkedIn's internal
"Voyager" JSON endpoints directly with a copied browser session cookie. There
is **no browser, no headless Chrome, no Selenium/Playwright** anywhere in the
stack.

```
GET /api/profile?url=https://www.linkedin.com/in/williamhgates/
X-API-Key: <your key>
```

---

## Contents

- [Quick start](#quick-start)
- [Configuration — getting the LinkedIn cookie](#configuration--getting-the-linkedin-cookie)
- [Deployment (Render)](#deployment-render)
- [API documentation](#api-documentation)
- [Approach](#approach)
- [Known limitations](#known-limitations)
- [Project layout](#project-layout)
- [Legal / Terms of Service](#legal--terms-of-service)

---

## Quick start

Requirements: **Go 1.22+**. No external Go dependencies.

```bash
git clone https://github.com/palak-kasoundhan/linkedin-profile-api.git
cd linkedin-profile-api

cp .env.example .env
# edit .env — set LINKEDIN_COOKIE, JSESSIONID and API_KEY (see next section)

go run ./cmd/server
# {"level":"INFO","msg":"listening","addr":":8080", ...}
```

Call it:

```bash
curl -s -H "X-API-Key: $API_KEY" \
  "http://localhost:8080/api/profile?url=https://www.linkedin.com/in/williamhgates/" | jq
```

Run the tests:

```bash
go test ./...
go vet ./...
```

There is also a one‑shot CLI for development that prints both the raw Voyager
payloads and the parsed result:

```bash
go run ./cmd/probe "https://www.linkedin.com/in/williamhgates/"
```

---

## Configuration — getting the LinkedIn cookie

The API authenticates to LinkedIn with **your own session cookie**. Use a
**burner / secondary account**, never your primary one (see
[Legal](#legal--terms-of-service)).

1. Log into `https://www.linkedin.com` in Chrome with the burner account.
2. Open DevTools (**F12**) → **Network** tab. Tick **Preserve log**.
3. Reload the page. Click the first document request (`feed/` or
   `www.linkedin.com`).
4. Right‑click it → **Copy** → **Copy as cURL (bash)**.
5. From that command, take the value of the **`-b` / `--cookie`** flag — the
   whole cookie string. It must contain at least `li_at`, `JSESSIONID` and
   `lidc` (LinkedIn returns a redirect loop without `lidc`).
6. Put it in `.env`:

   ```
   LINKEDIN_COOKIE=bcookie="v=2&<...>"; JSESSIONID=ajax:<digits>; li_at=<token>; lidc="b=<...>"; <the rest>
   JSESSIONID=ajax:<digits>
   ```

   `JSESSIONID` can be omitted — the app will read it out of `LINKEDIN_COOKIE`.
   The `csrf-token` request header is derived from it automatically.

| Variable | Required | Notes |
|---|---|---|
| `LINKEDIN_COOKIE` | ✅ | Full `Cookie:` header string from the browser |
| `JSESSIONID` | — | Falls back to the value inside `LINKEDIN_COOKIE` |
| `API_KEY` | recommended | Clients must send it as `X-API-Key`. Empty = auth disabled |
| `LINKEDIN_USER_AGENT` | — | Defaults to a recent Chrome UA; ideally match your browser |
| `CACHE_TTL` | — | Default `24h`. `0` disables the in‑memory cache |
| `PORT` | — | Default `8080` (Render injects its own) |

**Refreshing:** the session lasts weeks to months but LinkedIn can invalidate
it sooner. When the API starts returning `503 session_expired`, repeat the
steps above and update `LINKEDIN_COOKIE`.

---

## Deployment (Render)

The repo ships a [`Dockerfile`](Dockerfile) (multi‑stage → distroless, ~10 MB)
and a [`render.yaml`](render.yaml) blueprint.

1. Push this repo to GitHub.
2. Render dashboard → **New +** → **Blueprint** → select the repo.
3. Render reads `render.yaml` and creates the service. When prompted, set the
   secret env vars (they are marked `sync: false` so they are **not** stored in
   the repo):
   - `LINKEDIN_COOKIE`
   - `JSESSIONID`
   - `API_KEY`
4. Deploy. Health check is `GET /health`.
5. Your API is live at `https://<service-name>.onrender.com`.

Any Docker host works too:

```bash
docker build -t linkedin-profile-api .
docker run -p 8080:8080 --env-file .env linkedin-profile-api
```

---

## API documentation

### `GET /api/profile`

| | |
|---|---|
| Query param | `url` — a LinkedIn profile URL (`https://www.linkedin.com/in/<slug>/`) or a bare `<slug>` |
| Header | `X-API-Key: <key>` — required when `API_KEY` is set |
| Success | `200` with the profile JSON below |

#### Example request

```bash
curl -s -H "X-API-Key: secret" \
  "https://your-api.onrender.com/api/profile?url=https://www.linkedin.com/in/williamhgates/"
```

#### Example response (truncated)

```json
{
  "publicIdentifier": "williamhgates",
  "profileId": "ACoAAA8BYqEB...",
  "firstName": "Bill",
  "lastName": "Gates",
  "fullName": "Bill Gates",
  "headline": "Chair, Gates Foundation and Founder, Breakthrough Energy",
  "about": "Chair of the Gates Foundation. Founder of Breakthrough Energy. ...",
  "location": { "full": "Seattle, Washington, United States", "countryCode": "US" },
  "profilePicture": {
    "url": "https://media.licdn.com/dms/image/v2/.../profile-displayphoto-shrink_800_800/...",
    "variants": [
      { "url": "https://media.licdn.com/.../800_800/...", "width": 800, "height": 800 },
      { "url": "https://media.licdn.com/.../400_400/...", "width": 400, "height": 400 }
    ]
  },
  "backgroundPicture": { "url": "https://media.licdn.com/.../profile-displaybackgroundimage-shrink_350_1400/...", "variants": [ ... ] },
  "verified": true,
  "premium": true,
  "influencer": true,
  "experience": [
    {
      "title": "Co-chair",
      "companyName": "Gates Foundation",
      "companyUrn": "urn:li:fsd_company:8736",
      "dateRange": { "start": { "year": 2000 } },
      "current": true
    }
  ],
  "education": [
    {
      "schoolName": "Harvard University",
      "schoolUrn": "urn:li:fsd_school:18483",
      "dateRange": { "start": { "year": 1973 }, "end": { "year": 1975 } }
    }
  ],
  "skills": [],
  "certifications": [],
  "languages": [],
  "sourceUrl": "https://www.linkedin.com/in/williamhgates/",
  "retrievedAt": "2026-08-31T09:15:04Z",
  "partial": true,
  "notes": ["section not retrieved: skills", "section not retrieved: languages"]
}
```

#### Response schema

| Field | Type | Notes |
|---|---|---|
| `publicIdentifier` | string | vanity slug |
| `profileId` | string | internal `fsd_profile` id |
| `firstName`, `lastName`, `fullName` | string | |
| `headline` | string | |
| `about` | string | the "About" section (`\r\n` normalised to `\n`) |
| `location` | `{ full, countryCode }` | `full` is best‑effort (see limitations) |
| `industry` | string | when available |
| `profilePicture`, `backgroundPicture` | `{ url, variants[] }` | `url` = largest; `variants` each `{ url, width, height }` |
| `verified`, `premium`, `influencer` | bool | profile badges |
| `followerCount`, `connectionCount` | int | when available |
| `experience[]` | `{ title, companyName, companyUrn, employmentType, location, description, dateRange, current }` | order preserved |
| `education[]` | `{ schoolName, schoolUrn, degreeName, fieldOfStudy, grade, activities, description, dateRange }` | |
| `skills[]` | `{ name }` | |
| `certifications[]` | `{ name, authority, licenseNumber, url, dateRange }` | |
| `languages[]` | `{ name, proficiency }` | proficiency humanised, e.g. `Native or bilingual` |
| `projects[]`, `volunteering[]`, `honors[]`, `publications[]`, `courses[]` | | omitted when empty |
| `dateRange` | `{ start?: {year, month}, end?: {year, month} }` | `month` omitted when unknown; `end` absent = ongoing |
| `sourceUrl` | string | canonical profile URL |
| `retrievedAt` | RFC3339 string | fetch time (UTC) |
| `partial` | bool | `true` if a requested section could not be retrieved |
| `notes[]` | string | explains what is missing / partial |

#### Error responses

All errors share the shape `{ "error": { "code": "...", "message": "..." } }`.

| HTTP | `code` | Meaning |
|---|---|---|
| 400 | `missing_parameter` | no `url` query param |
| 400 | `invalid_url` | not a LinkedIn member profile URL |
| 401 | `unauthorized` | missing/invalid `X-API-Key` |
| 404 | `profile_not_found` | profile doesn't exist or isn't visible |
| 429 | `upstream_rate_limited` | LinkedIn is throttling the backend |
| 502 | `upstream_blocked` | LinkedIn anti‑bot block |
| 502 | `upstream_error` | unexpected LinkedIn response |
| 503 | `session_expired` | the backend cookie is dead — operator must refresh it |
| 504 | `upstream_timeout` | fetch took too long |

### `GET /health`

`200 { "status": "ok", "uptime": "..." }` — no auth required.

### `GET /`

Plain‑text usage summary.

---

## Approach

### 1. LinkedIn has no public profile API

The linkedin.com web app talks to a private JSON API internally called
**Voyager** (`https://www.linkedin.com/voyager/api/...`). It only answers
authenticated member sessions, so the backend replays a real browser session
cookie (`li_at` + `JSESSIONID` + `lidc` + the rest). The `csrf-token` header
must equal the `JSESSIONID` value.

### 2. The classic profile endpoints are gone

The endpoints most public scrapers use —
`/voyager/api/identity/profiles/{id}/profileView`, `/skills`,
`/profileContactInfo` — now return **HTTP 410 Gone**. LinkedIn also rebuilt the
profile *page* itself as server‑driven UI (React Server Components), so there is
no longer a single "profile JSON" call the website makes.

### 3. What still works: the `identity/dash/*` Rest.li collections

These are alive and, crucially, need **no rotating GraphQL `queryId` hash** —
just a stable Rest.li finder parameter:

| Data | Request |
|---|---|
| Core profile | `GET /voyager/api/identity/dash/profiles/urn:li:fsd_profile:{id}` |
| Experience | `identity/dash/profilePositions?q=viewee&profileUrn={urn}` |
| Education | `identity/dash/profileEducations?q=viewee&profileUrn={urn}` |
| Skills | `identity/dash/profileSkills?q=viewee&profileUrn={urn}` |
| Certifications | `identity/dash/profileCertifications?q=viewee&profileUrn={urn}` |
| Languages | `identity/dash/profileLanguages?q=viewee&profileUrn={urn}` |
| Projects, volunteering, honors, publications, courses | same `?q=viewee&profileUrn=` pattern |

They return LinkedIn's normalised "Decoration" JSON — a `data` envelope with an
ordered `*elements` URN list plus a bag of typed entities in `included[]`.

### 4. Resolving the URL → internal id

The section endpoints key off the internal `fsd_profile` id, not the vanity
slug, and there is no clean slug→id JSON endpoint. So the backend fetches the
public profile **HTML page** once (`GET /in/{slug}/`) and extracts the id with a
regex (the subject's id is by far the most frequent match). That same HTML is
reused to recover the human‑readable location string, which the `dash` core
response only returns as a `geo` URN.

### 5. Request flow

```
GET /api/profile?url=...
  └─ parse URL → vanity slug
  └─ fetch /in/{slug}/ HTML         → internal id + location text
  └─ GET identity/dash/profiles/{urn}          → core
  └─ GET identity/dash/profile{Positions,Educations,Skills,...}  → sections
  └─ normalise included[] graph → map to the response schema
  └─ cache by slug (default 24h)
```

### 6. Staying under the radar

The client that talks to LinkedIn:

- **paces every request** (≈1 s + jitter) and serialises them — bursts get the
  session invalidated;
- sends browser‑matching headers (`csrf-token`, `x-restli-protocol-version`,
  `x-li-lang`, `x-li-track`, real `User-Agent`);
- **does not follow redirects** — a `302` back to the same URL is LinkedIn's
  edge block, which the client detects and reports as `502 upstream_blocked`;
- detects `li_at=delete me` / `401` / login redirects and reports
  `503 session_expired`;
- caches results so the same profile isn't re‑fetched within the TTL.

### Tech

Go standard library only — `net/http` server (1.22 routing), `encoding/json`,
`log/slog`. No web framework, no scraper library, no DB. Multi‑stage Docker
build to a distroless static image.

---

## Known limitations

- **Terms of Service.** Automated access to LinkedIn violates their User
  Agreement. Use a burner account. See [Legal](#legal--terms-of-service).
- **Cookie lifetime.** The session cookie must be refreshed manually when it
  expires or gets flagged (`503 session_expired`).
- **Rate limits & anti‑bot.** From datacenter IPs (including most cloud hosts)
  LinkedIn blocks more aggressively. The pacing + cache mitigate this but a
  burst of unique profiles can still trip `429` / `502`. A residential/mobile
  proxy for the outbound calls would harden this; not included here.
- **Connection‑gated sections.** LinkedIn hides **skills**, **languages**, and
  sometimes **certifications** for people you are not connected to. When a
  section can't be retrieved the response sets `"partial": true` and lists it in
  `notes`. Connect the burner account to targets, or accept partial data.
- **Location is best‑effort.** The `dash` API returns location only as a geo
  URN; the readable string (`"Seattle, Washington, United States"`) is scraped
  from the profile HTML and can be missing for unusual page layouts.
  `location.countryCode` is always reliable when present.
- **`employmentType`, `industry`, company/school logos** are returned by
  LinkedIn as URNs and are not separately resolved (would need extra calls).
- **Schema drift.** These are private endpoints; LinkedIn can change or retire
  them without notice. Field mappings for experience, education, skills,
  certifications, projects, courses and honors are validated against real
  profiles; **languages, volunteering and publications** are mapped from
  LinkedIn's entity shapes but have not yet been seen populated in testing.
- **Single account = single throughput.** No account pool / rotation.

---

## Project layout

```
cmd/
  server/        HTTP server entrypoint
  probe/         dev CLI: dump raw + parsed output for one profile
internal/
  config/        env / .env loading
  linkedin/      reverse-engineered Voyager client
    client.go      paced HTTP client, headers, anti-bot detection
    resolve.go     URL -> vanity -> internal profile id (via HTML)
    profile.go     the identity/dash/* section calls
    restli.go      normalised included[] / *elements graph decoding
    errors.go      sentinel errors
  scrape/        orchestration: resolve -> fetch -> parse -> cache
  parse/         raw Voyager JSON -> public schema  (+ tests)
  model/         the public response schema
  api/           HTTP handlers, middleware (API key, logging, recover), error mapping  (+ tests)
Dockerfile
render.yaml
```

---

## Legal / Terms of Service

This project accesses LinkedIn through undocumented internal endpoints, which
is contrary to the [LinkedIn User Agreement](https://www.linkedin.com/legal/user-agreement)
and its prohibition on automated data collection. It is provided for
educational and research purposes as a hiring‑challenge submission.

- Use a **secondary account** you are willing to lose — automated access can get
  an account restricted or banned.
- Only the member's **own account credentials** are used; no other user's
  credentials are involved.
- Keep all credentials **out of the repository** — they live only in `.env`
  (git‑ignored) locally and in the host's secret store in production.
- Respect the data. Don't build bulk databases of people who haven't consented.
