package aprs

import (
	"bytes"
	"math"
	"testing"
)

func TestDecoderUnescapesFrame(t *testing.T) {
	decoder := NewDecoder(bytes.NewReader([]byte{0xc0, 0x00, 0x41, 0xdb, 0xdc, 0xc0}))
	frame, err := decoder.Next()
	if err != nil {
		t.Fatal(err)
	}
	if string(frame) != "\x00A\xc0" {
		t.Fatalf("decoded frame = %q", frame)
	}
}

func TestParseAX25Message(t *testing.T) {
	frame := []byte{0}
	frame = append(frame, Address("APRS", 0, false)...)
	frame = append(frame, Address("N0CALL", 7, true)...)
	frame = append(frame, 0x03, 0xf0)
	frame = append(frame, []byte(":WXREPORT: 21C wind 10kt")...)
	message, ok := Parse(frame)
	if !ok {
		t.Fatal("Parse rejected valid frame")
	}
	if message.Source != "N0CALL-7" || message.Destination != "APRS" {
		t.Fatalf("addresses = %s -> %s", message.Source, message.Destination)
	}
	if message.Payload != ":WXREPORT: 21C wind 10kt" {
		t.Fatalf("payload = %q", message.Payload)
	}
}

func TestEncodeMessageRoundTrip(t *testing.T) {
	decoder := NewDecoder(bytes.NewReader(EncodeMessage("N0CALL-7", "SV2JLD", "HELLO")))
	frame, err := decoder.Next()
	if err != nil {
		t.Fatal(err)
	}
	message, ok := Parse(frame)
	if !ok {
		t.Fatal("encoded message did not parse")
	}
	if message.Source != "SV2JLD" || message.Destination != "N0CALL-7" {
		t.Fatalf("addresses = %s -> %s", message.Source, message.Destination)
	}
}

func TestDigipeatRewritesFinalWideAlias(t *testing.T) {
	frame := []byte{0}
	frame = append(frame, Address("APRS", 0, false)...)
	frame = append(frame, Address("N0CALL", 0, false)...)
	frame = append(frame, Address("WIDE1", 1, true)...)
	frame = append(frame, 0x03, 0xf0, '!')
	repeated, ok := Digipeat(frame, "SV2JLD", []string{"WIDE1-1"})
	if !ok {
		t.Fatal("did not match WIDE1-1")
	}
	message, ok := Parse(repeated)
	if !ok || message.Path != "SV2JLD" {
		t.Fatalf("rewritten path = %q", message.Path)
	}
}

func TestDigipeatDecrementsWideTwo(t *testing.T) {
	frame := []byte{0}
	frame = append(frame, Address("APRS", 0, false)...)
	frame = append(frame, Address("N0CALL", 0, false)...)
	frame = append(frame, Address("WIDE2", 2, true)...)
	frame = append(frame, 0x03, 0xf0, '!')
	repeated, ok := Digipeat(frame, "SV2JLD", []string{"WIDE2-2"})
	if !ok {
		t.Fatal("did not match WIDE2-2")
	}
	message, ok := Parse(repeated)
	if !ok || message.Path != "SV2JLD > WIDE2-1" {
		t.Fatalf("rewritten path = %q", message.Path)
	}
}

func TestParseTNC2AndEncodePacket(t *testing.T) {
	message, ok := ParseTNC2("N0CALL>APRS,WIDE1-1:>hello")
	if !ok || message.Path != "WIDE1-1" || message.Payload != ">hello" {
		t.Fatalf("parsed message = %+v", message)
	}
	encoded := EncodePacket(message)
	frame, err := NewDecoder(bytes.NewReader(encoded)).Next()
	if err != nil {
		t.Fatal(err)
	}
	roundTrip, ok := Parse(frame)
	if !ok || roundTrip.Source != "N0CALL" || roundTrip.Payload != ">hello" {
		t.Fatalf("round trip = %+v", roundTrip)
	}
}

func TestParseCompressedPosition(t *testing.T) {
	message, ok := ParseTNC2("N0CALL>APRS:!/5L!!<*e7>7P")
	if !ok || message.Position == nil {
		t.Fatal("compressed position was not decoded")
	}
	if message.Position.Latitude < -90 || message.Position.Latitude > 90 || message.Position.Longitude < -180 || message.Position.Longitude > 180 {
		t.Fatalf("invalid coordinates: %+v", message.Position)
	}
}

