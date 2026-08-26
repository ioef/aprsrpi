package policy

import (
	"testing"
	"time"

	"aprsrpi/internal/aprs"
)

func TestAllowInternetMessageRequiresRecentRFStation(t *testing.T) {
	heard := NewHeard(time.Minute)
	message := aprs.Message{Kind: "message", Source: "N0CALL", Destination: "APRS", Payload: ":REMOTE   :hello"}
	if AllowInternetMessage(message, heard, true) {
		t.Fatal("gated a station not heard on RF")
	}
	heard.Mark("REMOTE")
	if !AllowInternetMessage(message, heard, true) {
		t.Fatal("did not gate a recently heard station")
	}
	message.Path = "TCPIP"
	if AllowInternetMessage(message, heard, true) {
		t.Fatal("gated a prohibited path")
	}
}
