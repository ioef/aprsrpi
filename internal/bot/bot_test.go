package bot

import (
	"bytes"
	"net/http"
	"net/http/httptest"
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

func TestQuakeCommandsQueryUSGSAndFormatResponses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Query().Get("format") != "geojson" || request.URL.Query().Get("orderby") != "time" {
			t.Fatalf("unexpected USGS query: %s", request.URL.RawQuery)
		}
		if request.URL.Query().Get("limit") == "3" {
			_, _ = writer.Write([]byte(`{"features":[{"properties":{"mag":6.1,"place":"Alaska"}},{"properties":{"mag":5.4,"place":"Tonga"}},{"properties":{"mag":4.9,"place":"Greece"}}]}`))
			return
		}
		if request.URL.Query().Get("limit") == "1" && request.URL.Query().Get("minlatitude") == "34" && request.URL.Query().Get("maxlongitude") == "30" {
			_, _ = writer.Write([]byte(`{"features":[{"properties":{"mag":4.2,"place":"Dodecanese Islands, Greece"}}]}`))
			return
		}
		if request.URL.Query().Get("limit") == "1" && request.URL.Query().Get("minlatitude") == "36" && request.URL.Query().Get("maxlongitude") == "19" {
			_, _ = writer.Write([]byte(`{"features":[{"properties":{"mag":3.8,"place":"Italy"}}]}`))
			return
		}
		t.Fatalf("unexpected country bounds in USGS query: %s", request.URL.RawQuery)
	}))
	defer server.Close()

	originalURL := usgsEarthquakeURL
	usgsEarthquakeURL = server.URL
	defer func() { usgsEarthquakeURL = originalURL }()

	if response := commandResponse("QUAKE", "QUAKE", Config{}); response != "Latest quakes: M6.1 Alaska; M5.4 Tonga; M4.9 Greece" {
		t.Fatalf("unexpected global quake response: %q", response)
	}
	if response := commandResponse("QUAKE?GREECE", "QUAKE?GREECE", Config{}); response != "Latest GREECE quake: M4.2 Dodecanese Islands, Greece" {
		t.Fatalf("unexpected country quake response: %q", response)
	}
	if response := commandResponse("QUAKE?ITALY", "QUAKE?ITALY", Config{}); response != "Latest ITALY quake: M3.8 Italy" {
		t.Fatalf("unexpected country quake response: %q", response)
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
