package bot

import (
	"bytes"
	"testing"

	"aprsrpi/internal/aprs"
)

func TestHandleRepliesOnlyToConfiguredCallsign(t *testing.T) {
	request := aprs.Message{Source: "N0CALL", Payload: ":SV2JLD   :HELP"}
	var output bytes.Buffer
	if err := Handle(&output, request, Config{Callsign: "SV2JLD"}); err != nil {
		t.Fatal(err)
	}
	response, ok := aprs.Parse(mustFrame(t, &output))
	if !ok || response.Destination != "N0CALL" || response.Source != "SV2JLD" {
		t.Fatalf("unexpected response: %+v", response)
	}

	output.Reset()
	request.Payload = ":OTHER   :HELP"
	if err := Handle(&output, request, Config{Callsign: "SV2JLD"}); err != nil {
		t.Fatal(err)
	}
	if output.Len() != 0 {
		t.Fatal("bot replied to a different callsign")
	}
}

func mustFrame(t *testing.T, input *bytes.Buffer) []byte {
	t.Helper()
	frame, err := aprs.NewDecoder(input).Next()
	if err != nil {
		t.Fatal(err)
	}
	return frame
}
