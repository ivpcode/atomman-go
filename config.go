package main

import (
	"os"

	"gopkg.in/yaml.v3"
)

// Config è la struttura completa della configurazione.
type Config struct {
	Serial struct {
		Port       string  `yaml:"port"`
		Baud       int     `yaml:"baud"`
		StartDelay float64 `yaml:"start_delay"`
		DSRDTR     bool    `yaml:"dsrdtr"`
		RTSCTS     bool    `yaml:"rtscts"`
	} `yaml:"serial"`

	// Tiles: lista dei tile da mostrare in rotazione.
	// Valori disponibili: cpu, gpu, memory, disk, datetime, network, volume, battery
	Tiles []string `yaml:"tiles"`

	// CustomLabels: sovrascrive i nomi rilevati automaticamente.
	CustomLabels struct {
		CPU    string `yaml:"cpu"`
		GPU    string `yaml:"gpu"`
		Memory string `yaml:"memory"`
		Disk   string `yaml:"disk"`
	} `yaml:"custom_labels"`

	// Weather: integrazione OpenWeather API (opzionale).
	Weather struct {
		APIKey         string `yaml:"api_key"`
		Location       string `yaml:"location"`
		Units          string `yaml:"units"` // metric | imperial
		Lang           string `yaml:"lang"`
		RefreshSeconds int    `yaml:"refresh_seconds"`
	} `yaml:"weather"`

	Fan struct {
		Prefer string `yaml:"prefer"` // auto | hwmon | nvidia
		MaxRPM int    `yaml:"max_rpm"` // usato solo con NVIDIA (RPM = % * max_rpm / 100)
	} `yaml:"fan"`

	Network struct {
		Interface string `yaml:"interface"` // vuoto = auto-detect
	} `yaml:"network"`

	Unlock struct {
		Attempts      int     `yaml:"attempts"`
		WindowSeconds float64 `yaml:"window_seconds"`
	} `yaml:"unlock"`

	Dashboard struct {
		Enabled       bool `yaml:"enabled"`
		NoColor       bool `yaml:"no_color"`
		RefreshMillis int  `yaml:"refresh_millis"`
	} `yaml:"dashboard"`

}

func defaultConfig() *Config {
	cfg := &Config{}
	cfg.Serial.Port = "/dev/ttyACM0"
	cfg.Serial.Baud = 115200
	cfg.Serial.StartDelay = 5.0
	cfg.Serial.DSRDTR = true
	cfg.Serial.RTSCTS = false

	cfg.Tiles = []string{
		"cpu", "gpu", "memory", "disk",
		"datetime", "network", "volume", "battery",
	}

	cfg.Weather.Units = "metric"
	cfg.Weather.Lang = "it"
	cfg.Weather.RefreshSeconds = 600

	cfg.Fan.Prefer = "auto"
	cfg.Fan.MaxRPM = 2000

	cfg.Unlock.Attempts = 3
	cfg.Unlock.WindowSeconds = 5.0

	cfg.Dashboard.RefreshMillis = 500
	return cfg
}

func loadConfig(path string) (*Config, error) {
	cfg := defaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}
	return cfg, yaml.Unmarshal(data, cfg)
}
