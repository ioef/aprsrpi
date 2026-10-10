package aprs

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ParseTNC2 parses the text packet format used by APRS-IS servers.
func ParseTNC2(line string) (Message, bool) {
	return parseTNC2(line, 0)
}

func parseTNC2(line string, depth int) (Message, bool) {
	if depth > 4 {
		return Message{}, false
	}
	parts := strings.SplitN(strings.TrimSpace(line), ":", 2)
	if len(parts) != 2 {
		return Message{}, false
	}
	header := strings.Split(parts[0], ">")
	if len(header) != 2 {
		return Message{}, false
	}
	addresses := strings.Split(header[1], ",")
	if len(addresses) == 0 || strings.TrimSpace(header[0]) == "" {
		return Message{}, false
	}
	return parseMessage(strings.TrimSpace(header[0]), strings.TrimSpace(addresses[0]), strings.Join(addresses[1:], " > "), parts[1], line, depth), true
}

func parseMessage(source, destination, path, payload, raw string, depth int) Message {
	firstByte := byte(0)
	if len(payload) > 0 {
		firstByte = payload[0]
	}
	payload = CleanPayload([]byte(payload))
	message := Message{
		Received:    time.Now().UTC().Format(time.RFC3339),
		Source:      source,
		Destination: destination,
		Path:        path,
		Payload:     payload,
		Raw:         raw,
		Kind:        "packet",
		Icon:        "radio",
	}
	applyPositionAndWeather(&message, payload, destination, firstByte)
	applyStructuredPayload(&message, payload, destination)
	applyPacketMetadata(&message, payload, depth)
	return message
}

func applyPositionAndWeather(message *Message, payload, destination string, firstByte byte) {
	message.Position = ParsePosition(payload, destination)
	message.Weather = ParseWeatherForMessage(payload, message.Position)
	if message.Position != nil {
		message.Symbol = message.Position.SymbolTable + message.Position.SymbolCode
	}
	if message.Weather != nil {
		message.Kind, message.Icon = "weather", "weather"
	}
	message.Type = packetType(payload, message.Position, message.Weather)
	if firstByte == dataTypeMicECurrentControl || firstByte == dataTypeMicEOldControl {
		message.Type = "mic-e"
	}
}

func applyStructuredPayload(message *Message, payload, destination string) {
	message.Telemetry = ParseTelemetry(payload)
	message.APRSMessage = ParseAPRSMessage(payload)
	if message.APRSMessage != nil {
		applyTelemetryMessage(message, message.APRSMessage.Text)
		message.IsMessage = true
		if isMessagePacketType(message.Type) {
			message.Kind, message.Icon = "message", "message"
		}
	}
	if strings.HasPrefix(payload, ":") {
		message.IsMessage = true
		message.Kind, message.Icon = "message", "message"
	}
	message.Object = ParseObject(payload, destination)
	if message.Object != nil {
		message.Position = message.Object.Position
		message.Symbol = symbolForPosition(message.Position)
	}
}

func applyTelemetryMessage(message *Message, text string) {
	for _, prefix := range []string{"PARM.", "UNIT.", "EQNS.", "BITS."} {
		if strings.HasPrefix(text, prefix) {
			message.Telemetry = ParseTelemetry(text)
			return
		}
	}
}

func isMessagePacketType(packetType string) bool {
	switch packetType {
	case "message", "ack", "reject", "bulletin", "announcement", "weather-bulletin":
		return true
	default:
		return false
	}
}

func symbolForPosition(position *Position) string {
	if position == nil {
		return ""
	}
	return position.SymbolTable + position.SymbolCode
}

func applyPacketMetadata(message *Message, payload string, depth int) {
	if strings.HasPrefix(payload, string(dataTypeStatus)) {
		message.StatusText = strings.TrimSpace(payload[ax25DestinationOffset:])
	}
	if strings.HasPrefix(payload, string(dataTypeQuery)) {
		message.Query = strings.TrimSpace(payload[ax25DestinationOffset:])
	}
	if strings.HasPrefix(payload, string(dataTypeGridLocator)) {
		message.GridLocator = ParseGridLocator(payload[ax25DestinationOffset:])
	}
	if strings.HasPrefix(payload, string(dataTypeThirdParty)) {
		thirdParty, ok := parseTNC2(payload[ax25DestinationOffset:], depth+1)
		if ok {
			message.ThirdParty = &thirdParty
		}
	}
}
func TNC2(message Message) string {
	header := message.Source + ">" + message.Destination
	if message.Path != "" {
		header += "," + strings.ReplaceAll(message.Path, " > ", ",")
	}
	return fmt.Sprintf("%s:%s", header, message.Payload)
}

