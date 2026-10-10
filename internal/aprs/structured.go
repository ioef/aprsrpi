package aprs

import (
	"math"
	"strconv"
	"strings"
)

func parseNMEAPosition(payload string) *Position {
	fields := strings.Split(payload[1:], ",")
	if len(fields) == 0 {
		return nil
	}
	kind := fields[0]
	if checksum := strings.IndexByte(kind, '*'); checksum >= 0 {
		kind = kind[:checksum]
	}
	latitudeIndex, longitudeIndex := -1, -1
	switch {
	case strings.HasSuffix(kind, "GGA"):
		if len(fields) < 11 || fields[6] == "" || fields[6] == "0" {
			return nil
		}
		latitudeIndex, longitudeIndex = 2, 4
	case strings.HasSuffix(kind, "GLL"):
		if len(fields) < 7 || fields[6] != "A" {
			return nil
		}
		latitudeIndex, longitudeIndex = 1, 3
	case strings.HasSuffix(kind, "RMC"):
		if len(fields) < 10 || fields[2] != "A" {
			return nil
		}
		latitudeIndex, longitudeIndex = 3, 5
	default:
		return nil
	}
	latitude, ok := nmeaCoordinate(fields[latitudeIndex], fields[latitudeIndex+1], true)
	if !ok {
		return nil
	}
	longitude, ok := nmeaCoordinate(fields[longitudeIndex], fields[longitudeIndex+1], false)
	if !ok {
		return nil
	}
	position := &Position{Latitude: latitude, Longitude: longitude, SymbolTable: "/", SymbolCode: ">"}
	if strings.HasSuffix(kind, "GGA") && len(fields) > 9 && fields[10] == "M" {
		if altitude, err := strconv.ParseFloat(fields[9], 64); err == nil {
			feet := int(math.Round(altitude * 3.280839895))
			position.AltitudeFt = &feet
		}
	}
	return positionMetadata(position, "")
}

func nmeaCoordinate(value, hemisphere string, latitude bool) (float64, bool) {
	raw, err := strconv.ParseFloat(value, 64)
	if err != nil || raw < 0 {
		return 0, false
	}
	degrees := int(raw / 100)
	minutes := raw - float64(degrees*100)
	maxDegrees := 90
	if !latitude {
		maxDegrees = 180
	}
	if degrees > maxDegrees || minutes >= 60 || degrees == maxDegrees && minutes != 0 {
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
	result := float64(degrees) + minutes/60
	if negative {
		result = -result
	}
	return result, true
}

func ParseAPRSMessage(payload string) *APRSMessage {
	if len(payload) < 11 || payload[0] != ':' || payload[10] != ':' {
		return nil
	}
	data := &APRSMessage{
		Addressee: strings.TrimSpace(payload[1:10]),
		Text:      payload[11:],
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
	if len(payload) < 19 || payload[0] != ';' || payload[10] != '*' && payload[10] != '_' {
		return nil
	}
	timestamp := payload[11:18]
	if !validPositionTimestamp(timestamp) {
		return nil
	}
	position := ParsePosition("!"+payload[18:], destination)
	if position != nil {
		position.Timestamp = timestamp
	}
	return &ObjectReport{
		Name:      strings.TrimRight(payload[1:10], " "),
		Alive:     payload[10] == '*',
		Timestamp: timestamp,
		Position:  position,
	}
}

func ParseGridLocator(payload string) string {
	if len(payload) < 4 {
		return ""
	}
	locator := strings.ToUpper(payload[:4])
	if locator[0] < 'A' || locator[0] > 'R' || locator[1] < 'A' || locator[1] > 'R' ||
		locator[2] < '0' || locator[2] > '9' || locator[3] < '0' || locator[3] > '9' {
		return ""
	}
	if len(payload) >= 6 && isGridSubsquare(payload[4:6]) {
		locator += strings.ToLower(payload[4:6])
		if len(payload) > 6 && payload[6] != ' ' {
			return ""
		}
		return locator
	}
	if len(payload) > 4 && payload[4] != ' ' {
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
		if char >= 'A' && char <= 'X' || char >= 'a' && char <= 'x' {
			continue
		}
		return false
	}
	return true
}

func isMessageNumber(value string) bool {
	if len(value) == 0 || len(value) > 5 {
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
