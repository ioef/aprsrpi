package bot

import (
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"aprsrpi/internal/aprs"
)

type Config struct{ Callsign, Location, WeatherCity, Repeaters, Sunrise, Sunset, OpenWeatherAPIKey string }

func Handle(writer io.Writer, message aprs.Message, config Config) error {
	target, command, ok := addressedMessage(message.Payload)
	if !ok || !strings.EqualFold(target, config.Callsign) {
		return nil
	}
	command = strings.TrimSpace(command)
	upper := strings.ToUpper(command)
	if match := regexp.MustCompile(`\{(\d+)\s*$`).FindStringSubmatch(command); len(match) == 2 {
		if _, err := writer.Write(aprs.EncodeMessage(message.Source, config.Callsign, "ack"+match[1])); err != nil {
			return err
		}
		command = strings.TrimSpace(strings.TrimSuffix(command, match[0]))
		upper = strings.ToUpper(command)
	}
	response := commandResponse(command, upper, config)
	return writeResponse(writer, message.Source, config.Callsign, response)
}

func addressedMessage(payload string) (string, string, bool) {
	if len(payload) < 11 || payload[0] != ':' || payload[10] != ':' {
		return "", "", false
	}
	return strings.TrimSpace(payload[1:10]), strings.TrimSpace(payload[11:]), true
}
func writeResponse(writer io.Writer, destination, source, response string) error {
	if len(response) > 67 {
		response = response[:67]
	}
	_, err := writer.Write(aprs.EncodeMessage(destination, source, response))
	return err
}

func commandResponse(command, upper string, config Config) string {
	switch {
	case upper == "WHEREMAI" || upper == "WHEREAMI":
		return fallback(config.Location, "Your location: 40.7128 N, 74.0060 W") + " 73!"
	case upper == "ISS_LOCATION":
		return fetchISSLocation()
	case upper == "ISS_ASTROS":
		return fetchISSAstronauts()
	case upper == "SKGWEATHER":
		return fetchWeather(fallback(config.WeatherCity, "Thessaloniki"), config.OpenWeatherAPIKey)
	case strings.HasPrefix(upper, "WEATHER?"):
		city := strings.TrimSpace(command[len("WEATHER?"):])
		if city == "" {
			city = fallback(config.WeatherCity, "Thessaloniki")
		}
		return fetchWeather(city, config.OpenWeatherAPIKey)
	case upper == "QUAKE":
		return fetchEarthquakes("")
	case strings.HasPrefix(upper, "QUAKE?"):
		return fetchEarthquakes(strings.TrimSpace(command[len("QUAKE?"):]))
	case upper == "TIP":
		return tips[rand.Intn(len(tips))]
	case upper == "REPEATERS":
		return fallback(config.Repeaters, "SV2A 145.750, SV2D 145.675 88.5Hz, SV2O 145.600 94.8Hz")
	case upper == "SUNRISE":
		return fmt.Sprintf("Sunrise: %s Sunset: %s", fallback(config.Sunrise, "06:45"), fallback(config.Sunset, "20:35"))
	case upper == "BEACON":
		return "QSL. Signal received loud and clear! 73!"
	case upper == "HELP":
		return "Cmds: WEATHER?CITY, QUAKE, QUAKE?COUNTRY, ISS_LOCATION, HELP"
	default:
		return "Unknown command. Try HELP"
	}
}

var tips = []string{"Check your SWR before blaming the radio.", "Always identify your station on the air.", "Keep a logbook for every useful contact.", "Carry spare fuses for portable operation.", "A good antenna is the best first upgrade."}

func fallback(value, defaultValue string) string {
	if strings.TrimSpace(value) == "" {
		return defaultValue
	}
	return value
}

