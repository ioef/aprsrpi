package aprs

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Message struct {
	ID          int64      `json:"id"`
	Received    string     `json:"received"`
	Source      string     `json:"source"`
	Destination string     `json:"destination"`
	Path        string     `json:"path"`
	Payload     string     `json:"payload"`
	Raw         string     `json:"raw"`
	Kind        string     `json:"kind"`
	Icon        string     `json:"icon"`
	Weather     *Weather   `json:"weather,omitempty"`
	Position    *Position  `json:"position,omitempty"`
	Symbol      string     `json:"symbol,omitempty"`
	IsMessage   bool       `json:"isMessage"`
	Type        string     `json:"type"`
	Telemetry   *Telemetry `json:"telemetry,omitempty"`
}

type Position struct {
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	SymbolTable string  `json:"symbolTable"`
	SymbolCode  string  `json:"symbolCode"`
}

type Telemetry struct {
	Sequence   int      `json:"sequence"`
	Analog     []int    `json:"analog"`
	Digital    string   `json:"digital"`
	Parameters []string `json:"parameters,omitempty"`
	Units      []string `json:"units,omitempty"`
	Bits       string   `json:"bits,omitempty"`
}

type Weather struct {
	TemperatureC   *float64 `json:"temperatureC,omitempty"`
	WindKnots      *int     `json:"windKnots,omitempty"`
	GustKnots      *int     `json:"gustKnots,omitempty"`
	RainLastHour   *int     `json:"rainLastHour,omitempty"`
	Rain24Hours    *int     `json:"rain24Hours,omitempty"`
	PressureHpa    *float64 `json:"pressureHpa,omitempty"`
	WindDirection  *int     `json:"windDirection,omitempty"`
	WindSpeedKnots *int     `json:"windSpeedKnots,omitempty"`
	Humidity       *int     `json:"humidity,omitempty"`
}

type Decoder struct {
	reader  *bufio.Reader
	frame   []byte
	inFrame bool
	escaped bool
}

func NewDecoder(reader io.Reader) *Decoder { return &Decoder{reader: bufio.NewReader(reader)} }
func (d *Decoder) Next() ([]byte, error) {
	for {
		value, err := d.reader.ReadByte()
		if err != nil {
			return nil, err
		}
		if value == 0xc0 {
			if d.inFrame && len(d.frame) > 1 {
				frame := d.frame
				d.frame = nil
				return frame, nil
			}
			d.frame = nil
			d.inFrame = true
			continue
		}
		if !d.inFrame {
			continue
		}
		if d.escaped {
			switch value {
			case 0xdc:
				value = 0xc0
			case 0xdd:
				value = 0xdb
			default:
				return nil, fmt.Errorf("invalid KISS escape byte 0x%02x", value)
			}
			d.escaped = false
		} else if value == 0xdb {
			d.escaped = true
			continue
		}
		d.frame = append(d.frame, value)
	}
}

func Parse(frame []byte) (Message, bool) {
	if len(frame) < 16 || frame[0] != 0 {
		return Message{}, false
	}
	addresses := []string{}
	offset := 1
	for offset+7 <= len(frame) {
		addresses = append(addresses, DecodeCallsign(frame[offset:offset+7]))
		last := frame[offset+6]&1 == 1
		offset += 7
		if last {
			break
		}
	}
	if len(addresses) < 2 || offset+2 > len(frame) {
		return Message{}, false
	}
	payload := strings.TrimSpace(string(frame[offset+2:]))
	if payload == "" {
		return Message{}, false
	}
	message := Message{Received: time.Now().UTC().Format(time.RFC3339), Source: addresses[1], Destination: addresses[0], Path: strings.Join(addresses[2:], " > "), Payload: payload, Raw: string(frame), Kind: "packet", Icon: "radio"}
	message.Weather = ParseWeather(payload)
	message.Position = ParsePosition(payload)
	if message.Position != nil {
		message.Symbol = message.Position.SymbolTable + message.Position.SymbolCode
	}
	if message.Weather != nil || strings.Contains(strings.ToUpper(payload), "WX") || strings.Contains(strings.ToUpper(payload), "WEATHER") {
		message.Kind, message.Icon = "weather", "weather"
	}
	message.Type = packetType(payload)
	message.Telemetry = ParseTelemetry(payload)
	if strings.HasPrefix(payload, ":") {
		message.Kind, message.Icon = "message", "message"
		message.IsMessage = true
	}
	return message, true
}

