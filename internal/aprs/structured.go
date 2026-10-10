package aprs

import (
	"math"
	"strconv"
	"strings"
)

const (
	nmeaGGALatitudeIndex  = 2
	nmeaGGALongitudeIndex = 4
	nmeaGGAAltitudeIndex  = 9
	nmeaGLLLatitudeIndex  = 1
	nmeaGLLLongitudeIndex = 3
	nmeaGLLValidityIndex  = 6
	nmeaRMCLatitudeIndex  = 3
	nmeaRMCLongitudeIndex = 5
	nmeaRMCValidityIndex  = 2
	noNMEAAltitude        = -1
	nmeaDegreesPerMinute  = 60.0
	nmeaDegreesScale      = 100.0
	gridFieldMin          = 'A'
	gridFieldMax          = 'R'
	gridSquareMin         = '0'
	gridSquareMax         = '9'
	gridSubsquareMin      = 'A'
	gridSubsquareMax      = 'X'
	nmeaValidStatus       = "A"
)

func parseNMEAPosition(payload string) *Position {
	fields := strings.Split(payload[ax25DestinationOffset:], ",")
	if len(fields) == 0 {
		return nil
	}
	kind := fields[0]
	if checksum := strings.IndexByte(kind, '*'); checksum >= 0 {
		kind = kind[:checksum]
	}
	layout, ok := nmeaLayout(kind, fields)
	if !ok {
		return nil
	}
	latitude, ok := nmeaCoordinate(fields[layout.latitude], fields[layout.latitude+1], true)
	if !ok {
		return nil
	}
	longitude, ok := nmeaCoordinate(fields[layout.longitude], fields[layout.longitude+1], false)
	if !ok {
		return nil
	}
	position := &Position{Latitude: latitude, Longitude: longitude, SymbolTable: "/", SymbolCode: ">"}
	position.AltitudeFt = nmeaAltitude(fields, layout.altitude)
	return positionMetadata(position, "")
}

type nmeaFieldLayout struct {
	latitude  int
	longitude int
	altitude  int
}

func nmeaLayout(kind string, fields []string) (nmeaFieldLayout, bool) {
	switch {
	case strings.HasSuffix(kind, "GGA"):
		return nmeaGGAFields(fields)
	case strings.HasSuffix(kind, "GLL"):
		return nmeaGLLFields(fields)
	case strings.HasSuffix(kind, "RMC"):
		return nmeaRMCFields(fields)
	default:
		return nmeaFieldLayout{}, false
	}
}

func nmeaGGAFields(fields []string) (nmeaFieldLayout, bool) {
	if len(fields) <= nmeaGGAAltitudeIndex+1 || fields[6] == "" || fields[6] == "0" {
		return nmeaFieldLayout{}, false
	}
	return nmeaFieldLayout{latitude: nmeaGGALatitudeIndex, longitude: nmeaGGALongitudeIndex, altitude: nmeaGGAAltitudeIndex}, true
}

func nmeaGLLFields(fields []string) (nmeaFieldLayout, bool) {
	if len(fields) < nmeaGLLValidityIndex+1 || fields[nmeaGLLValidityIndex] != nmeaValidStatus {
		return nmeaFieldLayout{}, false
	}
	return nmeaFieldLayout{latitude: nmeaGLLLatitudeIndex, longitude: nmeaGLLLongitudeIndex, altitude: noNMEAAltitude}, true
}

func nmeaRMCFields(fields []string) (nmeaFieldLayout, bool) {
	if len(fields) < nmeaRMCValidityIndex+1 || fields[nmeaRMCValidityIndex] != nmeaValidStatus {
		return nmeaFieldLayout{}, false
	}
	return nmeaFieldLayout{latitude: nmeaRMCLatitudeIndex, longitude: nmeaRMCLongitudeIndex, altitude: noNMEAAltitude}, true
}

func nmeaAltitude(fields []string, index int) *int {
	if index < 0 || len(fields) <= index+1 || fields[index+1] != nmeaMetersUnit {
		return nil
	}
	altitude, err := strconv.ParseFloat(fields[index], 64)
	if err != nil {
		return nil
	}
	feet := int(math.Round(altitude * nmeaAltitudeToFeet))
	return &feet
}

