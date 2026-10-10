package aprs

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Message struct {
	ID          int64         `json:"id"`
	Received    string        `json:"received"`
	Source      string        `json:"source"`
	Destination string        `json:"destination"`
	Path        string        `json:"path"`
	Payload     string        `json:"payload"`
	Raw         string        `json:"raw"`
	Kind        string        `json:"kind"`
	Icon        string        `json:"icon"`
	Weather     *Weather      `json:"weather,omitempty"`
	Position    *Position     `json:"position,omitempty"`
	Symbol      string        `json:"symbol,omitempty"`
	IsMessage   bool          `json:"isMessage"`
	Type        string        `json:"type"`
	Telemetry   *Telemetry    `json:"telemetry,omitempty"`
	APRSMessage *APRSMessage  `json:"aprsMessage,omitempty"`
	Object      *ObjectReport `json:"object,omitempty"`
	StatusText  string        `json:"statusText,omitempty"`
	Query       string        `json:"query,omitempty"`
	GridLocator string        `json:"gridLocator,omitempty"`
	ThirdParty  *Message      `json:"thirdParty,omitempty"`
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
	Timestamp   string  `json:"timestamp,omitempty"`
	Ambiguity   int     `json:"ambiguity,omitempty"`
	AltitudeFt  *int    `json:"altitudeFt,omitempty"`
	Course      *int    `json:"course,omitempty"`
	SpeedKnots  *int    `json:"speedKnots,omitempty"`
	DFBearing   *int    `json:"dfBearing,omitempty"`
	DFNumber    *int    `json:"dfNumber,omitempty"`
	DFRange     *int    `json:"dfRange,omitempty"`
	DFQuality   *int    `json:"dfQuality,omitempty"`
}

type Telemetry struct {
	Sequence      int         `json:"sequence"`
	SequenceLabel string      `json:"sequenceLabel,omitempty"`
	Analog        []int       `json:"analog"`
	AnalogValues  []float64   `json:"analogValues,omitempty"`
	Digital       string      `json:"digital"`
	Parameters    []string    `json:"parameters,omitempty"`
	Units         []string    `json:"units,omitempty"`
	Coefficients  [][]float64 `json:"coefficients,omitempty"`
	Bits          string      `json:"bits,omitempty"`
	Base91        bool        `json:"base91,omitempty"`
}

type Weather struct {
	TemperatureC        *float64 `json:"temperatureC,omitempty"`
	WindKnots           *int     `json:"windKnots,omitempty"`
	GustKnots           *int     `json:"gustKnots,omitempty"`
	RainLastHour        *int     `json:"rainLastHour,omitempty"`
	Rain24Hours         *int     `json:"rain24Hours,omitempty"`
	RainLastHourMm      *float64 `json:"rainLastHourMm,omitempty"`
	Rain24HoursMm       *float64 `json:"rain24HoursMm,omitempty"`
	PressureHpa         *float64 `json:"pressureHpa,omitempty"`
	WindDirection       *int     `json:"windDirection,omitempty"`
	WindSpeedKnots      *int     `json:"windSpeedKnots,omitempty"`
	WindSpeedMph        *int     `json:"windSpeedMph,omitempty"`
	GustMph             *int     `json:"gustMph,omitempty"`
	Humidity            *int     `json:"humidity,omitempty"`
	RainSinceMidnight   *int     `json:"rainSinceMidnight,omitempty"`
	RainSinceMidnightMm *float64 `json:"rainSinceMidnightMm,omitempty"`
	Luminosity          *int     `json:"luminosity,omitempty"`
	Software            string   `json:"software,omitempty"`
	Unit                string   `json:"unit,omitempty"`
}

type APRSMessage struct {
	Addressee string `json:"addressee"`
	Text      string `json:"text"`
	Number    string `json:"number,omitempty"`
	ReplyAck  string `json:"replyAck,omitempty"`
}

