# cache-demo

A side-by-side comparison of a "get profile" request served two ways:
straight from a database every time, versus backed by a Redis cache. Includes
a small Vite frontend so you can click buttons and watch the numbers live,
which is what you'd actually show while presenting this.

No external Go dependencies — the Redis client is a ~100-line hand-rolled
RESP-protocol client in `resp.go`, and the optional Supabase integration is
plain `net/http` calling Supabase's auto-generated REST API. Just Go's
standard library end to end.

## What it shows

| Endpoint                | What happens                                                                                   | Typical latency                                                        |
| ----------------------- | ---------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------- |
| `GET /profile/no-cache` | Always hits "the database" (simulated, or real Supabase if configured)                         | Flat, every single request                                             |
| `GET /profile/cached`   | Checks Redis first; on a miss, fetches + caches for 30s; on a hit, returns straight from cache | Same as no-cache on the first request (miss), then **under 5ms** after |

Every response includes a `duration_ms` field measured server-side.

## Project layout

```
cache-demo/
  main.go        - the two HTTP handlers + CORS
  supabase.go    - optional real-DB fetch via Supabase's REST API
  resp.go        - minimal hand-rolled Redis client (SET with TTL, GET, DEL)
  go.mod
  frontend/      - Vite app: buttons + live latency comparison
```

## Running the backend

You need Go 1.21+ and a Redis server reachable at `localhost:6379`.

```bash
# terminal 1 - start Redis (skip if you already have one running)
redis-server --port 6379

# terminal 2 - build and run the demo server
go build -o cache-demo .
./cache-demo
```

By default (no env vars set) it uses the simulated ~180ms fetch, so this
runs with zero setup. To point it at a real Supabase table instead, see the
next section.

## Running the frontend

```bash
cd frontend
npm install
npm run dev
```

Open the printed local URL (typically `http://localhost:5173`). It defaults
to talking to the backend at `http://localhost:8080` — there's an input box
at the top of the page if you're hosting the backend somewhere else, and it
remembers what you type there (saved in the browser, so it persists across
reloads).

Click **Fetch — No Cache** a few times: the number stays roughly flat.
Click **Reset Cache** then **Fetch — Cached** a few times: the first one
matches the no-cache number (that's the miss), then every one after drops to
single-digit milliseconds. That contrast is the whole demo.

## Plugging in a real database (Supabase)

Right now `simulateDBFetch()` in `main.go` just sleeps for 180ms and returns
a hardcoded record — a stand-in for "the cost of a real query." Since
you're hosting and presenting this, here's how to swap that for an actual
Supabase table without touching any of the caching logic.

**1. Create the table.** In your Supabase project, open the SQL editor and run:

```sql
create table profiles (
  id serial primary key,
  name text not null,
  brand text not null,
  role text not null,
  location text not null,
  stack text[] not null,
  bio text not null
);

insert into profiles (name, brand, role, location, stack, bio) values (
  'Ridwan Ambali',
  'Code Red',
  'Full Stack & Mobile Developer',
  'Abuja, Nigeria',
  array['React Native', 'Flutter', 'Laravel', 'Django Channels', 'Go'],
  '300-level CS student at AFIT Kaduna, freelances on Upwork & Fiverr.'
);
```

**2. Allow it to be read.** Supabase tables have Row Level Security on by
default, which blocks all access until you add a policy. For a public demo
record, a read-only policy is enough:

```sql
alter table profiles enable row level security;

create policy "public read access"
  on profiles for select
  using (true);
```

**3. Get your credentials.** In the Supabase dashboard: **Project Settings
→ API**. You need the **Project URL** and the **anon public** key (not the
service_role key — that one bypasses Row Level Security and should never be
put in client-facing config, though here it's only ever read server-side).

**4. Point the backend at it.** Set two environment variables before
running the server:

```bash
export SUPABASE_URL="https://<your-project-ref>.supabase.co"
export SUPABASE_ANON_KEY="<your-anon-key>"
./cache-demo
```

The startup log will confirm: `data source: Supabase (SUPABASE_URL is set)`.
That's the whole change — `dbFetch()` in `supabase.go` now does a real HTTP
round trip to Supabase's PostgREST API instead of sleeping, so your
"no-cache" numbers become genuine network + query latency (likely
100-400ms depending on where you and the Supabase region are), and the
cached path is unchanged — still Redis, still fast. That contrast is
actually a _stronger_ demo than the simulated version, since the "no-cache"
number is now real, not fabricated.

**Why REST instead of a Postgres driver?** Supabase gives you a normal
Postgres connection too, but talking to Postgres directly needs a driver
(`lib/pq`, `pgx`, etc.), which is a `go get` away — and if you hit the same
module-proxy restriction I ran into building this, the REST API sidesteps
it entirely since it's just HTTP. If you deploy somewhere with normal
internet access and want the "more real" version, swapping to a direct
Postgres connection is a reasonable next step, but not necessary for the
demo to work or to be truthful about what it's measuring.

## Plugging in a real Redis (Upstash)

`resp.go` defaults to `localhost:6379` with no password, which is enough
for local dev. To point it at a managed instance instead:

**1. Create a database.** In the [Upstash console](https://console.upstash.com),
create a Redis database in a region close to wherever you're hosting the
backend.

**2. Get the connection string.** Copy the `rediss://` URL from the
database's dashboard — it looks like
`rediss://default:<password>@<host>.upstash.io:6379`. The extra `s` in
`rediss` matters: Upstash's free tier only accepts TLS connections.

**3. Point the backend at it.**

```bash
export REDIS_URL="rediss://default:<password>@<host>.upstash.io:6379"
./cache-demo
```

`NewRedisClient()` reads `REDIS_URL` at startup, switches the socket to TLS
for a `rediss://` scheme (plain TCP for `redis://`), and sends an `AUTH`
command with the password from the URL before every command. No other code
changes needed. Leave `REDIS_URL` unset and it falls back to
`localhost:6379` with no auth, exactly as before.

## Hosting it for a presentation

A few things worth knowing before you deploy:

- **Redis needs to be reachable from wherever the backend runs.** Options:
  a small Redis instance on the same host/container as the backend (simplest),
  or a managed free-tier Redis (Upstash, Redis Cloud). Set `REDIS_URL` to
  point at it — see "Plugging in a real Redis (Upstash)" below.
- **CORS is wide open** (`Access-Control-Allow-Origin: *`) in `main.go`,
  which is fine for a demo but worth tightening to your actual frontend's
  domain if this stays running past the presentation.
- **The frontend just needs static hosting** once built — `npm run build`
  in `frontend/` produces a `dist/` folder you can drop on Vercel, Netlify,
  or GitHub Pages. Just make sure the backend URL box points at wherever
  you deployed the Go server.

## Files

- `main.go` — HTTP handlers, CORS, the response shape
- `supabase.go` — the real-DB fetch path (only used when env vars are set)
- `resp.go` — the minimal hand-rolled Redis client (SET with TTL, GET, DEL)
- `go.mod` — module file (no external dependencies)
- `frontend/` — Vite vanilla-JS app: buttons, live results, request history bars

# devops-redis-cache
