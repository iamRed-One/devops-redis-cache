# cache-demo — Frontend Design Brief

## What this page is

A single-screen demo comparing two ways of serving the same "get profile" request: always hitting the database (`GET /profile/no-cache`) versus checking Redis first (`GET /profile/cached`). The whole point of the UI is to make the latency gap between the two impossible to miss, then ground it with the real profile record being fetched.

## Required layout (as specified)

1. **Split view, top of page.** Two panels side by side: **No-Cache on the left, Cached on the right.**
2. **Time is the hero.** Each panel's `duration_ms` renders bold and large — the single biggest thing in its panel.
3. **Profile at the bottom.** One card below the split view showing the fetched profile record.

## Design system to build from

Grounded in the attached `DESIGN.md` (a dark, financial-platform-style system): a near-black canvas, flat surfaces with hairline borders (no shadows, no gradients), a single reserved accent color for brand-claim moments, and green/red used strictly as *text-color* signals for direction — never as a background fill.

### Colors

| Token | Hex | Use |
|---|---|---|
| Canvas | `#0b0e11` | Page background |
| Surface (card) | `#1e2329` | Card backgrounds |
| Surface elevated | `#2b3139` | Icon chips, meta chips, secondary buttons |
| Hairline | `#2b3139` | 1px borders on every card |
| Primary accent | `#fcd535` | Reserved — brand mark, the one "value-claim" number (speed-up stat), primary pill |
| Primary active | `#f0b90b` | Press state of the accent |
| On-dark (white) | `#ffffff` | Headings |
| Body | `#eaecef` | Default text |
| Muted | `#707a8a` | Captions, secondary labels |
| Muted-strong | `#929aa5` | Slightly stronger secondary text |
| Trading up (green) | `#0ecb81` | Cached panel's number + accents — text color only |
| Trading down (red) | `#f6465d` | No-Cache panel's number + accents — text color only |

Rule to hold the line on: red/green never become a card or button *background*. They're the number itself, the icon stroke, and small labels — that's what keeps them reading as signal rather than decoration.

### Typography

- **Copy typeface:** Inter (open-source stand-in for the system's BinanceNova), weights 400/500/600/700.
- **Number typeface:** JetBrains Mono (stand-in for BinancePlex) — every number on the page uses it: the two big durations, the speed-up multiplier, `duration_ms` chips. This is a hard rule in the source system: numbers never render in the copy typeface.
- Scale to use: hero page title 28–32px/600, panel duration numbers 64–88px/700 (this is intentionally larger than the source system's own number scale, since "bold and large" was explicit), panel labels 12px/600 with slight letter-spacing, body copy 13–14px/400.

### Spacing & shape

- 4px-based spacing scale. Card padding 24–32px, gap between the two split panels 20–24px.
- Radius: 6px for buttons/small chips, 8px for icon chips, 12px for the two main cards and the profile card, 9999px (pill) for badges and the avatar.

## Page structure, top to bottom

1. **Nav bar** (64px, canvas background, hairline bottom border): small accent-colored logo mark + "cache-demo" wordmark on the left; a backend-URL field on the right (this demo needs to point at wherever the Go server is running).
2. **Hero line:** short H1 ("No-Cache vs Cached") + one sentence of explanation.
3. **Split view** — two cards, equal width:
   - **No-Cache card:** database icon (red stroke), label "NO CACHE" + route `GET /profile/no-cache`, one line of description, the big red duration number with an "ms" unit, a caption ("flat, every single request"), a hairline divider, then small chips echoing the actual response fields — `source: db`, `duration_ms: …` — and a "Fetch" button.
   - **Cached card:** same anatomy, mirrored — bolt icon (green stroke), label "CACHED" + route `GET /profile/cached`, green duration number, caption ("served from cache" / "cache cleared — next fetch is a miss"), chips, "Fetch" button.
4. **Speed-up stat:** a single centered line using the one reserved accent color — "`Nx` faster with a warm cache" — computed live once both sides have been fetched at least once. Idle state before that: muted, "fetch both sides to compare."
5. **Reset control:** a plain secondary button next to the speed-up stat that clears the Redis key so the next "Cached" click is a miss again.
6. **Profile card** (bottom): avatar circle with initials, name + a small accent pill for the brand, role + location (with a pin icon), a row of stack chips, and the bio line. This is the literal `Profile` struct the backend returns — `name, brand, role, location, stack[], bio` — so every field earns a spot; nothing here is invented.

## Content is real, not placeholder

The sample record the backend returns (simulated fetch, and the same row seeded into Supabase per the README) is:

- Name: Ridwan Ambali
- Brand: Code Red
- Role: Full Stack & Mobile Developer
- Location: Abuja, Nigeria
- Stack: React Native, Flutter, Laravel, Django Channels, Go
- Bio: "300-level CS student at AFIT Kaduna, runs Jurvclaq Global Concepts, freelances on Upwork & Fiverr."

Typical numbers to design around: ~180ms flat for no-cache, ~2–5ms for a cache hit, 30s TTL between misses — so roughly a 40–90x speed-up is the honest range for that stat, not a fixed number.

## States to design for

- **Before any fetch:** both duration slots show a dash/placeholder, not a fabricated number; the profile card can still show the known sample record since it's static.
- **After a fetch:** duration fills in, source chip updates, speed-up stat activates once both sides have a value.
- **Cache reset:** cached panel's number/chips revert to placeholder, caption explains why.
- **Backend unreachable:** an inline error message in the panel that failed (small, red), not a full-page error.

## Responsive

Below ~720px, stack the two split-view cards instead of placing them side by side; everything else (nav, profile) already reads fine as a single column.

## Reference

I also built a static visual mockup of this exact layout on Claude's design canvas, in case it's useful as a side-by-side reference while you build: https://claude.ai/code/artifact/c1e9f5dc-26d9-4c9f-82ac-e4a78cc13cb7