type ObjectReport struct {
	Name      string    `json:"name"`
	Alive     bool      `json:"alive"`
	Timestamp string    `json:"timestamp,omitempty"`
	Position  *Position `json:"position,omitempty"`
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
	payload := string(frame[offset+2:])
	if payload == "" {
		return Message{}, false
	}
	return parseMessage(addresses[1], addresses[0], strings.Join(addresses[2:], " > "), payload, string(frame), 0), true
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
	if strings.HasPrefix(payload, "$") {
		return parseNMEAPosition(payload)
	}
	if len(payload) > 0 && (payload[0] == '`' || payload[0] == '\'') {
		if len(destination) > 0 {
			if pos := parseMicEPosition(destination[0], payload); pos != nil {
				return pos
			}
		}
	}
	if strings.HasPrefix(payload, ";") && len(payload) >= 28 {
		return ParsePosition("!" + payload[18:])
	}
	if strings.HasPrefix(payload, ")") {
		end := strings.IndexAny(payload[1:], "!_")
		if end >= 3 && end <= 9 {
			return ParsePosition("!" + payload[end+2:])
		}
	}
	start := positionStart(payload)
	if start < 0 || len(payload) < start+10 {
		return nil
	}
	value := payload[start:]
	timestamp := ""
	if start == 8 {
		timestamp = payload[1:8]
		if !validPositionTimestamp(timestamp) {
			return nil
		}
	}
	if len(value) >= 10 && (value[0] == '/' || value[0] == '\\') && isBase91(value[1:9]) {
		y := base91(value[1:5])
		x := base91(value[5:9])
		lat := 90 - float64(y)/380926
		lon := -180 + float64(x)/190463
		if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
			return nil
		}
		return positionMetadata(&Position{Latitude: lat, Longitude: lon, SymbolTable: string(value[0]), SymbolCode: string(value[9]), Timestamp: timestamp}, value[10:])
	}
	if len(value) < 19 {
		return nil
	}
	lat, latAmbiguity, ok := coordinateWithAmbiguity(value[0:7], true)
	if !ok {
		return nil
	}
	if value[7] != 'N' && value[7] != 'S' {
		return nil
	}
	lon, lonAmbiguity, ok := coordinateWithAmbiguity(value[9:17], false)
	if !ok {
		return nil
	}
	if value[17] != 'E' && value[17] != 'W' {
		return nil
	}
	if !isSymbolTable(value[8]) {
		return nil
	}
	if latAmbiguity != lonAmbiguity {
		return nil
	}
	if value[7] == 'S' {
		lat = -lat
	}
	if value[17] == 'W' {
		lon = -lon
	}
	return positionMetadata(&Position{Latitude: lat, Longitude: lon, SymbolTable: string(value[8]), SymbolCode: string(value[18]), Timestamp: timestamp, Ambiguity: latAmbiguity}, value[19:])
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
	for {
		comment := position.Comment
		switch {
		case len(comment) >= 8 && strings.HasPrefix(comment, "#PHG") && isDigits(comment[4:8]):
			position.PHG = comment[1:8]
			position.Comment = strings.TrimSpace(comment[8:])
		case len(comment) >= 7 && strings.HasPrefix(comment, "PHG") && isDigits(comment[3:7]):
			position.PHG = comment[:7]
			position.Comment = strings.TrimSpace(comment[7:])
		case len(comment) >= 9 && strings.HasPrefix(comment, "/A=") && isDigits(comment[3:9]):
			altitude, _ := strconv.Atoi(comment[3:9])
			position.AltitudeFt = &altitude
			position.Comment = strings.TrimSpace(comment[9:])
		case len(comment) >= 7 && comment[3] == '/' && isDigits(comment[:3]) && isDigits(comment[4:7]):
			course, _ := strconv.Atoi(comment[:3])
			speed, _ := strconv.Atoi(comment[4:7])
			position.Course = &course
			position.SpeedKnots = &speed
			position.Comment = strings.TrimSpace(comment[7:])
		case position.SymbolTable == "/" && position.SymbolCode == "\\" && parseDFExtension(position, comment):
		default:
			return positionMetadataURL(position)
		}
	}
}

