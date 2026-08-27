package aprs

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
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
	Comment     string  `json:"comment,omitempty"`
	PHG         string  `json:"phg,omitempty"`
	URL         string  `json:"url,omitempty"`
	Locator     string  `json:"locator,omitempty"`
	MicEStatus  string  `json:"micEStatus,omitempty"`
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
	RainLastHourMm *float64 `json:"rainLastHourMm,omitempty"`
	Rain24HoursMm  *float64 `json:"rain24HoursMm,omitempty"`
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
	if len(frame) < 16 || frame[0]&0x0f != 0 {
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
	if len(addresses) < 2 || offset+2 > len(frame) || frame[offset] != 0x03 || frame[offset+1] != 0xf0 {
		return Message{}, false
	}
	payload := CleanPayload(frame[offset+2:])
	if payload == "" {
		return Message{}, false
	}
	message := Message{Received: time.Now().UTC().Format(time.RFC3339), Source: addresses[1], Destination: addresses[0], Path: strings.Join(addresses[2:], " > "), Payload: payload, Raw: string(frame), Kind: "packet", Icon: "radio"}
	message.Position = ParsePosition(payload, addresses[0])
	if message.Position != nil && message.Position.SymbolCode == "_" {
		message.Weather = ParseWeather(payload)
	}
	if message.Position != nil {
		message.Symbol = message.Position.SymbolTable + message.Position.SymbolCode
	}
	if message.Weather != nil {
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

// CleanPayload keeps APRS text readable while removing TNC line noise and invalid UTF-8.
func CleanPayload(payload []byte) string {
	text := strings.ToValidUTF8(string(payload), "�")
	text = strings.ReplaceAll(text, "\r", "")
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.TrimSpace(text)
	return strings.Map(func(char rune) rune {
		if char == '\t' || char == '�' || utf8.ValidRune(char) && unicode.IsPrint(char) {
			return char
		}
		return '�'
	}, text)
}

func ParsePosition(payload string, destination ...string) *Position {
	if len(payload) > 0 && (payload[0] == '`' || payload[0] == '\'') {
		if len(destination) > 0 {
			if pos := parseMicEPosition(destination[0], payload); pos != nil {
				return pos
			}
		}
	}
	if strings.HasPrefix(payload, ";") && len(payload) >= 37 {
		return ParsePosition(payload[18:])
	}
	if strings.HasPrefix(payload, ")") && len(payload) >= 30 {
		return ParsePosition(payload[10:])
	}
	start := positionStart(payload)
	if start < 0 || len(payload) < start+10 {
		return nil
	}
	value := payload[start:]
	if len(value) >= 10 && (value[0] == '/' || value[0] == '\\') && isBase91(value[1:9]) {
		y := base91(value[1:5])
		x := base91(value[5:9])
		lat := 90 - float64(y)/380926
		lon := -180 + float64(x)/190463
		return positionMetadata(&Position{Latitude: lat, Longitude: lon, SymbolTable: string(value[0]), SymbolCode: string(value[9])}, value[10:])
	}
	if len(value) < 19 {
		return nil
	}
	lat, ok := coordinate(value[0:7], true)
	if !ok {
		return nil
	}
	if value[7] != 'N' && value[7] != 'S' {
		return nil
	}
	lon, ok := coordinate(value[9:17], false)
	if !ok {
		return nil
	}
	if value[17] != 'E' && value[17] != 'W' {
		return nil
	}
	if !isSymbolTable(value[8]) {
		return nil
	}
	if value[7] == 'S' {
		lat = -lat
	}
	if value[17] == 'W' {
		lon = -lon
	}
	return positionMetadata(&Position{Latitude: lat, Longitude: lon, SymbolTable: string(value[8]), SymbolCode: string(value[18])}, value[19:])
}

func parseMicEPosition(destination, payload string) *Position {
	if len(payload) < 10 || (payload[0] != '`' && payload[0] != '\'') {
		return nil
	}
	dest := strings.ToUpper(strings.TrimSpace(destination))
	if len(dest) < 6 {
		return nil
	}
	dest = dest[:6]
	lat, status, ok := decodeMicELatitudeAndStatus([]byte(dest))
	if !ok {
		return nil
	}
	lon, ok := decodeMicELongitude([]byte(dest), payload[1:4])
	if !ok {
		return nil
	}
	if len(payload) < 10 {
		return nil
	}
	symbolCode := payload[7:8]
	symbolTable := payload[8:9]
	comment := strings.TrimSpace(payload[9:])
	if comment == "" {
		return positionMetadata(&Position{Latitude: lat, Longitude: lon, SymbolTable: symbolTable, SymbolCode: symbolCode, MicEStatus: status}, "")
	}
	if len(comment) > 0 && (comment[0] == '`' || comment[0] == '\'' || comment[0] == '>' || comment[0] == ']' || comment[0] == ' ' || comment[0] == '#' || comment[0] == '$' || comment[0] == '%' || comment[0] == '^' || comment[0] == '|' || comment[0] == '~' || comment[0] == '"') {
		comment = comment[1:]
	}
	if len(comment) >= 4 && isMicEAltitude(comment[:4]) {
		comment = comment[4:]
	}
	position := &Position{Latitude: lat, Longitude: lon, SymbolTable: symbolTable, SymbolCode: symbolCode, MicEStatus: status}
	return positionMetadata(position, comment)
}

func isMicEAltitude(value string) bool {
	if len(value) != 4 {
		return false
	}
	return value[3] == '}' && value[0] >= '!' && value[0] <= '~' && value[1] >= '!' && value[1] <= '~' && value[2] >= '!' && value[2] <= '~'
}

func decodeMicELatitudeAndStatus(destination []byte) (float64, string, bool) {
	if len(destination) < 6 {
		return 0, "", false
	}
	stdMsg := 0
	custMsg := 0
	value0, ok := micEDigitWithFlags(destination[0], 4, &stdMsg, &custMsg)
	if !ok {
		return 0, "", false
	}
	value1, ok := micEDigitWithFlags(destination[1], 2, &stdMsg, &custMsg)
	if !ok {
		return 0, "", false
	}
	value2, ok := micEDigitWithFlags(destination[2], 1, &stdMsg, &custMsg)
	if !ok {
		return 0, "", false
	}
	value3, ok := micEDigitWithFlags(destination[3], 0, &stdMsg, &custMsg)
	if !ok {
		return 0, "", false
	}
	value4, ok := micEDigitWithFlags(destination[4], 0, &stdMsg, &custMsg)
	if !ok {
		return 0, "", false
	}
	value5, ok := micEDigitWithFlags(destination[5], 0, &stdMsg, &custMsg)
	if !ok {
		return 0, "", false
	}
	lat := float64(value0*10+value1) + float64(value2*1000+value3*100+value4*10+value5)/6000.0
	if destination[3] >= '0' && destination[3] <= '9' || destination[3] == 'L' {
		lat = -lat
	}
	return lat, decodeMicEStatus(stdMsg, custMsg), true
}

func decodeMicELongitude(destination []byte, info string) (float64, bool) {
	if len(info) < 3 {
		return 0, false
	}
	offset := 0
	if destination[4] >= '0' && destination[4] <= '9' || destination[4] == 'L' {
		offset = 0
	} else if destination[4] >= 'P' && destination[4] <= 'Z' {
		offset = 1
	}
	first := info[0]
	var lon float64
	switch {
	case offset == 1 && first >= 118 && first <= 127:
		lon = float64(first - 118)
	case offset == 0 && first >= 38 && first <= 127:
		lon = float64(first-38) + 10
	case offset == 1 && first >= 108 && first <= 117:
		lon = float64(first-108) + 100
	case offset == 1 && first >= 38 && first <= 107:
		lon = float64(first-38) + 110
	default:
		return 0, false
	}
	second := info[1]
	switch {
	case second >= 88 && second <= 97:
		lon += float64(second-88) / 60.0
	case second >= 38 && second <= 87:
		lon += float64(second-38+10) / 60.0
	default:
		return 0, false
	}
	third := info[2]
	if third < 28 || third > 127 {
		return 0, false
	}
	lon += float64(third-28) / 6000.0
	if destination[5] >= 'P' && destination[5] <= 'Z' {
		lon = -lon
	}
	return lon, true
}

func micEDigitWithFlags(ch byte, mask int, stdMsg, custMsg *int) (int, bool) {
	switch {
	case ch >= '0' && ch <= '9':
		return int(ch - '0'), true
	case ch >= 'A' && ch <= 'J':
		*custMsg |= mask
		return int(ch - 'A'), true
	case ch >= 'P' && ch <= 'Y':
		*stdMsg |= mask
		return int(ch - 'P'), true
	case ch == 'K':
		*custMsg |= mask
		return 0, true
	case ch == 'L':
		return 0, true
	case ch == 'Z':
		*stdMsg |= mask
		return 0, true
	default:
		return 0, false
	}
}

func decodeMicEStatus(stdMsg, custMsg int) string {
	stdText := []string{"Emergency", "Priority", "Special", "Committed", "Returning", "In Service", "En Route", "Off Duty"}
	custText := []string{"Emergency", "Custom-6", "Custom-5", "Custom-4", "Custom-3", "Custom-2", "Custom-1", "Custom-0"}
	if stdMsg == 0 && custMsg == 0 {
		return "Emergency"
	}
	if stdMsg == 0 && custMsg != 0 {
		return custText[custMsg]
	}
	if stdMsg != 0 && custMsg == 0 {
		return stdText[stdMsg]
	}
	return "Unknown MIC-E Message Type"
}

func isSymbolTable(value byte) bool {
	return value == '/' || value == '\\' || value >= '0' && value <= '9' || value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}

func positionMetadata(position *Position, comment string) *Position {
	position.Comment = strings.TrimSpace(comment)
	position.Locator = maidenhead(position.Latitude, position.Longitude)
	if strings.HasPrefix(position.Comment, "#PHG") && len(position.Comment) >= 8 {
		position.PHG = position.Comment[1:8]
		position.Comment = strings.TrimSpace(position.Comment[8:])
	}
	lower := strings.ToLower(position.Comment)
	for _, scheme := range []string{"http://", "https://"} {
		if start := strings.Index(lower, scheme); start >= 0 {
			position.URL = strings.TrimSpace(position.Comment[start:])
			position.Comment = strings.TrimSpace(position.Comment[:start])
			break
		}
	}
	return position
}

func maidenhead(latitude, longitude float64) string {
	if latitude < -90 || latitude > 90 || longitude < -180 || longitude > 180 {
		return ""
	}
	longitude += 180
	latitude += 90
	fieldLon, fieldLat := int(longitude/20), int(latitude/10)
	longitude -= float64(fieldLon * 20)
	latitude -= float64(fieldLat * 10)
	squareLon, squareLat := int(longitude/2), int(latitude)
	longitude -= float64(squareLon * 2)
	latitude -= float64(squareLat)
	subLon, subLat := int(longitude*12), int(latitude*24)
	return string([]byte{'A' + byte(fieldLon), 'A' + byte(fieldLat), '0' + byte(squareLon), '0' + byte(squareLat), 'A' + byte(subLon), 'A' + byte(subLat)})
}

func positionStart(payload string) int {
	if payload == "" {
		return -1
	}
	switch payload[0] {
	case '!', '=':
		return 1
	case '/', '@':
		if len(payload) < 8 {
			return -1
		}
		return 8
	default:
		return 0
	}
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
	"windSlash":   regexp.MustCompile(`(\d{3})/(\d{3})`),
	"gust":        regexp.MustCompile(`(?i)g(\d{3})`),
	"rainHour":    regexp.MustCompile(`(?i)r(\d{3})`),
	"rain24":      regexp.MustCompile(`(?i)p(\d{3})`),
	"humidity":    regexp.MustCompile(`(?i)h(\d{2})`),
	"pressure":    regexp.MustCompile(`(?i)b(\d{5})`),
}

func ParseWeather(payload string) *Weather {
	if !weatherFields.MatchString(payload) {
		return nil
	}
	weather := &Weather{}
	found := false
	if match := weatherPatterns["temperature"].FindStringSubmatch(payload); len(match) == 2 {
		value, _ := strconv.ParseFloat(match[1], 64)
		value = (value - 32) * 5 / 9
		weather.TemperatureC = &value
		found = true
	}
	if match := weatherPatterns["windSlash"].FindStringSubmatch(payload); len(match) == 3 {
		direction, _ := strconv.Atoi(match[1])
		speed, _ := strconv.Atoi(match[2])
		weather.WindDirection = &direction
		weather.WindSpeedKnots = &speed
		weather.WindKnots = &speed
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
				millimeters := float64(value) * 0.254
				weather.RainLastHourMm = &millimeters
			case "rain24":
				weather.Rain24Hours = &value
				millimeters := float64(value) * 0.254
				weather.Rain24HoursMm = &millimeters
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

var weatherFields = regexp.MustCompile(`(?i)(\d{3}/\d{3}|g\d{3}|t-?\d{3}|r\d{3}|p\d{3}|P\d{3}|b\d{5}|h\d{2})`)

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
