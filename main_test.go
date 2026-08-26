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
