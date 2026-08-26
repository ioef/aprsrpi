package policy

import (
	"strings"

	"aprsrpi/internal/aprs"
)

func AllowInternetMessage(message aprs.Message, heard *Heard, enabled bool) bool {
	target, ok := messageTarget(message.Payload)
	if !enabled || message.Kind != "message" || !ok || !heard.Recent(target) {
		return false
	}
	path := strings.ToUpper(message.Path)
	for _, blocked := range []string{"TCPIP", "TCPXX", "RFONLY", "NOGATE", "QAX"} {
		if strings.Contains(path, blocked) {
			return false
		}
	}
	body := strings.TrimSpace(message.Payload)
	if len(body) >= 2 && body[0] == ':' {
		destination := strings.TrimSpace(body[1:10])
		if strings.HasPrefix(destination, "BLN") {
			return false
		}
	}
	return true
}

func messageTarget(payload string) (string, bool) {
	if len(payload) < 11 || payload[0] != ':' || payload[10] != ':' {
		return "", false
	}
	return strings.TrimSpace(payload[1:10]), true
}
