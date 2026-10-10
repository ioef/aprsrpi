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

const (
	ax25AddressLength         = 7
	ax25AddressExtension      = 6
	ax25AddressCountMinimum   = 2
	ax25CommandMask           = 0x0f
	ax25DataCommand           = 0
	ax25ControlField          = 0x03
	ax25NoLayer3Protocol      = 0xf0
	ax25ControlAndPIDLength   = 2
	ax25DestinationOffset     = 1
	ax25MinimumFrameLength    = ax25DestinationOffset + ax25AddressLength*ax25AddressCountMinimum + ax25ControlAndPIDLength
	ax25AddressRepeatedMask   = 0x80
	ax25LastAddressMask       = 0x01
	ax25AddressSsidMask       = 0x0f
	ax25AddressSsidShift      = 1
	ax25AddressShift          = 1
	ax25FrameFlag             = 0xc0
	ax25EscapeByte            = 0xdb
	ax25EscapedFrameFlag      = 0xdc
	ax25EscapedEscapeByte     = 0xdd
	positionTimestampLength   = 7
	latitudeLength            = 7
	longitudeLength           = 8
	latitudeHemisphereOffset  = latitudeLength
	symbolTableOffset         = latitudeHemisphereOffset + 1
	longitudeOffset           = symbolTableOffset + 1
	longitudeHemisphereOffset = longitudeOffset + longitudeLength
	symbolCodeOffset          = longitudeHemisphereOffset + 1
	uncompressedPositionLen   = symbolCodeOffset + 1
	minimumPositionDataLength = 10
	latitudeDegreeLength      = 2
	longitudeDegreeLength     = 3
	latitudeLimit             = 90
	longitudeLimit            = 180
	timestampedPositionOffset = 8
	base91CharacterMinimum    = '!'
	base91CharacterMaximum    = '}'
	base91TelemetryWidth      = 2
	base91TelemetryMinimum    = 4
	base91TelemetryMaximum    = 14
	base91TelemetryMaxValue   = 8280
	telemetryAnalogChannels   = 5
	telemetryDigitalChannels  = 6
	weatherHumidityFullScale  = 100
	weatherRainInchesToMM     = 0.254
	weatherTemperatureOffset  = 32.0
	weatherCelsiusNumerator   = 5.0
	weatherCelsiusDenominator = 9.0
	objectNameLength          = 9
	objectPositionOffset      = 18
	itemNameMinimumLength     = 3
	itemNameMaximumLength     = 9
	messageAddresseeLength    = 9
	messageSeparatorOffset    = 10
	messageTextOffset         = 11
	messageNumberMaxLength    = 5
	gridSquareLength          = 4
	gridSubsquareLength       = 6
	timestampMaximumDay       = 31
	timestampMaximumHour      = 23
	timestampMaximumMinute    = 59
	maidenheadFieldLongitude  = 20
	maidenheadFieldLatitude   = 10
	maidenheadSquareLongitude = 2
	maidenheadSubsquareScale  = 12
	maidenheadSubsquareYScale = 24
	compressedLatitudeScale   = 380926.0
	compressedLongitudeScale  = 190463.0

	dataTypePositionNoMessaging   = '!'
	dataTypePositionMessaging     = '='
	dataTypePositionTimestamped   = '@'
	dataTypePositionNoMessageTime = '/'
	dataTypeObject                = ';'
	dataTypeItem                  = ')'
	dataTypeMessage               = ':'
	dataTypeStatus                = '>'
	dataTypeQuery                 = '?'
	dataTypeWeather               = '_'
	dataTypeGridLocator           = '['
	dataTypeThirdParty            = '}'
	dataTypeMicECurrent           = '`'
	dataTypeMicEOld               = '\''
	dataTypeMicECurrentControl    = 0x1c
	dataTypeMicEOldControl        = 0x1d

	coordinateDecimalPointOffset = 2
	coordinateMinuteDigitCount   = 4
	coordinateMinutesLength      = coordinateMinuteDigitCount + 1
	coordinateAmbiguityMaximum   = 4
	aprsMinutesPerDegree         = 60.0
	positionAltitudePrefix       = "/A="
	positionPHGPrefix            = "PHG"
	weatherUnitSeparator         = 'w'
	nmeaAltitudeToFeet           = 3.280839895
	nmeaMetersUnit               = "M"
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
		if value == ax25FrameFlag {
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
			case ax25EscapedFrameFlag:
				value = ax25FrameFlag
			case ax25EscapedEscapeByte:
				value = ax25EscapeByte
			default:
				return nil, fmt.Errorf("invalid KISS escape byte 0x%02x", value)
			}
			d.escaped = false
		} else if value == ax25EscapeByte {
			d.escaped = true
			continue
		}
		d.frame = append(d.frame, value)
	}
}

