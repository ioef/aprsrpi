package config

import (
	"encoding/json"
	"fmt"
	"os"
)

type Config struct {
	HTTPAddress string           `json:"httpAddress"`
	WebRoot     string           `json:"webRoot"`
	LogFile     string           `json:"logFile"`
	LogLevel    string           `json:"logLevel"`
	KISS        KISSConfig       `json:"kiss"`
	Bot         BotConfig        `json:"bot"`
	APRSIS      APRSISConfig     `json:"aprsIs"`
	Station     StationConfig    `json:"station"`
	IGate       IGateConfig      `json:"igate"`
	Digipeater  DigipeaterConfig `json:"digipeater"`
}
type APRSISConfig struct {
	Enabled  bool   `json:"enabled"`
	Server   string `json:"server"`
	Callsign string `json:"callsign"`
	Passcode string `json:"passcode"`
	Filter   string `json:"filter"`
}
type StationConfig struct {
	Callsign      string  `json:"callsign"`
	Latitude      float64 `json:"latitude"`
	Longitude     float64 `json:"longitude"`
	SymbolTable   string  `json:"symbolTable"`
	SymbolCode    string  `json:"symbolCode"`
	Comment       string  `json:"comment"`
	BeaconMinutes int     `json:"beaconMinutes"`
}
type IGateConfig struct {
	Enabled             bool   `json:"enabled"`
	MessageGate         bool   `json:"messageGate"`
	HeardTimeoutMinutes int    `json:"heardTimeoutMinutes"`
	RFFilter            string `json:"rfFilter"`
	MaxPerMinute        int    `json:"maxPerMinute"`
	MaxPerFiveMinutes   int    `json:"maxPerFiveMinutes"`
}
type DigipeaterConfig struct {
	Enabled           bool     `json:"enabled"`
	Callsign          string   `json:"callsign"`
	Aliases           []string `json:"aliases"`
	MaxHops           int      `json:"maxHops"`
	RateLimitSeconds  int      `json:"rateLimitSeconds"`
	MaxPerMinute      int      `json:"maxPerMinute"`
	MaxPerFiveMinutes int      `json:"maxPerFiveMinutes"`
}

type KISSConfig struct {
	Endpoint string `json:"endpoint"`
	Baud     int    `json:"baud"`
}
type BotConfig struct {
	Callsign          string `json:"callsign"`
	Location          string `json:"location"`
	WeatherCity       string `json:"weatherCity"`
	Repeaters         string `json:"repeaters"`
	Sunrise           string `json:"sunrise"`
	Sunset            string `json:"sunset"`
	OpenWeatherAPIKey string `json:"openWeatherApiKey"`
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config %s: %w", path, err)
	}
	var value Config
	if err := json.Unmarshal(data, &value); err != nil {
		return Config{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	return value, nil
}

func WithDefaults(value Config) Config {
	if value.HTTPAddress == "" {
		value.HTTPAddress = ":8080"
	}
	if value.WebRoot == "" {
		value.WebRoot = "web/dist"
	}
	if value.LogFile == "" {
		value.LogFile = "/var/log/aprsrpi/aprsrpi.log"
	}
	if value.KISS.Endpoint == "" {
		value.KISS.Endpoint = "tcp://127.0.0.1:8001"
	}
	if value.KISS.Baud == 0 {
		value.KISS.Baud = 9600
	}
	if value.Bot.Callsign == "" {
		value.Bot.Callsign = "SV2JLD"
	}
	if value.Bot.WeatherCity == "" {
		value.Bot.WeatherCity = "Thessaloniki"
	}
	if value.APRSIS.Callsign == "" {
		value.APRSIS.Callsign = value.Bot.Callsign
	}
	if value.Station.Callsign == "" {
		value.Station.Callsign = value.APRSIS.Callsign
	}
	if value.Station.SymbolTable == "" {
		value.Station.SymbolTable = "/"
	}
	if value.Station.SymbolCode == "" {
		value.Station.SymbolCode = "&"
	}
	if value.Station.BeaconMinutes == 0 {
		value.Station.BeaconMinutes = 30
	}
	if value.APRSIS.Passcode == "" {
		value.APRSIS.Passcode = "-1"
	}
	if value.IGate.HeardTimeoutMinutes == 0 {
		value.IGate.HeardTimeoutMinutes = 30
	}
	if value.IGate.MaxPerMinute == 0 {
		value.IGate.MaxPerMinute = 6
	}
	if value.IGate.MaxPerFiveMinutes == 0 {
		value.IGate.MaxPerFiveMinutes = 20
	}
	if value.Digipeater.Callsign == "" {
		value.Digipeater.Callsign = value.Bot.Callsign
	}
	if len(value.Digipeater.Aliases) == 0 {
		value.Digipeater.Aliases = []string{"WIDE1-1", "WIDE2-1", "WIDE2-2"}
	}
	if value.Digipeater.MaxHops == 0 {
		value.Digipeater.MaxHops = 2
	}
	if value.Digipeater.RateLimitSeconds == 0 {
		value.Digipeater.RateLimitSeconds = 10
	}
	if value.Digipeater.MaxPerMinute == 0 {
		value.Digipeater.MaxPerMinute = 20
	}
	if value.Digipeater.MaxPerFiveMinutes == 0 {
		value.Digipeater.MaxPerFiveMinutes = 80
	}
	return value
}
