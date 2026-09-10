// cache-demo compares two ways of serving the same "get profile" request:
//
//   GET /profile/no-cache   - always hits the "database" (simulated, or real Supabase)
//   GET /profile/cached     - checks Redis first, only hits the "database"
//                             on a cache miss, then stores the result
//
// Hit each endpoint a few times in a row (curl, the frontend, whatever) and
// compare the duration_ms field in the response. The no-cache path stays
// roughly flat every single time (the cost of a real query, or the
// simulated 180ms). The cached path is that same cost on the first request
// (a miss) and drops to low single-digit milliseconds on every request
// after that, until the 30s TTL expires and it repeats.
//
// The "database" is simulated by default (see simulateDBFetch below) so
// this runs with zero setup. Set SUPABASE_URL and SUPABASE_ANON_KEY to
// point it at a real Supabase table instead — see README.md.
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"
)

// Profile is the "record" this demo fetches. Standing in for a row in a
// real users/profiles table. Field order/names match the Supabase table
// columns 1:1 so the same struct decodes either source.
type Profile struct {
	Name     string   `json:"name"`
	Brand    string   `json:"brand"`
	Role     string   `json:"role"`
	Location string   `json:"location"`
	Stack    []string `json:"stack"`
	Bio      string   `json:"bio"`
}

// simulateDBFetch stands in for a real database round trip: a network hop
// to the DB, a query plan, disk/page cache reads, serialization. 180ms is
// on the pessimistic side of "simple query on a warm connection pool" —
// tune it if you want to model something snappier or slower. Only used
// when Supabase isn't configured (see dbFetch in supabase.go).
func simulateDBFetch() Profile {
	time.Sleep(180 * time.Millisecond)
	return Profile{
		Name:     "Ridwan Ambali",
		Brand:    "Code Red",
		Role:     "Full Stack & Mobile Developer",
		Location: "Abuja, Nigeria",
		Stack:    []string{"React Native", "Flutter", "Laravel", "Django Channels", "Go"},
		Bio:      "300-level CS student at AFIT Kaduna, runs Jurvclaq Global Concepts, freelances on Upwork & Fiverr.",
	}
}

type response struct {
	Source     string  `json:"source"`      // "db" or "cache"
	DurationMs float64 `json:"duration_ms"` // measured server-side, on this request
	Profile    Profile `json:"profile"`
}

const cacheKey = "profile:demo"
const cacheTTLSeconds = 30

func main() {
	redis := NewRedisClient("localhost:6379")

	http.HandleFunc("/profile/no-cache", withCORS(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		profile, err := dbFetch()
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, response{
			Source:     "db",
			DurationMs: msSince(start),
			Profile:    profile,
		})
	}))

	http.HandleFunc("/profile/cached", withCORS(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		if raw, hit, err := redis.Get(cacheKey); err == nil && hit {
			var profile Profile
			if err := json.Unmarshal([]byte(raw), &profile); err == nil {
				writeJSON(w, response{
					Source:     "cache",
					DurationMs: msSince(start),
					Profile:    profile,
				})
				return
			}
		}

		// cache miss (or Redis hiccup) - fall back to the "database"
		profile, err := dbFetch()
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		if raw, err := json.Marshal(profile); err == nil {
			if err := redis.Set(cacheKey, string(raw), cacheTTLSeconds); err != nil {
				log.Printf("warning: could not write to cache: %v", err)
			}
		}
		writeJSON(w, response{
			Source:     "db",
			DurationMs: msSince(start),
			Profile:    profile,
		})
	}))

	// lets you re-trigger a cache miss on demand instead of waiting out the TTL
	http.HandleFunc("/reset", withCORS(func(w http.ResponseWriter, r *http.Request) {
		if err := redis.Del(cacheKey); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Write([]byte("cache cleared\n"))
	}))

	addr := ":8080"
	log.Printf("cache-demo listening on %s", addr)
	if useSupabase {
		log.Printf("  data source: Supabase (SUPABASE_URL is set)")
	} else {
		log.Printf("  data source: simulated (~180ms) — set SUPABASE_URL + SUPABASE_ANON_KEY to use a real table")
	}
	log.Printf("  GET  /profile/no-cache  -> always hits the database")
	log.Printf("  GET  /profile/cached    -> first hit is a DB fetch, next hits (within %ds) are served from Redis", cacheTTLSeconds)
	log.Printf("  GET  /reset             -> clears the cache so you can re-demo the miss")
	log.Fatal(http.ListenAndServe(addr, nil))
}

// withCORS lets the Vite dev server (a different origin/port) call this API
// directly from the browser. Wide open (Access-Control-Allow-Origin: *) is
// fine for a demo you're presenting; tighten it to your actual frontend
// origin before leaving it running anywhere long-term.
func withCORS(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h(w, r)
	}
}

func msSince(start time.Time) float64 {
	return float64(time.Since(start).Microseconds()) / 1000.0
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}