func Parse(frame []byte) (Message, bool) {
	if len(frame) < ax25MinimumFrameLength || frame[0]&ax25CommandMask != ax25DataCommand {
		return Message{}, false
	}
	addresses := []string{}
	offset := ax25DestinationOffset
	for offset+ax25AddressLength <= len(frame) {
		addresses = append(addresses, DecodeCallsign(frame[offset:offset+ax25AddressLength]))
		last := frame[offset+ax25AddressExtension]&1 == 1
		offset += ax25AddressLength
		if last {
			break
		}
	}
	if len(addresses) < ax25AddressCountMinimum || offset+ax25ControlAndPIDLength > len(frame) || frame[offset] != ax25ControlField || frame[offset+1] != ax25NoLayer3Protocol {
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
	if position := parseSpecialPosition(payload, destination); position != nil {
		return position
	}
	start := positionStart(payload)
	if start < 0 || len(payload) < start+minimumPositionDataLength {
		return nil
	}
	timestamp := positionTimestamp(payload, start)
	if start == timestampedPositionOffset && timestamp == "" {
		return nil
	}
	return parsePositionData(payload[start:], timestamp)
}

func parseSpecialPosition(payload string, destination []string) *Position {
	if strings.HasPrefix(payload, "$") {
		return parseNMEAPosition(payload)
	}
	if len(payload) > 0 && (payload[0] == dataTypeMicECurrent || payload[0] == dataTypeMicEOld) && len(destination) > 0 {
		return parseMicEPosition(destination[0], payload)
	}
	if strings.HasPrefix(payload, string(dataTypeObject)) && len(payload) >= objectPositionOffset+minimumPositionDataLength {
		return parsePositionObject(payload)
	}
	if strings.HasPrefix(payload, string(dataTypeItem)) {
		return parsePositionItem(payload)
	}
	return nil
}

func parsePositionObject(payload string) *Position {
	return ParsePosition(string(dataTypePositionNoMessaging) + payload[objectPositionOffset:])
}

func parsePositionItem(payload string) *Position {
	end := strings.IndexAny(payload[ax25DestinationOffset:], string([]byte{dataTypePositionNoMessaging, dataTypeWeather}))
	itemNameLength := end
	if itemNameLength < itemNameMinimumLength || itemNameLength > itemNameMaximumLength {
		return nil
	}
	return ParsePosition(string(dataTypePositionNoMessaging) + payload[itemNameLength+ax25DestinationOffset+1:])
}

func positionTimestamp(payload string, start int) string {
	if start != timestampedPositionOffset {
		return ""
	}
	timestamp := payload[ax25DestinationOffset : ax25DestinationOffset+positionTimestampLength]
	if validPositionTimestamp(timestamp) {
		return timestamp
	}
	return ""
}

func parsePositionData(value, timestamp string) *Position {
	if len(value) > 0 && isDigits(value[:1]) {
		if position := parseUncompressedPosition(value, timestamp); position != nil {
			return position
		}
		if looksLikeUncompressedPosition(value) {
			return nil
		}
	}
	if isCompressedPosition(value) {
		return parseCompressedPosition(value, timestamp)
	}
	return nil
}

func looksLikeUncompressedPosition(value string) bool {
	return len(value) >= uncompressedPositionLen &&
		value[latitudeDegreeLength+coordinateDecimalPointOffset] == '.' &&
		(value[latitudeHemisphereOffset] == 'N' || value[latitudeHemisphereOffset] == 'S') &&
		isSymbolTable(value[symbolTableOffset]) &&
		value[longitudeOffset+longitudeDegreeLength+coordinateDecimalPointOffset] == '.' &&
		(value[longitudeHemisphereOffset] == 'E' || value[longitudeHemisphereOffset] == 'W')
}

func isCompressedPosition(value string) bool {
	return len(value) >= 10 && isSymbolTable(value[0]) && isBase91(value[1:9])
}

func parseCompressedPosition(value, timestamp string) *Position {
	y, x := base91(value[1:5]), base91(value[5:9])
	lat, lon := float64(latitudeLimit)-float64(y)/compressedLatitudeScale, -float64(longitudeLimit)+float64(x)/compressedLongitudeScale
	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return nil
	}
	position := &Position{Latitude: lat, Longitude: lon, SymbolTable: string(value[0]), SymbolCode: string(value[9]), Timestamp: timestamp}
	return positionMetadata(position, value[10:])
}