func positionMetadataURL(position *Position) *Position {
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

func parseDFExtension(position *Position, comment string) bool {
	parts := strings.SplitN(comment, "/", 3)
	if len(parts) != 3 || len(parts[1]) != 3 || len(parts[2]) < 3 || !isDigits(parts[1]) || !isDigits(parts[2][:3]) {
		return false
	}
	bearing, _ := strconv.Atoi(parts[1])
	number := int(parts[2][0] - '0')
	rangeValue := int(parts[2][1] - '0')
	quality := int(parts[2][2] - '0')
	position.DFBearing, position.DFNumber, position.DFRange, position.DFQuality = &bearing, &number, &rangeValue, &quality
	position.Comment = strings.TrimSpace(parts[2][3:])
	return true
}

func isDigits(value string) bool {
	if value == "" {
		return false
	}
	for index := range value {
		if value[index] < '0' || value[index] > '9' {
			return false
		}
	}
	return true
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

func validPositionTimestamp(value string) bool {
	if len(value) != 7 {
		return false
	}
	switch value[6] {
	case 'z', '/':
		return validDigits(value[:6], 31, 23, 59)
	case 'h':
		return validDigits(value[:6], 0, 23, 59)
	default:
		return false
	}
}

func validDigits(value string, maxDay, maxHour, maxMinute int) bool {
	if len(value) != 6 {
		return false
	}
	for index := range value {
		if value[index] < '0' || value[index] > '9' {
			return false
		}
	}
	first, _ := strconv.Atoi(value[:2])
	second, _ := strconv.Atoi(value[2:4])
	third, _ := strconv.Atoi(value[4:6])
	if maxDay > 0 {
		return first >= 1 && first <= maxDay && second <= maxHour && third <= maxMinute
	}
	return first <= maxHour && second <= maxMinute && third <= 59
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

func packetType(payload string, position *Position, weather *Weather) string {
	if payload == "" {
		return "unknown"
	}
	if payload[0] == ';' {
		return "object"
	}
	if payload[0] == ')' {
		return "item"
	}
	if weather != nil || payload[0] == '_' || position != nil && position.SymbolCode == "_" {
		return "weather"
	}
	switch payload[0] {
	case '!', '=':
		if position == nil {
			if payload[0] == '!' && (strings.HasPrefix(payload, "!#") || strings.HasPrefix(payload, "!$") || strings.HasPrefix(payload, "!*")) {
				return "raw-weather"
			}
			return "raw-weather"
		}
		if position.SymbolTable == "/" && position.SymbolCode == "\\" {
			return "df"
		}
		return "position"
	case '/', '@':
		if position == nil {
			return "unknown"
		}
		if position.SymbolTable == "/" && position.SymbolCode == "\\" {
			return "df"
		}
		return "position"
	case '$':
		if strings.HasPrefix(payload, "$ULTW") {
			return "raw-weather"
		}
		return "raw-gps"
	case '%':
		return "agrelo-dfjr"
	case '&':
		return "peet-bros"
	case '#', '*':
		return "raw-weather"
	case '\'', '`':
		return "mic-e"
	case ':':
		if data := ParseAPRSMessage(payload); data != nil {
			switch {
			case strings.HasPrefix(strings.ToLower(data.Text), "ack"):
				return "ack"
			case strings.HasPrefix(strings.ToLower(data.Text), "rej"):
				return "reject"
			case strings.HasPrefix(strings.ToUpper(data.Addressee), "NWS-"):
				return "weather-bulletin"
			case strings.HasPrefix(strings.ToUpper(data.Addressee), "BLN"):
				if len(data.Addressee) > 3 && data.Addressee[3] >= 'A' && data.Addressee[3] <= 'Z' {
					return "announcement"
				}
				return "bulletin"
			}
		}
		return "message"
	case '<':
		return "capabilities"
	case '>':
		return "status"
	case '?':
		return "query"
	case '[':
		return "maidenhead"
	case '{':
		return "user-defined"
	case '|':
		return "fault"
	case '}':
		return "third-party"
	case 'T':
		if strings.HasPrefix(payload, "T#") {
			return "telemetry"
		}
	case 'P':
		if strings.HasPrefix(payload, "PARM.") {
			return "telemetry"
		}
	case 'U':
		if strings.HasPrefix(payload, "UNIT.") {
			return "telemetry"
		}
	case 'E':
		if strings.HasPrefix(payload, "EQNS.") {
			return "telemetry"
		}
	case 'B':
		if strings.HasPrefix(payload, "BITS.") {
			return "telemetry"
		}
	case '+':
		return "shelter"
	case '.':
		return "space-weather"
	case ',', '"', '(', '-', '\\', ']', '^':
		return "reserved"
	case 0x1c, 0x1d:
		return "mic-e"
	}
	if payload[0] >= '0' && payload[0] <= '9' || payload[0] >= 'A' && payload[0] <= 'S' || payload[0] >= 'U' && payload[0] <= 'Z' || payload[0] == '~' {
		return "reserved"
	}
	return "unknown"
}

func ParseTelemetry(payload string) *Telemetry {
	if strings.HasPrefix(payload, "T#") {
		parts := strings.Split(payload[2:], ",")
		if len(parts) < 7 {
			return nil
		}
		sequence, sequenceLabel := parseTelemetrySequence(parts[0])
		telemetry := &Telemetry{Sequence: sequence, SequenceLabel: sequenceLabel, Digital: strings.TrimSpace(parts[6])}
		for _, value := range parts[1:6] {
			number, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
			if err != nil {
				return nil
			}
			telemetry.AnalogValues = append(telemetry.AnalogValues, number)
			telemetry.Analog = append(telemetry.Analog, int(number))
		}
		return telemetry
	}
	if strings.HasPrefix(payload, "PARM.") || strings.HasPrefix(payload, "UNIT.") || strings.HasPrefix(payload, "EQNS.") || strings.HasPrefix(payload, "BITS.") {
		telemetry := &Telemetry{}
		values := strings.Split(strings.TrimSpace(payload[5:]), ",")
		if strings.HasPrefix(payload, "PARM.") {
			telemetry.Parameters = values
		}
		if strings.HasPrefix(payload, "UNIT.") {
			telemetry.Units = values
		}
		if strings.HasPrefix(payload, "EQNS.") {
			if len(values)%3 != 0 {
				return nil
			}
			for i := 0; i < len(values); i += 3 {
				coefficients := make([]float64, 3)
				for j := range coefficients {
					value, err := strconv.ParseFloat(strings.TrimSpace(values[i+j]), 64)
					if err != nil {
						return nil
					}
					coefficients[j] = value
				}
				telemetry.Coefficients = append(telemetry.Coefficients, coefficients)
			}
		}
		if strings.HasPrefix(payload, "BITS.") {
			telemetry.Bits = strings.TrimSpace(payload[5:])
		}
		return telemetry
	}
	if start := strings.Index(payload, "|"); start >= 0 {
		if end := strings.Index(payload[start+1:], "|"); end >= 0 {
			encoded := payload[start+1 : start+1+end]
			if len(encoded) >= 4 && len(encoded)%2 == 0 && len(encoded) <= 14 && isBase91(encoded) {
				values := make([]int, len(encoded)/2)
				for i := range values {
					values[i] = base91(encoded[i*2 : i*2+2])
					if values[i] > 8280 {
						return nil
					}
				}
				telemetry := &Telemetry{Sequence: values[0], Base91: true}
				channelCount := len(values) - 1
				if channelCount > 6 {
					return nil
				}
				if channelCount == 6 {
					channelCount--
					telemetry.Digital = base91DigitalBits(values[len(values)-1])
				}
				telemetry.Analog = append(telemetry.Analog, values[1:1+channelCount]...)
				for _, value := range values[1 : 1+channelCount] {
					telemetry.AnalogValues = append(telemetry.AnalogValues, float64(value))
				}
				return telemetry
			}
		}
	}
	return nil
}

func base91DigitalBits(value int) string {
	bits := []byte("00000000")
	for index := range bits {
		if value&(1<<index) != 0 {
			bits[index] = '1'
		}
	}
	return string(bits)
}

func coordinate(value string, latitude bool) (float64, bool) {
	result, _, ok := coordinateWithAmbiguity(value, latitude)
	return result, ok
}

func coordinateWithAmbiguity(value string, latitude bool) (float64, int, bool) {
	if latitude && len(value) != 7 || !latitude && len(value) != 8 {
		return 0, 0, false
	}
	degreeWidth := 2
	if !latitude {
		degreeWidth = 3
	}
	degrees, err := strconv.Atoi(value[:degreeWidth])
	if err != nil {
		return 0, 0, false
	}
	minutesText := value[degreeWidth:]
	minutesDigits := minutesText[:2] + minutesText[3:]
	ambiguity := strings.Count(minutesText, " ")
	if strings.Count(minutesText, ".") != 1 || minutesText[2] != '.' || ambiguity > 4 {
		return 0, 0, false
	}
	allowedSpaces := map[int]bool{}
	switch ambiguity {
	case 0:
	case 1:
		allowedSpaces[3] = true
	case 2:
		allowedSpaces[2], allowedSpaces[3] = true, true
	case 3:
		allowedSpaces[1], allowedSpaces[2], allowedSpaces[3] = true, true, true
	case 4:
		allowedSpaces[0], allowedSpaces[1], allowedSpaces[2], allowedSpaces[3] = true, true, true, true
	default:
		return 0, 0, false
	}
	for i, char := range minutesDigits {
		if char == ' ' && !allowedSpaces[i] {
			return 0, 0, false
		}
		if char != ' ' && (char < '0' || char > '9') {
			return 0, 0, false
		}
	}
	normalized := strings.ReplaceAll(minutesText, " ", "0")
	minutes, err := strconv.ParseFloat(normalized, 64)
	if err != nil || minutes >= 60 {
		return 0, 0, false
	}
	switch ambiguity {
	case 1:
		minutes += 0.005
	case 2:
		minutes += 0.05
	case 3:
		minutes += 5
	case 4:
		minutes += 30
	}
	maxDegrees := 90
	if !latitude {
		maxDegrees = 180
	}
	if degrees > maxDegrees || degrees == maxDegrees && minutes > 0 || minutes >= 60 {
		return 0, 0, false
	}
	return float64(degrees) + minutes/60, ambiguity, true
}

var weatherPatterns = map[string]*regexp.Regexp{
	"temperature":    regexp.MustCompile(`(?i)t(-?\d{3,4})`),
	"direction":      regexp.MustCompile(`(?i)d(\d{3,4})`),
	"speed":          regexp.MustCompile(`(?i)s(\d{3,4})`),
	"windSlash":      regexp.MustCompile(`(\d{3})/(\d{3})`),
	"gust":           regexp.MustCompile(`(?i)g(\d{3,4})`),
	"rainHour":       regexp.MustCompile(`(?i)r(\d{3,4})`),
	"rain24":         regexp.MustCompile(`p(\d{3,4})`),
	"rainMidnight":   regexp.MustCompile(`P(\d{3,4})`),
	"humidity":       regexp.MustCompile(`(?i)h(\d{2,3})`),
	"pressure":       regexp.MustCompile(`(?i)b(\d{5,6})`),
	"luminosity":     regexp.MustCompile(`L(\d{3})`),
	"luminosityHigh": regexp.MustCompile(`l(\d{3})`),
	"windDirection":  regexp.MustCompile(`(?i)c(\d{3,4})`),
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
	names := make([]string, 0, len(weatherPatterns))
	for name := range weatherPatterns {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		pattern := weatherPatterns[name]
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
				weather.WindSpeedMph = &value
				weather.WindKnots = &value
			case "gust":
				weather.GustKnots = &value
				weather.GustMph = &value
			case "rainHour":
				weather.RainLastHour = &value
				millimeters := float64(value) * 0.254
				weather.RainLastHourMm = &millimeters
			case "rain24":
				weather.Rain24Hours = &value
				millimeters := float64(value) * 0.254
				weather.Rain24HoursMm = &millimeters
			case "rainMidnight":
				weather.RainSinceMidnight = &value
				millimeters := float64(value) * 0.254
				weather.RainSinceMidnightMm = &millimeters
			case "humidity":
				if value == 0 {
					value = 100
				}
				weather.Humidity = &value
			case "pressure":
				pressure := float64(value) / 10
				weather.PressureHpa = &pressure
			case "luminosity":
				weather.Luminosity = &value
			case "luminosityHigh":
				value += 1000
				weather.Luminosity = &value
			case "windDirection":
				weather.WindDirection = &value
			}
			found = true
		}
	}
	weather.Software, weather.Unit = parseWeatherStationInfo(payload)
	if !found {
		return nil
	}
	return weather
}

var weatherFields = regexp.MustCompile(`(?i)(\d{3}/\d{3}|[cdgprs]\d{3,4}|t-?\d{3,4}|P\d{3,4}|b\d{5,6}|h\d{2,3}|[Ll]\d{3})`)

func ParseWeatherForMessage(payload string, position *Position) *Weather {
	if strings.HasPrefix(payload, "_") || position != nil && position.SymbolCode == "_" {
		return ParseWeather(payload)
	}
	return nil
}

func parseWeatherStationInfo(payload string) (string, string) {
	marker := strings.LastIndexByte(payload, 'w')
	if marker < 0 {
		return "", ""
	}
	metadata := payload[marker+1:]
	if len(metadata) < 3 || len(metadata) > 5 {
		return "", ""
	}
	software, unit := "", metadata
	if strings.Contains("dMPSWX", metadata[:1]) {
		software, unit = metadata[:1], metadata[1:]
	}
	if len(unit) < 1 || len(unit) > 4 {
		return "", ""
	}
	for index := range unit {
		if !isSymbolTable(unit[index]) && unit[index] != '-' {
			return "", ""
		}
	}
	return software, unit
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
