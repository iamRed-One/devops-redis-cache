# cache-demo

A side-by-side comparison of a "current weather in Kaduna" request served two ways:
by calling a live internet API every time, versus backed by a Redis cache. Includes
a small Vite frontend so you can click buttons and watch the numbers live,
which is what you'd actually show while presenting this.

No external Go dependencies — the Redis client is a ~100-line hand-rolled
RESP-protocol client in `resp.go`, and the weather lookup is plain `net/http`
calling the free Open-Meteo API (no key). Just Go's standard library end to end.

## What it shows

| Endpoint                | What happens                                                                                   | Typical latency                                                        |
| ----------------------- | ---------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------- |
| `GET /weather/no-cache` | Always calls the live Open-Meteo weather API                         | Flat, every single request                                             |
| `GET /weather/cached`   | Checks Redis first; on a miss, fetches + caches for 30s; on a hit, returns straight from cache | Same as no-cache on the first request (miss), then **under 5ms** after |

Every response includes a `duration_ms` field measured server-side.

## Project layout

```
cache-demo/
  main.go        - the two HTTP handlers + CORS
  weather.go     - live weather fetch from the Open-Meteo API
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

It calls the free Open-Meteo weather API (no key needed), so the machine
running it needs internet access.

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

## Deploying to AWS

`deploy/aws/` contains scripts that deploy the whole thing to AWS (S3 +
CloudFront for the frontend, EC2 for the Go backend, ElastiCache for Redis):

```bash
bash deploy/aws/deploy.sh      # first-time setup (about 15 min)
bash deploy/aws/redeploy.sh    # push code changes afterwards
bash deploy/aws/teardown.sh    # delete everything
```

See `deploy/aws/DEFENSE_GUIDE.md` for the architecture, security choices and
likely examiner questions.

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
- `weather.go` — the live Open-Meteo fetch (the "slow source" being cached)
- `resp.go` — the minimal hand-rolled Redis client (SET with TTL, GET, DEL)
- `go.mod` — module file (no external dependencies)
- `frontend/` — Vite vanilla-JS app: buttons, live results, request history bars

# devops-redis-cache