func parseUncompressedPosition(value, timestamp string) *Position {
	if len(value) < uncompressedPositionLen {
		return nil
	}
	lat, latAmbiguity, ok := coordinateWithAmbiguity(value[:latitudeLength], true)
	if !ok {
		return nil
	}
	if value[latitudeHemisphereOffset] != 'N' && value[latitudeHemisphereOffset] != 'S' {
		return nil
	}
	lon, lonAmbiguity, ok := coordinateWithAmbiguity(value[longitudeOffset:longitudeHemisphereOffset], false)
	if !ok {
		return nil
	}
	if value[longitudeHemisphereOffset] != 'E' && value[longitudeHemisphereOffset] != 'W' {
		return nil
	}
	if !isSymbolTable(value[symbolTableOffset]) {
		return nil
	}
	if latAmbiguity != lonAmbiguity {
		return nil
	}
	if value[latitudeHemisphereOffset] == 'S' {
		lat = -lat
	}
	if value[longitudeHemisphereOffset] == 'W' {
		lon = -lon
	}
	return positionMetadata(&Position{Latitude: lat, Longitude: lon, SymbolTable: string(value[symbolTableOffset]), SymbolCode: string(value[symbolCodeOffset]), Timestamp: timestamp, Ambiguity: latAmbiguity}, value[symbolCodeOffset+1:])
}

