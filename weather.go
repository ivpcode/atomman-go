package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"
)

type weatherData struct {
	Code int    // codice icona 1-40
	Lo   int    // temperatura minima
	Hi   int    // temperatura massima
	Zone string // città
	Desc string // descrizione (es. "cielo sereno")
}

var weatherCache struct {
	mu      sync.Mutex
	data    *weatherData
	ts      time.Time
	warnedNoKey bool
}

// GetWeather restituisce i dati meteo (dal cache se recenti).
func GetWeather(cfg *Config) *weatherData {
	weatherCache.mu.Lock()
	defer weatherCache.mu.Unlock()

	key := cfg.Weather.APIKey
	if key == "" {
		if !weatherCache.warnedNoKey {
			fmt.Println("[Weather] Nessuna API key OpenWeather — campi meteo vuoti. Aggiungila in config.yaml.")
			weatherCache.warnedNoKey = true
		}
		return nil
	}

	refresh := time.Duration(cfg.Weather.RefreshSeconds) * time.Second
	if weatherCache.data != nil && time.Since(weatherCache.ts) < refresh {
		return weatherCache.data
	}

	data := fetchWeather(cfg, key)
	weatherCache.data = data
	weatherCache.ts = time.Now()
	return data
}

func fetchWeather(cfg *Config, key string) *weatherData {
	if !internetOK() {
		return nil
	}

	lat, lon, zone, err := resolveLocation(cfg.Weather.Location, key)
	if err != nil {
		return nil
	}

	owData, err := callOneCall(lat, lon, key, cfg.Weather.Units, cfg.Weather.Lang)
	if err != nil {
		return nil
	}

	cur := owData["current"].(map[string]interface{})
	daily := owData["daily"].([]interface{})
	if len(daily) == 0 {
		return nil
	}
	d0 := daily[0].(map[string]interface{})

	wx := cur["weather"].([]interface{})[0].(map[string]interface{})
	owID := int(wx["id"].(float64))
	icon := fmt.Sprintf("%v", wx["icon"])
	desc := fmt.Sprintf("%v", wx["description"])

	temps := d0["temp"].(map[string]interface{})
	lo := int(temps["min"].(float64))
	hi := int(temps["max"].(float64))

	return &weatherData{
		Code: mapWeatherCode(owID, icon),
		Lo:   lo,
		Hi:   hi,
		Zone: zone,
		Desc: desc,
	}
}

func internetOK() bool {
	conn, err := net.DialTimeout("udp", "8.8.8.8:53", 1500*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func resolveLocation(loc, key string) (lat, lon float64, zone string, err error) {
	// Prova lat,lon diretta
	var a, b float64
	if n, _ := fmt.Sscanf(loc, "%f,%f", &a, &b); n == 2 {
		return a, b, loc, nil
	}
	// Geocoding per nome città
	q := url.QueryEscape(loc)
	apiURL := fmt.Sprintf("https://api.openweathermap.org/geo/1.0/direct?q=%s&limit=1&appid=%s", q, key)
	var results []map[string]interface{}
	if err = apiGet(apiURL, &results); err != nil || len(results) == 0 {
		return 0, 0, "", fmt.Errorf("geocoding fallito per %q", loc)
	}
	r := results[0]
	lat = r["lat"].(float64)
	lon = r["lon"].(float64)
	name := fmt.Sprintf("%v", r["name"])
	cc := fmt.Sprintf("%v", r["country"])
	return lat, lon, fmt.Sprintf("%s,%s", name, cc), nil
}

func callOneCall(lat, lon float64, key, units, lang string) (map[string]interface{}, error) {
	apiURL := fmt.Sprintf(
		"https://api.openweathermap.org/data/3.0/onecall?lat=%f&lon=%f&units=%s&lang=%s&exclude=minutely,hourly,alerts&appid=%s",
		lat, lon, units, lang, key,
	)
	var result map[string]interface{}
	return result, apiGet(apiURL, &result)
}

func apiGet(rawURL string, dest interface{}) error {
	client := &http.Client{Timeout: 7 * time.Second}
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "AtomMan-Go/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(dest)
}

// mapWeatherCode converte l'ID OpenWeather nel codice icona del display (1-40).
func mapWeatherCode(owID int, icon string) int {
	day := len(icon) > 0 && icon[len(icon)-1] == 'd'
	if owID == 800 {
		if day {
			return 1
		}
		return 3
	}
	if owID == 801 {
		if day {
			return 5
		}
		return 6
	}
	if owID == 802 {
		if day {
			return 7
		}
		return 8
	}
	if owID >= 803 {
		return 9
	}
	g := owID / 100
	switch g {
	case 2: // temporale
		if owID == 202 || owID == 212 || owID == 232 {
			return 16
		}
		return 11
	case 3: // pioggerella
		return 13
	case 5:
		switch {
		case owID == 511:
			return 19
		case owID >= 520:
			return 10
		case owID == 500:
			return 13
		case owID == 501:
			return 14
		default:
			return 15
		}
	case 6: // neve
		switch owID {
		case 600:
			return 22
		case 601:
			return 23
		case 602, 621, 622:
			return 24
		case 620:
			return 21
		default:
			return 20
		}
	case 7: // atmosfera
		switch owID {
		case 701, 741:
			return 30
		case 711, 721:
			return 31
		case 731, 751:
			return 27
		case 781:
			return 36
		default:
			return 31
		}
	}
	return 99
}