func EncodePacket(message Message) []byte {
	destinationCall, destinationSSID := SplitCallsign(message.Destination)
	sourceCall, sourceSSID := SplitCallsign(message.Source)
	frame := []byte{0}
	frame = append(frame, Address(destinationCall, destinationSSID, false)...)
	frame = append(frame, Address(sourceCall, sourceSSID, false)...)
	path := strings.Split(message.Path, " > ")
	for index, item := range path {
		if strings.TrimSpace(item) == "" {
			continue
		}
		call, ssid, repeated := pathAddress(item)
		frame = append(frame, addressWithRepeated(call, ssid, repeated, index == len(path)-1)...)
	}
	if len(path) == 0 {
		frame[ax25DestinationOffset+2*ax25AddressLength-1] |= ax25LastAddressMask
	}
	frame = append(frame, ax25ControlField, ax25NoLayer3Protocol)
	frame = append(frame, []byte(message.Payload)...)
	encoded := []byte{ax25FrameFlag}
	for _, value := range frame {
		switch value {
		case ax25FrameFlag:
			encoded = append(encoded, ax25EscapeByte, ax25EscapedFrameFlag)
		case ax25EscapeByte:
			encoded = append(encoded, ax25EscapeByte, ax25EscapedEscapeByte)
		default:
			encoded = append(encoded, value)
		}
	}
	return append(encoded, ax25FrameFlag)
}

func Digipeat(frame []byte, callsign string, aliases []string, hopLimits ...int) ([]byte, bool) {
	offset := 1
	maxHops := digipeaterHopLimit(hopLimits)
	for addressIndex := 0; offset+ax25AddressLength <= len(frame); addressIndex++ {
		address := frame[offset : offset+ax25AddressLength]
		last := address[ax25AddressExtension]&ax25LastAddressMask != 0
		if repeatedAddress(addressIndex, address) {
			name := DecodeCallsign(address)
			if matchesAlias(name, aliases) {
				return rewriteMatchedAlias(frame, offset, address, callsign, name, maxHops)
			}
		}
		offset += ax25AddressLength
		if last {
			break
		}
	}
	return nil, false
}

func digipeaterHopLimit(hopLimits []int) int {
	if len(hopLimits) > 0 && hopLimits[0] > 0 {
		return hopLimits[0]
	}
	return 7
}

func repeatedAddress(index int, address []byte) bool {
	return index >= ax25AddressCountMinimum && address[ax25AddressExtension]&ax25AddressRepeatedMask == 0
}

func matchesAlias(name string, aliases []string) bool {
	for _, alias := range aliases {
		if strings.EqualFold(name, strings.TrimSuffix(alias, "*")) {
			return true
		}
	}
	return false
}

func rewriteMatchedAlias(frame []byte, offset int, address []byte, callsign, name string, maxHops int) ([]byte, bool) {
	last := address[ax25AddressExtension]&ax25LastAddressMask != 0
	count, wideTwo := wideTwoRemainingHops(name)
	call, ssid := SplitCallsign(callsign)
	if !wideTwo || count <= 1 {
		replacement := addressWithRepeated(call, ssid, true, last)
		copy(address, replacement)
		return append([]byte(nil), frame...), true
	}
	if count > maxHops {
		return nil, false
	}
	return insertRemainingWideTwo(frame, offset, call, ssid, count, last), true
}

func wideTwoRemainingHops(name string) (int, bool) {
	upperName := strings.ToUpper(name)
	if !strings.HasPrefix(upperName, "WIDE2-") {
		return 0, false
	}
	count, err := strconv.Atoi(strings.TrimPrefix(upperName, "WIDE2-"))
	return count, err == nil
}

func insertRemainingWideTwo(frame []byte, offset int, call string, ssid byte, count int, last bool) []byte {
	repeated := addressWithRepeated(call, ssid, true, false)
	remaining := addressWithRepeated("WIDE2", byte(count-1), false, last)
	result := make([]byte, len(frame)+ax25AddressLength)
	copy(result, frame[:offset])
	copy(result[offset:offset+ax25AddressLength], repeated)
	copy(result[offset+ax25AddressLength:offset+2*ax25AddressLength], remaining)
	copy(result[offset+2*ax25AddressLength:], frame[offset+ax25AddressLength:])
	return result
}

func pathAddress(value string) (string, byte, bool) {
	repeated := strings.HasSuffix(value, "*")
	value = strings.TrimSuffix(value, "*")
	call, ssid := SplitCallsign(value)
	return call, ssid, repeated
}
func addressWithRepeated(call string, ssid byte, repeated, last bool) []byte {
	address := Address(call, ssid, last)
	if repeated {
		address[ax25AddressExtension] |= ax25AddressRepeatedMask
	}
	return address
}