func parseMicEPosition(destination, payload string) *Position {
	if len(payload) < 10 || (payload[0] != dataTypeMicECurrent && payload[0] != dataTypeMicEOld) {
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
	longitude += longitudeLimit
	latitude += latitudeLimit
	fieldLon, fieldLat := int(longitude/maidenheadFieldLongitude), int(latitude/maidenheadFieldLatitude)
	longitude -= float64(fieldLon * maidenheadFieldLongitude)
	latitude -= float64(fieldLat * maidenheadFieldLatitude)
	squareLon, squareLat := int(longitude/maidenheadSquareLongitude), int(latitude)
	longitude -= float64(squareLon * maidenheadSquareLongitude)
	latitude -= float64(squareLat)
	subLon, subLat := int(longitude*maidenheadSubsquareScale), int(latitude*maidenheadSubsquareYScale)
	return string([]byte{'A' + byte(fieldLon), 'A' + byte(fieldLat), '0' + byte(squareLon), '0' + byte(squareLat), 'A' + byte(subLon), 'A' + byte(subLat)})
}

func positionStart(payload string) int {
	if payload == "" {
		return -1
	}
	switch payload[0] {
	case dataTypePositionNoMessaging, dataTypePositionMessaging:
		return 1
	case dataTypePositionNoMessageTime, dataTypePositionTimestamped:
		if len(payload) < timestampedPositionOffset {
			return -1
		}
		return timestampedPositionOffset
	default:
		return 0
	}
}

func validPositionTimestamp(value string) bool {
	if len(value) != positionTimestampLength {
		return false
	}
	switch value[positionTimestampLength-1] {
	case 'z':
		return validDigits(value[:positionTimestampLength-1], timestampMaximumDay, timestampMaximumHour, timestampMaximumMinute)
	case 'h':
		return validDigits(value[:positionTimestampLength-1], 0, timestampMaximumHour, timestampMaximumMinute)
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
		if char < base91CharacterMinimum || char > base91CharacterMaximum {
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
	if value := objectOrItemType(payload[0]); value != "" {
		return value
	}
	if weather != nil || payload[0] == dataTypeWeather || position != nil && position.SymbolCode == "_" {
		return "weather"
	}
	if value := positionPacketType(payload, position); value != "" {
		return value
	}
	if payload[0] == dataTypeMessage {
		return messagePacketType(payload)
	}
	if value := telemetryPacketType(payload); value != "" {
		return value
	}
	return otherPacketType(payload[0], payload)
}

func objectOrItemType(identifier byte) string {
	switch identifier {
	case dataTypeObject:
		return "object"
	case dataTypeItem:
		return "item"
	default:
		return ""
	}
}

func positionPacketType(payload string, position *Position) string {
	switch payload[0] {
	case dataTypePositionNoMessaging, dataTypePositionMessaging:
		if position == nil {
			return "raw-weather"
		}
	case dataTypePositionNoMessageTime, dataTypePositionTimestamped:
		if position == nil {
			return "unknown"
		}
	default:
		return ""
	}
	if position.SymbolTable == "/" && position.SymbolCode == "\\" {
		return "df"
	}
	return "position"
}

func messagePacketType(payload string) string {
	data := ParseAPRSMessage(payload)
	if data == nil {
		return "message"
	}
	text := strings.ToLower(data.Text)
	if strings.HasPrefix(text, "ack") {
		return "ack"
	}
	if strings.HasPrefix(text, "rej") {
		return "reject"
	}
	addressee := strings.ToUpper(data.Addressee)
	if strings.HasPrefix(addressee, "NWS-") {
		return "weather-bulletin"
	}
	if strings.HasPrefix(addressee, "BLN") {
		if len(addressee) > 3 && addressee[3] >= 'A' && addressee[3] <= 'Z' {
			return "announcement"
		}
		return "bulletin"
	}
	return "message"
}

func telemetryPacketType(payload string) string {
	for _, prefix := range []string{"T#", "PARM.", "UNIT.", "EQNS.", "BITS."} {
		if strings.HasPrefix(payload, prefix) {
			return "telemetry"
		}
	}
	return ""
}

func otherPacketType(identifier byte, payload string) string {
	switch identifier {
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
	case dataTypeMicEOld, dataTypeMicECurrent, dataTypeMicEOldControl, dataTypeMicECurrentControl:
		return "mic-e"
	case '<':
		return "capabilities"
	case dataTypeStatus:
		return "status"
	case dataTypeQuery:
		return "query"
	case '[':
		return "maidenhead"
	case '{':
		return "user-defined"
	case '|':
		return "fault"
	case '}':
		return "third-party"
	case '+':
		return "shelter"
	case '.':
		return "space-weather"
	case ',', '"', '(', '-', '\\', ']', '^':
		return "reserved"
	}
	if identifier >= '0' && identifier <= '9' || identifier >= 'A' && identifier <= 'S' || identifier >= 'U' && identifier <= 'Z' || identifier == '~' {
		return "reserved"
	}
	return "unknown"
}

func ParseTelemetry(payload string) *Telemetry {
	if strings.HasPrefix(payload, "T#") {
		return parseTraditionalTelemetry(payload[2:])
	}
	if telemetry := parseTelemetryDefinition(payload); telemetry != nil {
		return telemetry
	}
	return parseBase91Telemetry(payload)
}

func parseTraditionalTelemetry(payload string) *Telemetry {
	parts := strings.Split(payload, ",")
	if len(parts) < 7 {
		return nil
	}
	sequence, label := parseTelemetrySequence(parts[0])
	telemetry := &Telemetry{Sequence: sequence, SequenceLabel: label, Digital: strings.TrimSpace(parts[6])}
	for _, value := range parts[1 : telemetryAnalogChannels+1] {
		number, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil {
			return nil
		}
		telemetry.AnalogValues = append(telemetry.AnalogValues, number)
		telemetry.Analog = append(telemetry.Analog, int(number))
	}
	return telemetry
}

func parseTelemetryDefinition(payload string) *Telemetry {
	if len(payload) < 5 {
		return nil
	}
	kind, values, ok := telemetryDefinitionParts(payload)
	if !ok {
		return nil
	}
	telemetry := &Telemetry{}
	switch kind {
	case "PARM":
		telemetry.Parameters = values
	case "UNIT":
		telemetry.Units = values
	case "BITS":
		telemetry.Bits = strings.TrimSpace(strings.TrimPrefix(payload, "BITS."))
	case "EQNS":
		telemetry.Coefficients = parseTelemetryCoefficients(values)
		if telemetry.Coefficients == nil {
			return nil
		}
	}
	return telemetry
}

func telemetryDefinitionParts(payload string) (string, []string, bool) {
	for _, kind := range []string{"PARM", "UNIT", "EQNS", "BITS"} {
		if strings.HasPrefix(payload, kind+".") {
			return kind, strings.Split(strings.TrimSpace(payload[5:]), ","), true
		}
	}
	return "", nil, false
}

func parseTelemetryCoefficients(values []string) [][]float64 {
	if len(values) == 0 || len(values)%3 != 0 {
		return nil
	}
	var coefficients [][]float64
	for index := 0; index < len(values); index += 3 {
		coefficientSet, ok := parseCoefficientSet(values[index : index+3])
		if !ok {
			return nil
		}
		coefficients = append(coefficients, coefficientSet)
	}
	return coefficients
}

func parseCoefficientSet(values []string) ([]float64, bool) {
	coefficients := make([]float64, len(values))
	for index, value := range values {
		parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil {
			return nil, false
		}
		coefficients[index] = parsed
	}
	return coefficients, true
}

func parseBase91Telemetry(payload string) *Telemetry {
	if start := strings.Index(payload, "|"); start >= 0 {
		if end := strings.Index(payload[start+1:], "|"); end >= 0 {
			return parseBase91TelemetryValues(payload[start+1 : start+1+end])
		}
	}
	return nil
}

func parseBase91TelemetryValues(encoded string) *Telemetry {
	if len(encoded) < base91TelemetryMinimum || len(encoded)%base91TelemetryWidth != 0 ||
		len(encoded) > base91TelemetryMaximum || !isBase91(encoded) {
		return nil
	}
	values := make([]int, len(encoded)/base91TelemetryWidth)
	for index := range values {
		start := index * base91TelemetryWidth
		values[index] = base91(encoded[start : start+base91TelemetryWidth])
		if values[index] > base91TelemetryMaxValue {
			return nil
		}
	}
	telemetry := &Telemetry{Sequence: values[0], Base91: true}
	channelCount := len(values) - 1
	if channelCount > telemetryDigitalChannels {
		return nil
	}
	if channelCount == telemetryDigitalChannels {
		channelCount--
		telemetry.Digital = base91DigitalBits(values[len(values)-1])
	}
	for _, value := range values[1 : 1+channelCount] {
		telemetry.Analog = append(telemetry.Analog, value)
		telemetry.AnalogValues = append(telemetry.AnalogValues, float64(value))
	}
	return telemetry
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
	degrees, minutesText, ok := splitCoordinate(value, latitude)
	if !ok {
		return 0, 0, false
	}
	minutes, ambiguity, ok := ambiguousMinutes(minutesText)
	if !ok {
		return 0, 0, false
	}
	if !coordinateInBounds(degrees, minutes, latitude) {
		return 0, 0, false
	}
	return float64(degrees) + minutes/aprsMinutesPerDegree, ambiguity, true
}

func splitCoordinate(value string, latitude bool) (int, string, bool) {
	degreeWidth := latitudeDegreeLength
	expectedLength := latitudeLength
	if !latitude {
		degreeWidth, expectedLength = longitudeDegreeLength, longitudeLength
	}
	if len(value) != expectedLength {
		return 0, "", false
	}
	if !isDigits(value[:degreeWidth]) {
		return 0, "", false
	}
	degrees, err := strconv.Atoi(value[:degreeWidth])
	return degrees, value[degreeWidth:], err == nil
}

func ambiguousMinutes(value string) (float64, int, bool) {
	if len(value) != coordinateMinutesLength || value[coordinateDecimalPointOffset] != '.' {
		return 0, 0, false
	}
	ambiguity := strings.Count(value, " ")
	if ambiguity > coordinateAmbiguityMaximum || !validAmbiguitySpaces(value, ambiguity) {
		return 0, 0, false
	}
	minutes, err := strconv.ParseFloat(strings.ReplaceAll(value, " ", "0"), 64)
	if err != nil || minutes >= 60 {
		return 0, 0, false
	}
	return minutes + ambiguityOffset(ambiguity), ambiguity, true
}

func validAmbiguitySpaces(value string, ambiguity int) bool {
	allowed := [5]bool{}
	switch ambiguity {
	case 0:
	case 1:
		allowed[3] = true
	case 2:
		allowed[2], allowed[3] = true, true
	case 3:
		allowed[1], allowed[2], allowed[3] = true, true, true
	case 4:
		allowed[0], allowed[1], allowed[2], allowed[3] = true, true, true, true
	}
	for index, char := range []byte{value[0], value[1], value[3], value[4]} {
		if char == ' ' && !allowed[index] || char != ' ' && (char < '0' || char > '9') {
			return false
		}
	}
	return true
}

func ambiguityOffset(ambiguity int) float64 {
	switch ambiguity {
	case 1:
		return 0.005
	case 2:
		return 0.05
	case 3:
		return 5
	case 4:
		return 30
	default:
		return 0
	}
}

func coordinateInBounds(degrees int, minutes float64, latitude bool) bool {
	maxDegrees := latitudeLimit
	if !latitude {
		maxDegrees = longitudeLimit
	}
	return degrees >= 0 && degrees <= maxDegrees && !(degrees == maxDegrees && minutes > 0) && minutes < aprsMinutesPerDegree
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
	found := parseWeatherTemperature(payload, weather)
	found = parseWeatherSlashWind(payload, weather) || found
	found = parseWeatherFields(payload, weather) || found
	weather.Software, weather.Unit = parseWeatherStationInfo(payload)
	if !found {
		return nil
	}
	return weather
}

func parseWeatherTemperature(payload string, weather *Weather) bool {
	match := weatherPatterns["temperature"].FindStringSubmatch(payload)
	if len(match) != 2 {
		return false
	}
	value, _ := strconv.ParseFloat(match[1], 64)
	celsius := (value - weatherTemperatureOffset) * weatherCelsiusNumerator / weatherCelsiusDenominator
	weather.TemperatureC = &celsius
	return true
}

func parseWeatherSlashWind(payload string, weather *Weather) bool {
	match := weatherPatterns["windSlash"].FindStringSubmatch(payload)
	if len(match) != 3 {
		return false
	}
	direction, _ := strconv.Atoi(match[1])
	speed, _ := strconv.Atoi(match[2])
	weather.WindDirection = &direction
	weather.WindSpeedKnots, weather.WindKnots = &speed, &speed
	return true
}

func parseWeatherFields(payload string, weather *Weather) bool {
	names := make([]string, 0, len(weatherPatterns))
	for name := range weatherPatterns {
		names = append(names, name)
	}
	sort.Strings(names)
	found := false
	for _, name := range names {
		if name == "temperature" || name == "windSlash" {
			continue
		}
		found = applyWeatherPattern(payload, weather, name) || found
	}
	return found
}

func applyWeatherPattern(payload string, weather *Weather, name string) bool {
	match := weatherPatterns[name].FindStringSubmatch(payload)
	if len(match) != 2 {
		return false
	}
	value, _ := strconv.Atoi(match[1])
	switch name {
	case "direction", "windDirection":
		weather.WindDirection = &value
	case "speed":
		weather.WindSpeedKnots, weather.WindSpeedMph, weather.WindKnots = &value, &value, &value
	case "gust":
		weather.GustKnots, weather.GustMph = &value, &value
	case "rainHour":
		weather.RainLastHour, weather.RainLastHourMm = rainValue(value)
	case "rain24":
		weather.Rain24Hours, weather.Rain24HoursMm = rainValue(value)
	case "rainMidnight":
		weather.RainSinceMidnight, weather.RainSinceMidnightMm = rainValue(value)
	case "humidity":
		weather.Humidity = humidityValue(value)
	case "pressure":
		pressure := float64(value) / 10
		weather.PressureHpa = &pressure
	case "luminosity":
		weather.Luminosity = &value
	case "luminosityHigh":
		value += 1000
		weather.Luminosity = &value
	}
	return true
}

func rainValue(value int) (*int, *float64) {
	millimeters := float64(value) * weatherRainInchesToMM
	return &value, &millimeters
}

func humidityValue(value int) *int {
	if value == 0 {
		value = weatherHumidityFullScale
	}
	return &value
}

var weatherFields = regexp.MustCompile(`(?i)(\d{3}/\d{3}|[cdgprs]\d{3,4}|t-?\d{3,4}|P\d{3,4}|b\d{5,6}|h\d{2,3}|[Ll]\d{3})`)

func ParseWeatherForMessage(payload string, position *Position) *Weather {
	if strings.HasPrefix(payload, "_") || position != nil && position.SymbolCode == "_" {
		return ParseWeather(payload)
	}
	return nil
}

func parseWeatherStationInfo(payload string) (string, string) {
	marker := strings.LastIndexByte(payload, weatherUnitSeparator)
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
	ssid := (address[ax25AddressExtension] >> ax25AddressSsidShift) & ax25AddressSsidMask
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
	address := make([]byte, ax25AddressLength)
	call = strings.ToUpper(call)
	for i := 0; i < ax25AddressLength-1; i++ {
		value := byte(' ')
		if i < len(call) {
			value = call[i]
		}
		address[i] = value << ax25AddressShift
	}
	address[ax25AddressExtension] = 0x60 | ((ssid & ax25AddressSsidMask) << ax25AddressSsidShift)
	if last {
		address[ax25AddressExtension] |= ax25LastAddressMask
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