func ParsePosition(payload string) *Position {
	if strings.HasPrefix(payload, ";") && len(payload) >= 37 {
		return ParsePosition(payload[18:])
	}
	if strings.HasPrefix(payload, ")") && len(payload) >= 30 {
		return ParsePosition(payload[10:])
	}
	start := 0
	if payload[0] == '!' || payload[0] == '=' || payload[0] == '/' || payload[0] == '@' {
		start = 1
		if payload[0] == '/' || payload[0] == '@' {
			start = 8
		}
	}
	if len(payload) < start+19 {
		if len(payload) < start+10 {
			return nil
		}
	}
	value := payload[start:]
	if value[0] != '/' && value[0] != '\\' {
		return nil
	}
	if len(value) >= 10 && isBase91(value[1:9]) {
		y := base91(value[1:5])
		x := base91(value[5:9])
		lat := 90 - float64(y)/380926
		lon := -180 + float64(x)/190463
		return &Position{Latitude: lat, Longitude: lon, SymbolTable: string(value[0]), SymbolCode: string(value[9])}
	}
	if len(value) < 19 {
		return nil
	}
	lat, ok := coordinate(value[1:8], true)
	if !ok {
		return nil
	}
	if value[8] != 'N' && value[8] != 'S' {
		return nil
	}
	lon, ok := coordinate(value[10:18], false)
	if !ok {
		return nil
	}
	if value[18] != 'E' && value[18] != 'W' {
		return nil
	}
	if value[8] == 'S' {
		lat = -lat
	}
	if value[18] == 'W' {
		lon = -lon
	}
	return &Position{Latitude: lat, Longitude: lon, SymbolTable: string(value[0]), SymbolCode: string(value[9])}
}

func isBase91(value string) bool {
	for _, char := range value {
		if char < '!' || char > '}' {
			return false
		}
	}
	return true
}
func base91(value string) int {
	result := 0
	for _, char := range value {
		result = result*91 + int(char-'!')
	}
	return result
}

func packetType(payload string) string {
	if payload == "" {
		return "unknown"
	}
	switch payload[0] {
	case '!', '=', '/', '@':
		if ParsePosition(payload) != nil {
			return "position"
		}
	case ';':
		return "object"
	case ')':
		return "item"
	case 'T', 'P', 'U', 'E', 'B':
		if strings.HasPrefix(payload, "T#") || strings.HasPrefix(payload, "PARM") || strings.HasPrefix(payload, "UNIT") || strings.HasPrefix(payload, "EQNS") || strings.HasPrefix(payload, "BITS") {
			return "telemetry"
		}
	}
	return "packet"
}

func ParseTelemetry(payload string) *Telemetry {
	if strings.HasPrefix(payload, "T#") {
		parts := strings.Split(payload[2:], ",")
		if len(parts) < 7 {
			return nil
		}
		sequence, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil
		}
		telemetry := &Telemetry{Sequence: sequence, Digital: parts[6]}
		for _, value := range parts[1:6] {
			number, err := strconv.Atoi(value)
			if err != nil {
				return nil
			}
			telemetry.Analog = append(telemetry.Analog, number)
		}
		return telemetry
	}
	if strings.HasPrefix(payload, "PARM.") || strings.HasPrefix(payload, "UNIT.") || strings.HasPrefix(payload, "BITS.") {
		telemetry := &Telemetry{}
		values := strings.SplitN(payload[5:], ",", 2)
		if strings.HasPrefix(payload, "PARM.") {
			telemetry.Parameters = values
		}
		if strings.HasPrefix(payload, "UNIT.") {
			telemetry.Units = values
		}
		if strings.HasPrefix(payload, "BITS.") {
			telemetry.Bits = strings.TrimSpace(payload[5:])
		}
		return telemetry
	}
	return nil
}

