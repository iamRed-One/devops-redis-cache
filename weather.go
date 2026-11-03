package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Weather is the "record" this demo fetches: live conditions for one city,
// pulled from Open-Meteo (https://open-meteo.com), a free public weather API
// that needs no API key. It stands in for any slow third-party dependency
// (payment gateway, maps API, a rate-limited service): the call crosses the
// internet, so every request pays the full round trip, which is exactly the
// kind of cost a cache removes.
type Weather struct {
	City       string  `json:"city"`
	Country    string  `json:"country"`
	Condition  string  `json:"condition"`
	TempC      float64 `json:"temp_c"`
	Humidity   int     `json:"humidity_pct"`
	WindKmh    float64 `json:"wind_kmh"`
	ObservedAt string  `json:"observed_at"` // timestamp reported by Open-Meteo (local time)
	FetchedAt  string  `json:"fetched_at"`  // when OUR server called Open-Meteo (UTC)
}

// Kaduna, Nigeria (home of AFIT).
const (
	cityName    = "Kaduna"
	countryName = "Nigeria"
	cityLat     = "10.52"
	cityLon     = "7.44"
)

const weatherURL = "https://api.open-meteo.com/v1/forecast?latitude=" + cityLat + "&longitude=" + cityLon +
	"&current=temperature_2m,relative_humidity_2m,wind_speed_10m,weather_code&timezone=Africa%2FLagos"

// A short timeout so a slow upstream fails fast instead of hanging the demo.
var weatherClient = &http.Client{Timeout: 8 * time.Second}

type openMeteoResponse struct {
	Current struct {
		Time        string  `json:"time"`
		Temperature float64 `json:"temperature_2m"`
		Humidity    int     `json:"relative_humidity_2m"`
		Wind        float64 `json:"wind_speed_10m"`
		Code        int     `json:"weather_code"`
	} `json:"current"`
}

// dbFetch is the single place that goes to "the slow source". The handlers in
// main.go call this on every no-cache request and on cache misses only.
func dbFetch() (Weather, error) {
	resp, err := weatherClient.Get(weatherURL)
	if err != nil {
		return Weather{}, fmt.Errorf("calling open-meteo: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Weather{}, fmt.Errorf("open-meteo returned HTTP %d", resp.StatusCode)
	}

	var body openMeteoResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return Weather{}, fmt.Errorf("decoding open-meteo response: %w", err)
	}

	return Weather{
		City:       cityName,
		Country:    countryName,
		Condition:  describeWeatherCode(body.Current.Code),
		TempC:      body.Current.Temperature,
		Humidity:   body.Current.Humidity,
		WindKmh:    body.Current.Wind,
		ObservedAt: body.Current.Time,
		FetchedAt:  time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// describeWeatherCode maps WMO weather interpretation codes to plain words.
func describeWeatherCode(code int) string {
	switch {
	case code == 0:
		return "Clear sky"
	case code == 1 || code == 2:
		return "Mostly clear"
	case code == 3:
		return "Overcast"
	case code == 45 || code == 48:
		return "Fog"
	case code >= 51 && code <= 57:
		return "Drizzle"
	case code >= 61 && code <= 67:
		return "Rain"
	case code >= 71 && code <= 77:
		return "Snow"
	case code >= 80 && code <= 82:
		return "Rain showers"
	case code >= 95:
		return "Thunderstorm"
	default:
		return "Unknown"
	}
}
