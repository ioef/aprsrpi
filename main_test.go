package main

import (
	"testing"

	"aprsrpi/internal/aprs"
)

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
