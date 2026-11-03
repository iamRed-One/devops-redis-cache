// cache-demo compares two ways of serving the same request: "current weather
// in Kaduna", fetched live from the public Open-Meteo API.
//
//	GET /weather/no-cache   - always calls the upstream API
//	GET /weather/cached     - checks Redis first, only calls the API on a
//	                          cache miss, then stores the result for 30s
//
// Hit each endpoint a few times in a row and compare the duration_ms field in
// the response. The no-cache path pays the full internet round trip every
// time. The cached path pays it once (the miss) and then answers from Redis
// in low single-digit milliseconds until the 30s TTL expires. The fetched_at
// field in the payload shows when the data was really pulled, so you can see
// the cached copy getting older.
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"
)

type response struct {
	Source     string  `json:"source"`      // "db" or "cache"
	DurationMs float64 `json:"duration_ms"` // measured server-side, on this request
	Data       Weather `json:"data"`
}

const cacheKey = "weather:kaduna"
const cacheTTLSeconds = 30

func main() {
	redis := NewRedisClient()

	http.HandleFunc("/weather/no-cache", withCORS(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		data, err := dbFetch()
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, response{
			Source:     "db",
			DurationMs: msSince(start),
			Data:       data,
		})
	}))

	http.HandleFunc("/weather/cached", withCORS(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		if raw, hit, err := redis.Get(cacheKey); err == nil && hit {
			var data Weather
			if err := json.Unmarshal([]byte(raw), &data); err == nil {
				writeJSON(w, response{
					Source:     "cache",
					DurationMs: msSince(start),
					Data:       data,
				})
				return
			}
		}

		// cache miss (or Redis hiccup) - fall back to the live API
		data, err := dbFetch()
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		if raw, err := json.Marshal(data); err == nil {
			if err := redis.Set(cacheKey, string(raw), cacheTTLSeconds); err != nil {
				log.Printf("warning: could not write to cache: %v", err)
			}
		}
		writeJSON(w, response{
			Source:     "db",
			DurationMs: msSince(start),
			Data:       data,
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

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080" // local dev default; Render (and most PaaS hosts) inject PORT
	}
	addr := ":" + port
	log.Printf("cache-demo listening on %s", addr)
	log.Printf("  data source: Open-Meteo live weather for %s", cityName)
	log.Printf("  GET  /weather/no-cache  -> always calls the weather API")
	log.Printf("  GET  /weather/cached    -> first hit calls the API, next hits (within %ds) are served from Redis", cacheTTLSeconds)
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