func fetchWeather(city, key string) string {
	if key == "" {
		return "Weather API key is not configured"
	}
	request, err := http.NewRequest(http.MethodGet, "https://api.openweathermap.org/data/2.5/weather?q="+url.QueryEscape(city)+"&appid="+url.QueryEscape(key)+"&units=metric", nil)
	if err != nil {
		return "Weather request failed"
	}
	client := http.Client{Timeout: 8 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return "Weather service unavailable"
	}
	defer response.Body.Close()
	var data struct {
		Main struct {
			Temp float64 `json:"temp"`
		} `json:"main"`
		Weather []struct {
			Description string `json:"description"`
		} `json:"weather"`
	}
	if response.StatusCode != http.StatusOK || json.NewDecoder(response.Body).Decode(&data) != nil || len(data.Weather) == 0 {
		return "Weather unavailable for " + city
	}
	return fmt.Sprintf("Weather in %s: %.1f C, %s", city, data.Main.Temp, data.Weather[0].Description)
}

var usgsEarthquakeURL = "https://earthquake.usgs.gov/fdsnws/event/1/query"

func fetchEarthquakes(country string) string {
	parameters := url.Values{"format": {"geojson"}, "orderby": {"time"}}
	boundedCountry := false
	if country == "" {
		parameters.Set("limit", "3")
	} else {
		if bounds, ok := earthquakeCountryBounds[strings.ToUpper(country)]; ok {
			boundedCountry = true
			parameters.Set("limit", "1")
			parameters.Set("minlatitude", strconv.FormatFloat(bounds.lowerLatitude, 'f', -1, 64))
			parameters.Set("maxlatitude", strconv.FormatFloat(bounds.upperLatitude, 'f', -1, 64))
			parameters.Set("minlongitude", strconv.FormatFloat(bounds.lowerLongitude, 'f', -1, 64))
			parameters.Set("maxlongitude", strconv.FormatFloat(bounds.upperLongitude, 'f', -1, 64))
		} else {
			parameters.Set("limit", "200")
		}
	}
	var data struct {
		Features []struct {
			Properties struct {
				Magnitude float64 `json:"mag"`
				Place     string  `json:"place"`
			} `json:"properties"`
		} `json:"features"`
	}
	if !getJSON(usgsEarthquakeURL+"?"+parameters.Encode(), &data) {
		return "Earthquake service unavailable"
	}
	if len(data.Features) == 0 {
		if country == "" {
			return "No recent earthquakes reported"
		}
		return "No recent earthquake found for " + country
	}
	if country != "" {
		quake := data.Features[0].Properties
		if !boundedCountry {
			found := false
			for _, feature := range data.Features {
				if strings.Contains(strings.ToUpper(feature.Properties.Place), strings.ToUpper(country)) {
					quake = feature.Properties
					found = true
					break
				}
			}
			if !found {
				return "No recent earthquake found for " + country
			}
		}
		return fmt.Sprintf("Latest %s quake: M%.1f %s", country, quake.Magnitude, quake.Place)
	}
	quakes := make([]string, 0, len(data.Features))
	for _, feature := range data.Features {
		quake := feature.Properties
		quakes = append(quakes, fmt.Sprintf("M%.1f %s", quake.Magnitude, quake.Place))
	}
	return "Latest quakes: " + strings.Join(quakes, "; ")
}

func fetchISSLocation() string {
	var data struct {
		Message  string `json:"message"`
		Position struct {
			Latitude  string `json:"latitude"`
			Longitude string `json:"longitude"`
		} `json:"iss_position"`
	}
	if !getJSON("https://api.open-notify.org/iss-now.json", &data) || data.Message != "success" {
		return "Could not retrieve ISS location"
	}
	return "ISS Location " + data.Position.Latitude + " N, " + data.Position.Longitude + " E"
}
func fetchISSAstronauts() string {
	var data struct {
		Message string `json:"message"`
		People  []struct {
			Name string `json:"name"`
		} `json:"people"`
	}
	if !getJSON("https://api.open-notify.org/astros.json", &data) || data.Message != "success" {
		return "Could not retrieve astronaut data"
	}
	names := make([]string, 0, len(data.People))
	for _, person := range data.People {
		names = append(names, person.Name)
	}
	return "In space: " + strings.Join(names, ", ")
}
func getJSON(endpoint string, target any) bool {
	client := http.Client{Timeout: 8 * time.Second}
	response, err := client.Get(endpoint)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	return response.StatusCode == http.StatusOK && json.NewDecoder(response.Body).Decode(target) == nil
}
