package filter

import (
	"testing"

	"aprsrpi/internal/aprs"
)

func TestMatchCommonFilterFamilies(t *testing.T) {
	message := aprs.Message{Source: "N0CALL", Destination: "APRS", IsMessage: true, Kind: "message", Payload: ":SV2JLD   :hello"}
	if !Match("t/m", message) {
		t.Fatal("message type filter did not match")
	}
	if !Match("p/N0CALL", message) {
		t.Fatal("source filter did not match")
	}
	if !Match("b/APRS", message) {
		t.Fatal("destination filter did not match")
	}
	if Match("p/OTHER", message) {
		t.Fatal("wrong source filter matched")
	}
	if !Match("t/m&!p/OTHER", message) {
		t.Fatal("combined filter did not match")
	}
	if Match("!t/m", message) {
		t.Fatal("negated filter matched")
	}
}

func TestMatchRadiusFilter(t *testing.T) {
	lat, lon := 40.64, 22.94
	message := aprs.Message{Position: &aprs.Position{Latitude: lat, Longitude: lon}}
	if !Match("r/40.640/22.940/1", message) {
		t.Fatal("near radius did not match")
	}
	if Match("r/41.640/22.940/1", message) {
		t.Fatal("far radius matched")
	}
}