func nmeaCoordinate(value, hemisphere string, latitude bool) (float64, bool) {
	raw, err := strconv.ParseFloat(value, 64)
	if err != nil || raw < 0 {
		return 0, false
	}
	degrees := int(raw / nmeaDegreesScale)
	minutes := raw - float64(degrees)*nmeaDegreesScale
	maxDegrees := latitudeLimit
	if !latitude {
		maxDegrees = longitudeLimit
	}
	if degrees > maxDegrees || minutes >= nmeaDegreesPerMinute || degrees == maxDegrees && minutes != 0 {
		return 0, false
	}
	negative := false
	if latitude {
		switch hemisphere {
		case "N":
		case "S":
			negative = true
		default:
			return 0, false
		}
	} else {
		switch hemisphere {
		case "E":
		case "W":
			negative = true
		default:
			return 0, false
		}
	}
	result := float64(degrees) + minutes/nmeaDegreesPerMinute
	if negative {
		result = -result
	}
	return result, true
}

func ParseAPRSMessage(payload string) *APRSMessage {
	if len(payload) <= messageTextOffset-1 || payload[0] != dataTypeMessage || payload[messageSeparatorOffset] != ':' {
		return nil
	}
	data := &APRSMessage{
		Addressee: strings.TrimSpace(payload[1:messageSeparatorOffset]),
		Text:      payload[messageTextOffset:],
	}
	if open := strings.LastIndex(data.Text, "{"); open >= 0 && open+1 < len(data.Text) {
		number := data.Text[open+1:]
		replyAck := ""
		if close := strings.IndexByte(number, '}'); close >= 0 {
			replyAck = number[close+1:]
			number = number[:close]
		}
		if isMessageNumber(number) && (replyAck == "" || isMessageNumber(replyAck)) {
			data.Text = data.Text[:open]
			data.Number = number
			data.ReplyAck = replyAck
		}
	}
	return data
}

func ParseObject(payload, destination string) *ObjectReport {
	if len(payload) < objectPositionOffset+1 || payload[0] != dataTypeObject || payload[objectNameLength+1] != '*' && payload[objectNameLength+1] != '_' {
		return nil
	}
	objectStateOffset := objectNameLength + 1
	timestampStart := objectStateOffset + 1
	positionOffset := objectPositionOffset
	timestamp := payload[timestampStart:positionOffset]
	if !validPositionTimestamp(timestamp) {
		return nil
	}
	position := ParsePosition(string(dataTypePositionNoMessaging)+payload[positionOffset:], destination)
	if position != nil {
		position.Timestamp = timestamp
	}
	return &ObjectReport{
		Name:      strings.TrimRight(payload[1:objectStateOffset], " "),
		Alive:     payload[objectStateOffset] == '*',
		Timestamp: timestamp,
		Position:  position,
	}
}

func ParseGridLocator(payload string) string {
	if len(payload) < gridSquareLength {
		return ""
	}
	locator := strings.ToUpper(payload[:gridSquareLength])
	if locator[0] < gridFieldMin || locator[0] > gridFieldMax || locator[1] < gridFieldMin || locator[1] > gridFieldMax ||
		locator[2] < gridSquareMin || locator[2] > gridSquareMax || locator[3] < gridSquareMin || locator[3] > gridSquareMax {
		return ""
	}
	if len(payload) >= gridSubsquareLength && isGridSubsquare(payload[gridSquareLength:gridSubsquareLength]) {
		locator += strings.ToLower(payload[gridSquareLength:gridSubsquareLength])
		if len(payload) > gridSubsquareLength && payload[gridSubsquareLength] != ' ' {
			return ""
		}
		return locator
	}
	if len(payload) > gridSquareLength && payload[gridSquareLength] != ' ' {
		return ""
	}
	return locator
}

func parseTelemetrySequence(value string) (int, string) {
	sequence, err := strconv.Atoi(value)
	if err == nil {
		return sequence, ""
	}
	return 0, strings.TrimSpace(value)
}

func isGridSubsquare(value string) bool {
	if len(value) != 2 {
		return false
	}
	for index := range value {
		char := value[index]
		if char >= gridSubsquareMin && char <= gridSubsquareMax || char >= 'a' && char <= 'x' {
			continue
		}
		return false
	}
	return true
}

func isMessageNumber(value string) bool {
	if len(value) == 0 || len(value) > messageNumberMaxLength {
		return false
	}
	for index := range value {
		char := value[index]
		if char >= '0' && char <= '9' || char >= 'A' && char <= 'Z' || char >= 'a' && char <= 'z' {
			continue
		}
		return false
	}
	return true
}
