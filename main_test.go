package main

import (
	"testing"

	"aprsrpi/internal/aprs"
	"aprsrpi/internal/gateway"
)

func TestPublishMessageKeepsWeatherPacketLatest(t *testing.T) {
	hub := gateway.NewHub()
	message := aprs.Message{
		Source:  "SV2HNH",
		Payload: "t073h18b10220",
		Weather: &aprs.Weather{},
		Type:    "weather",
	}

	publishMessage(hub, message)

	snapshot := hub.Snapshot()
	if len(snapshot) != 2 {
		t.Fatalf("published %d messages, want weather packet and summary", len(snapshot))
	}
	if snapshot[0].Payload != message.Payload || snapshot[0].Weather == nil {
		t.Fatalf("latest message = %+v, want original weather packet", snapshot[0])
	}
}

func TestRFUploadRewritesInternetPath(t *testing.T) {
	message := aprs.Message{Source: "SV2HNH", Destination: "APDW18", Path: "WIDE1-1 > qAO > SV4IMN-2", Payload: "!test"}
	got := rfUpload(message, "SV2JLD")
	want := "SV2HNH>APDW18,WIDE1-1,qAR,SV2JLD:!test"
	if got != want {
		t.Fatalf("rf upload = %q, want %q", got, want)
	}
}

func TestFormatCoordinate(t *testing.T) {
	if got := formatCoordinate(40.6401, true); got != "4038.41N" {
		t.Fatalf("latitude = %q", got)
	}
	if got := formatCoordinate(22.9444, false); got != "02256.66E" {
		t.Fatalf("longitude = %q", got)
	}
	if got := formatCoordinate(-22.9444, false); got != "02256.66W" {
		t.Fatalf("negative longitude = %q", got)
	}
}
