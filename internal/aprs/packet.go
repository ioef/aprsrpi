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
	message.Position = ParsePosition(payload, destination)
	message.Weather = ParseWeatherForMessage(payload, message.Position)
	if message.Position != nil {
		message.Symbol = message.Position.SymbolTable + message.Position.SymbolCode
	}
	if message.Weather != nil {
		message.Kind, message.Icon = "weather", "weather"
	}
	message.Type = packetType(payload, message.Position, message.Weather)
	if firstByte == 0x1c || firstByte == 0x1d {
		message.Type = "mic-e"
	}
	message.Telemetry = ParseTelemetry(payload)
	message.APRSMessage = ParseAPRSMessage(payload)
	if message.APRSMessage != nil {
		if strings.HasPrefix(message.APRSMessage.Text, "PARM.") ||
			strings.HasPrefix(message.APRSMessage.Text, "UNIT.") ||
			strings.HasPrefix(message.APRSMessage.Text, "EQNS.") ||
			strings.HasPrefix(message.APRSMessage.Text, "BITS.") {
			message.Telemetry = ParseTelemetry(message.APRSMessage.Text)
		}
		message.IsMessage = true
		if message.Type == "message" || message.Type == "ack" || message.Type == "reject" || message.Type == "bulletin" || message.Type == "announcement" || message.Type == "weather-bulletin" {
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
		message.Symbol = ""
		if message.Position != nil {
			message.Symbol = message.Position.SymbolTable + message.Position.SymbolCode
		}
	}
	if strings.HasPrefix(payload, ">") {
		message.StatusText = strings.TrimSpace(payload[1:])
	}
	if strings.HasPrefix(payload, "?") {
		message.Query = strings.TrimSpace(payload[1:])
	}
	if strings.HasPrefix(payload, "[") {
		message.GridLocator = ParseGridLocator(payload[1:])
	}
	if strings.HasPrefix(payload, "}") {
		if thirdParty, ok := parseTNC2(payload[1:], depth+1); ok {
			message.ThirdParty = &thirdParty
		}
	}
	return message
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
		frame[14] |= 1
	}
	frame = append(frame, 0x03, 0xf0)
	frame = append(frame, []byte(message.Payload)...)
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

func Digipeat(frame []byte, callsign string, aliases []string, hopLimits ...int) ([]byte, bool) {
	offset := 1
	maxHops := 7
	if len(hopLimits) > 0 && hopLimits[0] > 0 {
		maxHops = hopLimits[0]
	}
	for addressIndex := 0; offset+7 <= len(frame); addressIndex++ {
		address := frame[offset : offset+7]
		last := address[6]&1 == 1
		if addressIndex >= 2 && address[6]&0x80 == 0 {
			name := DecodeCallsign(address)
			for _, alias := range aliases {
				if strings.EqualFold(name, strings.TrimSuffix(alias, "*")) {
					call, ssid := SplitCallsign(callsign)
					replacement := addressWithRepeated(call, ssid, true, last)
					if strings.HasPrefix(strings.ToUpper(name), "WIDE2-") {
						count, err := strconv.Atoi(strings.TrimPrefix(strings.ToUpper(name), "WIDE2-"))
						if err == nil && count > 1 {
							if count > maxHops {
								return nil, false
							}
							replacement = addressWithRepeated(call, ssid, true, false)
							remaining := addressWithRepeated("WIDE2", byte(count-1), false, last)
							result := make([]byte, len(frame)+7)
							copy(result, frame[:offset])
							copy(result[offset:offset+7], replacement)
							copy(result[offset+7:offset+14], remaining)
							copy(result[offset+14:], frame[offset+7:])
							return result, true
						}
					}
					copy(address, replacement)
					return append([]byte(nil), frame...), true
				}
			}
		}
		offset += 7
		if last {
			break
		}
	}
	return nil, false
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
		address[6] |= 0x80
	}
	return address
}
