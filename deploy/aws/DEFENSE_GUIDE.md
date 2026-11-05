# cache-demo on AWS: explanation guide

Use this to present and defend the project. Numbers below were measured on the
live deployment (region `eu-west-1`, Ireland).

## 1. The one-sentence story

> A web app shows live weather for Kaduna, pulled from a public internet API.
> Without a cache every request makes that slow internet call. With a cache,
> the first request fills Redis and every request after that is answered from
> memory in about 2 ms, until the entry expires after 30 seconds.

## 2. Architecture

```
                    HTTPS
 Browser  ───────────────────►  CloudFront (CDN, AWS edge)
                                   │
              static files         │          /weather/*  and  /reset
         ┌─────────────────────────┴───────────────────────────┐
         ▼                                                     ▼
   S3 bucket (private)                              EC2 t4g.micro  :8080
   index.html, JS, CSS                              Go binary (systemd service)
                                                       │              │
                                          cache HIT    │              │  cache MISS
                                          ~2 ms        ▼              ▼
                                              ElastiCache Redis    Open-Meteo API
                                              (same VPC, :6379)    (internet call)
```

| Piece | Service | Why this and not something else |
|---|---|---|
| Frontend | S3 + CloudFront | Static files need no server. S3 is cheap storage, CloudFront adds HTTPS and caches the files near users. |
| Backend | EC2 `t4g.micro` | ElastiCache can only be reached from inside a VPC. A plain VM in the VPC is the simplest way to be inside it and still reach the internet. Lambda in a VPC would need a NAT gateway, which is costly. |
| Cache | ElastiCache Redis 7.1 | Managed Redis: AWS patches it and monitors it. Lives in the same VPC as the backend, so a round trip is well under 1 ms. |
| One URL for both | CloudFront routes by path | `/weather/*` and `/reset` go to EC2, everything else goes to S3. The browser sees one HTTPS origin, so there are no CORS errors and no mixed-content blocking. |

## 3. What happens on a request

1. The browser calls `https://<cloudfront>/weather/cached`.
2. CloudFront forwards it to the EC2 backend (API paths are **not** cached by CloudFront, so the demo measures Redis and not the CDN).
3. The Go server runs `GET weather:kaduna` against Redis.
   - **Hit:** it returns the stored JSON. `source: "cache"`, about 2 ms.
   - **Miss:** it calls the Open-Meteo weather API, runs `SET weather:kaduna ... EX 30`, and returns it. `source: "db"` (the slow source).
4. `/weather/no-cache` skips Redis and always goes to the data source.
5. `/reset` deletes the key so you can show a miss again on demand.

This pattern is called **cache-aside** (also "lazy loading"): the application
checks the cache first and fills it on a miss.

## 4. Measured results

| Request | Result |
|---|---|
| No cache, first call (cold connection) | about 157 ms |
| No cache, repeated | 25 to 27 ms (real call to Open-Meteo from Ireland) |
| Cached, first call (miss) | about 31 ms |
| Cached, later calls (hit) | 1.6 to 2.0 ms |

That is roughly **15x faster** on a warm connection. The no-cache time depends
on the upstream API and the network, so it varies from run to run. If asked why
it is not 100x: the weather API is fast and nearby (Europe). The slower the
source, the bigger the win, which is the point of caching.

`duration_ms` is measured inside the Go server, so it excludes the network
trip from the browser. That is deliberate: it isolates the cost of the data
fetch itself.

## 5. Security choices (say these out loud, examiners like them)

- **S3 is private.** Only this CloudFront distribution can read it (Origin Access Control). Opening the S3 URL directly returns 403.
- **The backend port only accepts CloudFront.** The EC2 security group allows port 8080 from CloudFront's published IP ranges only. There is no SSH port at all.
- **Redis only accepts the backend.** The cache security group allows port 6379 from the backend's security group and from nothing else.
- **No keys in code.** The EC2 box gets S3 access through an IAM role, not stored credentials.
- **Known limitations** (be upfront): Redis traffic inside the VPC is not TLS-encrypted, and CloudFront to EC2 is plain HTTP. Both are acceptable for a demo and would be hardened for production.

## 6. Cost and cleanup

- Roughly **$0.50 per day** while running (ElastiCache node, EC2, public IPv4). It is drawn from the AWS credit balance.
- A **budget alert** emails at $10 and $25.
- **Delete everything:** `bash deploy/aws/teardown.sh` (CloudFront takes about 10 minutes to delete).

## 7. Live demo script (about 2 minutes)

1. Open the CloudFront URL.
2. Click **Fetch: No Cache** three times. The number stays around 25 to 30 ms (the first call can be slower).
3. Click **Reset Cache**, then **Fetch: Cached**. The first call is about 30 ms (miss).
4. Click **Fetch: Cached** again. About 2 ms (hit).
5. Say: "Same live data, same code path. The only difference is Redis."

## 8. Questions you may be asked

**Why not just cache in the application's memory?**
In-process memory is lost on restart and is not shared if you run more than one
server. Redis is a separate shared store, so every server sees the same cache.

**What is the TTL and why 30 seconds?**
Time to live: the entry deletes itself after 30 s. It bounds how stale data can
get. Short for a demo; for real data you pick it based on how fast the data
changes.

**What if Redis goes down?**
The handler treats a Redis error like a miss and falls back to the data source,
so the app stays up, only slower.

**Is this highly available?**
No. It is one cache node and one server. For production you would add a replica
with automatic failover and put the servers behind a load balancer.

**Why ElastiCache and not Upstash or self-hosted Redis?**
Upstash is reached over the internet, so every hit pays an internet round trip.
ElastiCache sits next to the backend in the same VPC. Self-hosting means you
patch, back up and monitor it yourself.

**What is a cache miss storm / stampede?**
When a hot key expires and many requests miss at once, they all hit the data
source together. Mitigations: jittered TTLs, request coalescing, or refreshing
before expiry. Not implemented here.

**How is the infrastructure created?**
With the AWS CLI in `deploy/aws/deploy.sh`. It is scripted so it can be
repeated and torn down exactly. The production-grade version of this idea is
infrastructure as code (CloudFormation or Terraform).

## 9. Files

- `deploy.sh`: creates everything, safe to re-run
- `teardown.sh`: deletes everything
- `.state` (git-ignored): IDs of the created resources
