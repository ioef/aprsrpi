package aprs

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseTNC2 parses the text packet format used by APRS-IS servers.
func ParseTNC2(line string) (Message, bool) {
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
	message := Message{Source: strings.TrimSpace(header[0]), Destination: strings.TrimSpace(addresses[0]), Path: strings.Join(addresses[1:], " > "), Payload: CleanPayload([]byte(parts[1])), Raw: line, Kind: "packet", Icon: "radio"}
	message.Position = ParsePosition(message.Payload, message.Destination)
	if message.Position != nil && message.Position.SymbolCode == "_" {
		message.Weather = ParseWeather(message.Payload)
	}
	if message.Position != nil {
		message.Symbol = message.Position.SymbolTable + message.Position.SymbolCode
	}
	if message.Weather != nil {
		message.Kind, message.Icon = "weather", "weather"
	}
	if strings.HasPrefix(message.Payload, ":") {
		message.Kind, message.Icon = "message", "message"
		message.IsMessage = true
	}
	message.Type = packetType(message.Payload)
	return message, true
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
