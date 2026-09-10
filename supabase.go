package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

// useSupabase is decided once at startup: if both env vars are present we
// hit the real Supabase table; otherwise we fall back to the simulated
// fetch, so the demo still runs with zero setup.
var useSupabase = os.Getenv("SUPABASE_URL") != "" && os.Getenv("SUPABASE_ANON_KEY") != ""

// fetchProfileFromSupabase reads a single row from the "profiles" table via
// Supabase's auto-generated REST API (PostgREST). That's just an HTTP GET
// with an API key header — no Postgres driver or extra Go dependency needed,
// which matters here since this environment can't reach the Go module proxy.
//
// Table expected (see README.md for the exact SQL):
//   profiles(name text, brand text, role text, location text,
//             stack text[], bio text)
func fetchProfileFromSupabase() (Profile, error) {
	baseURL := os.Getenv("SUPABASE_URL")
	key := os.Getenv("SUPABASE_ANON_KEY")

	endpoint := fmt.Sprintf("%s/rest/v1/profiles?select=*&limit=1", baseURL)
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return Profile{}, err
	}
	req.Header.Set("apikey", key)
	req.Header.Set("Authorization", "Bearer "+key)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Profile{}, fmt.Errorf("calling supabase: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Profile{}, fmt.Errorf("supabase returned HTTP %d (check SUPABASE_URL / SUPABASE_ANON_KEY and that the profiles table exists)", resp.StatusCode)
	}

	var rows []Profile
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		return Profile{}, fmt.Errorf("decoding supabase response: %w", err)
	}
	if len(rows) == 0 {
		return Profile{}, fmt.Errorf("profiles table returned no rows — insert one (see README.md)")
	}
	return rows[0], nil
}

// dbFetch is the single place that decides where "the database" actually
// is. Everything else in main.go just calls this and doesn't care whether
// the answer came from Supabase or the simulated fetch.
func dbFetch() (Profile, error) {
	if useSupabase {
		return fetchProfileFromSupabase()
	}
	return simulateDBFetch(), nil
}