func coordinate(value string, latitude bool) (float64, bool) {
	if latitude && len(value) != 7 || !latitude && len(value) != 8 {
		return 0, false
	}
	degreeWidth := 2
	if !latitude {
		degreeWidth = 3
	}
	degrees, err := strconv.ParseFloat(value[:degreeWidth], 64)
	minutes, err2 := strconv.ParseFloat(value[degreeWidth:], 64)
	if err != nil || err2 != nil || minutes >= 60 {
		return 0, false
	}
	return degrees + minutes/60, true
}

var weatherPatterns = map[string]*regexp.Regexp{
	"temperature": regexp.MustCompile(`(?i)t(-?\d{3})`),
	"direction":   regexp.MustCompile(`(?i)d(\d{3})`),
	"speed":       regexp.MustCompile(`(?i)s(\d{3})`),
	"gust":        regexp.MustCompile(`(?i)g(\d{3})`),
	"rainHour":    regexp.MustCompile(`(?i)r(\d{3})`),
	"rain24":      regexp.MustCompile(`(?i)p(\d{3})`),
	"humidity":    regexp.MustCompile(`(?i)h(\d{2})`),
	"pressure":    regexp.MustCompile(`(?i)b(\d{5})`),
}

func ParseWeather(payload string) *Weather {
	weather := &Weather{}
	found := false
	if match := weatherPatterns["temperature"].FindStringSubmatch(payload); len(match) == 2 {
		value, _ := strconv.ParseFloat(match[1], 64)
		value = (value - 32) * 5 / 9
		weather.TemperatureC = &value
		found = true
	}
	for name, pattern := range weatherPatterns {
		if name == "temperature" {
			continue
		}
		if match := pattern.FindStringSubmatch(payload); len(match) == 2 {
			value, _ := strconv.Atoi(match[1])
			switch name {
			case "direction":
				weather.WindDirection = &value
			case "speed":
				weather.WindSpeedKnots = &value
				weather.WindKnots = &value
			case "gust":
				weather.GustKnots = &value
			case "rainHour":
				weather.RainLastHour = &value
			case "rain24":
				weather.Rain24Hours = &value
			case "humidity":
				weather.Humidity = &value
			case "pressure":
				pressure := float64(value) / 10
				weather.PressureHpa = &pressure
			}
			found = true
		}
	}
	if !found {
		return nil
	}
	return weather
}

func DecodeCallsign(address []byte) string {
	call := make([]byte, 6)
	for i := range call {
		call[i] = address[i] >> 1
	}
	name := strings.TrimSpace(string(call))
	ssid := (address[6] >> 1) & 0x0f
	if ssid > 0 {
		return name + "-" + strconv.Itoa(int(ssid))
	}
	return name
}
func SplitCallsign(value string) (string, byte) {
	parts := strings.SplitN(strings.ToUpper(strings.TrimSpace(value)), "-", 2)
	if len(parts) == 1 {
		return parts[0], 0
	}
	ssid, _ := strconv.Atoi(parts[1])
	if ssid < 0 || ssid > 15 {
		ssid = 0
	}
	return parts[0], byte(ssid)
}
func Address(call string, ssid byte, last bool) []byte {
	address := make([]byte, 7)
	call = strings.ToUpper(call)
	for i := 0; i < 6; i++ {
		value := byte(' ')
		if i < len(call) {
			value = call[i]
		}
		address[i] = value << 1
	}
	address[6] = 0x60 | ((ssid & 0x0f) << 1)
	if last {
		address[6] |= 1
	}
	return address
}
func EncodeMessage(destination, source, text string) []byte {
	destinationCall, destinationSSID := SplitCallsign(destination)
	sourceCall, sourceSSID := SplitCallsign(source)
	frame := []byte{0}
	frame = append(frame, Address(destinationCall, destinationSSID, false)...)
	frame = append(frame, Address(sourceCall, sourceSSID, true)...)
	frame = append(frame, 0x03, 0xf0)
	frame = append(frame, []byte(fmt.Sprintf(":%-9s:%s", destination, text))...)
	encoded := []byte{0xc0}
	for _, value := range frame {
		switch value {
		case 0xc0:
			encoded = append(encoded, 0xdb, 0xdc)
		case 0xdb:
			encoded = append(encoded, 0xdb, 0xdd)
		default:
			encoded = append(encoded, value)
		}
	}
	return append(encoded, 0xc0)
}