func TestParseStandardWeather(t *testing.T) {
	weather := ParseWeather("/18030.00N/02200.00Ed180t090g120s080r001p005h75b10132")
	if weather == nil || weather.TemperatureC == nil || weather.WindDirection == nil || weather.WindSpeedKnots == nil || weather.GustKnots == nil || weather.Humidity == nil || weather.PressureHpa == nil {
		t.Fatalf("incomplete weather: %+v", weather)
	}
}

func TestOverlayPositionIsNotWeather(t *testing.T) {
	message, ok := ParseTNC2("SV2HNH>APDW18,WIDE1-1:!4039.62NS02257.66E#PHG2480http://sv2hnh.no-ip.org:8901/")
	if !ok || message.Position == nil {
		t.Fatal("overlay position was not decoded")
	}
	if message.Position.SymbolTable != "S" || message.Position.SymbolCode != "#" {
		t.Fatalf("symbol = %q%q", message.Position.SymbolTable, message.Position.SymbolCode)
	}
	if message.Weather != nil || message.Kind == "weather" {
		t.Fatalf("non-weather packet classified as weather: %+v", message)
	}
}

func TestCleanPayloadKeepsMessageTextReadable(t *testing.T) {
	got := CleanPayload([]byte(":SV2JLD   :hello\r\n\xffworld"))
	if got != ":SV2JLD   :hello �world" {
		t.Fatalf("cleaned payload = %q", got)
	}
}

func TestParseTimestampedWeatherPackets(t *testing.T) {
	packets := []string{
		"SV2JU-2>APAGW,WIDE2-2,qAO,SV2HNH:@261626z4034.78N/02300.72E_245/004g008t089r000p000P000h19b10120 AGWTracker",
		"CW8081>APRS,TCPXX*,qAX,CWOP-3:@260601z4033.98N/02258.47E_000/000g000t079r000p000P000b10118h68L274.WD 31",
	}
	for _, packet := range packets {
		message, ok := ParseTNC2(packet)
		if !ok || message.Position == nil || message.Weather == nil {
			t.Fatalf("packet not decoded: %+v", message)
		}
		if message.Weather.TemperatureC == nil || message.Weather.PressureHpa == nil || message.Weather.Humidity == nil || message.Weather.WindSpeedKnots == nil {
			t.Fatalf("weather incomplete: %+v", message.Weather)
		}
		if message.Kind != "weather" || message.Type != "position" {
			t.Fatalf("classification = kind=%s type=%s", message.Kind, message.Type)
		}
	}
}

func TestParseMicEPositionFromRealPacket(t *testing.T) {
	packet := "N1ZZN-9>T2SP0W:`c_Vm6hk/`\"49}Jeff Mobile_%"
	message, ok := ParseTNC2(packet)
	if !ok || message.Position == nil {
		t.Fatal("Mic-E packet was not decoded")
	}
	if math.Abs(message.Position.Latitude-42.50116666666667) > 1e-9 {
		t.Fatalf("latitude = %v", message.Position.Latitude)
	}
	if math.Abs(message.Position.Longitude+71.12633333333332) > 1e-9 {
		t.Fatalf("longitude = %v", message.Position.Longitude)
	}
	if message.Position.SymbolTable != "/" || message.Position.SymbolCode != "k" {
		t.Fatalf("symbol = %q%q", message.Position.SymbolTable, message.Position.SymbolCode)
	}
	if message.Position.Comment != "Jeff Mobile_%" {
		t.Fatalf("comment = %q", message.Position.Comment)
	}
	if message.Position.MicEStatus == "" {
		t.Fatal("Mic-E status was not decoded")
	}
}

func TestNormalPositionIsNotClassifiedAsWeather(t *testing.T) {
	message, ok := ParseTNC2("SV2HNH>APRS:!4039.62NS02257.66E#PHG2480http://sv2hnh.no-ip.org:8901/")
	if !ok || message.Position == nil {
		t.Fatal("position report was not decoded")
	}
	if message.Weather != nil || message.Kind == "weather" {
		t.Fatalf("normal position classified as weather: %+v", message)
	}
}
