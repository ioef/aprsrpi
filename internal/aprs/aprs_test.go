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

func TestParseCompressedPositionCommentAndTelemetry(t *testing.T) {
	message, ok := ParseTNC2("SV2AHT-13>APNU19:!L:5b/T.H/#  GLoRa APRS|(i${|")
	if !ok || message.Position == nil {
		t.Fatalf("compressed position was not decoded: %+v", message)
	}
	if message.Position.Comment != "LoRa APRS" {
		t.Fatalf("comment = %q, want %q", message.Position.Comment, "LoRa APRS")
	}
	if message.Telemetry == nil || !message.Telemetry.Base91 {
		t.Fatalf("base91 telemetry was not decoded: %+v", message.Telemetry)
	}
}

func TestParsePHGPositionComment(t *testing.T) {
	message, ok := ParseTNC2("SV2AHT-11>APNU19:!4006.43NT02227.59E#PHG3390/[ HRT tel:2310 310649-24hours ] .")
	if !ok || message.Position == nil {
		t.Fatalf("position was not decoded: %+v", message)
	}
	if message.Position.Comment != "/[ HRT tel:2310 310649-24hours ] ." {
		t.Fatalf("comment = %q", message.Position.Comment)
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
		if message.Kind != "weather" || message.Type != "weather" {
			t.Fatalf("classification = kind=%s type=%s", message.Kind, message.Type)
		}
	}
}

func TestParseCompressedOverlayWeatherPacket(t *testing.T) {
	packet := "XX2HRW-10>APLRG1:=L9yA3T??b_ !G.../...g...t061h55b10123Digi & i-Gate Batt=4.17V"
	message, ok := ParseTNC2(packet)
	if !ok {
		t.Fatal("compressed overlay weather packet was rejected")
	}
	if message.Position == nil || message.Position.SymbolTable != "L" || message.Position.SymbolCode != "_" {
		t.Fatalf("compressed overlay position was not decoded: %+v", message.Position)
	}
	if message.Weather == nil || message.Weather.TemperatureC == nil || message.Weather.Humidity == nil || message.Weather.PressureHpa == nil {
		t.Fatalf("weather fields were not decoded: %+v", message.Weather)
	}
	if message.Kind != "weather" || message.Type != "weather" {
		t.Fatalf("classification = kind=%s type=%s, want weather", message.Kind, message.Type)
	}
}

func TestClassifiesKnownPacketTypes(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		want    string
	}{
		{"position without timestamp", "!4037.93N/02257.06E>comment", "position"},
		{"position with timestamp", "@101024z4037.93N/02257.06E>comment", "position"},
		{"weather position", "@101024z4037.93N/02257.06E_000/000g000t073h18b10220", "weather"},
		{"standalone weather", "_t073h18b10220", "weather"},
		{"raw GPS", "$GPGGA,123519", "raw-gps"},
		{"Agrelo DFJr", "%DFJR", "agrelo-dfjr"},
		{"Peet Bros", "&PEET", "peet-bros"},
		{"Mic-E", "`abc", "mic-e"},
		{"message", ":N0CALL   :Hello", "message"},
		{"message acknowledgement", ":N0CALL   :ack123", "ack"},
		{"message rejection", ":N0CALL   :rej123", "reject"},
		{"object", ";OBJECT   *101024z4037.93N/02257.06E>", "object"},
		{"item", ")ITEM!4037.93N/02257.06E>", "item"},
		{"capabilities", "<capabilities", "capabilities"},
		{"status", ">status", "status"},
		{"query", "?APRSD", "query"},
		{"telemetry data", "T#001,001,002,003,004,005,00000000", "telemetry"},
		{"telemetry parameters", "PARM.Battery,Temperature", "telemetry"},
		{"telemetry units", "UNIT.V,degC", "telemetry"},
		{"telemetry equations", "EQNS.0,1,0", "telemetry"},
		{"telemetry bits", "BITS.11111111,Digital", "telemetry"},
		{"Maidenhead locator", "[AB12", "maidenhead"},
		{"user-defined", "{test", "user-defined"},
		{"fault", "|test", "fault"},
		{"third-party", "}N0CALL>APRS:>status", "third-party"},
		{"reserved identifier", "\"unused", "reserved"},
		{"unknown identifier", "xunsupported", "unknown"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			message, ok := ParseTNC2("N0CALL>APRS:" + test.payload)
			if !ok {
				t.Fatal("ParseTNC2 rejected packet")
			}
			if message.Type != test.want {
				t.Fatalf("Type = %q, want %q", message.Type, test.want)
			}
		})
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

func TestStandardDataTypeIdentifiers(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		want    string
	}{
		{"positionless weather", "_t073h22b10210", "weather"},
		{"raw Ultimeter weather", "$ULTW0031003702CE", "raw-weather"},
		{"Peet Bros weather", "#50B7500820082", "raw-weather"},
		{"Peet Bros alternate", "*7007600000000", "raw-weather"},
		{"shelter data", "+092345zShelter", "shelter"},
		{"space weather", ".Solar data", "space-weather"},
		{"legacy Peet Bros marker", "&PEET", "peet-bros"},
		{"reserved digit", "0unused", "reserved"},
		{"legacy unknown lowercase", "xunsupported", "unknown"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			message, ok := ParseTNC2("N0CALL>APRS:" + test.payload)
			if !ok || message.Type != test.want {
				t.Fatalf("parsed type = %q, want %q (ok=%t)", message.Type, test.want, ok)
			}
		})
	}
}

func TestParseAmbiguousCoordinatesAndTimestamp(t *testing.T) {
	message, ok := ParseTNC2("N0CALL>APRS:@101647z49  .  N/072  .  W>comment")
	if !ok || message.Position == nil {
		t.Fatalf("ambiguous position was not parsed: %+v", message)
	}
	if message.Position.Ambiguity != 4 || math.Abs(message.Position.Latitude-49.5) > 1e-9 || math.Abs(message.Position.Longitude+72.5) > 1e-9 {
		t.Fatalf("position = %+v; want center of 1-degree ambiguity cell", message.Position)
	}
	if message.Position.Timestamp != "101647z" {
		t.Fatalf("timestamp = %q", message.Position.Timestamp)
	}

	if position := ParsePosition("!9100.00N/18100.00E>"); position != nil {
		t.Fatalf("out-of-range coordinates accepted: %+v", position)
	}
	if position := ParsePosition("@321299z4034.78N/02300.72E>"); position != nil {
		t.Fatalf("invalid timestamp accepted: %+v", position)
	}
}

func TestParseObjectsItemsAndGridLocators(t *testing.T) {
	object, ok := ParseTNC2("N0CALL>APRS:;LEADER   *092345z/5L!!<*e7>7P[")
	if !ok || object.Type != "object" || object.Object == nil || !object.Object.Alive || object.Object.Name != "LEADER" || object.Object.Position == nil {
		t.Fatalf("compressed object was not decoded: %+v", object)
	}

	killed, ok := ParseTNC2("N0CALL>APRS:;LEADER   _092345z4903.50N/07201.75W>")
	if !ok || killed.Object == nil || killed.Object.Alive {
		t.Fatalf("killed object was not decoded: %+v", killed)
	}

	item, ok := ParseTNC2("N0CALL>APRS:)AID #2!4903.50N/07201.75WA")
	if !ok || item.Type != "item" || item.Position == nil || item.Position.SymbolCode != "A" {
		t.Fatalf("item report was not decoded: %+v", item)
	}

	grid, ok := ParseTNC2("N0CALL>APRS:[IO91SX")
	if !ok || grid.GridLocator != "IO91sx" {
		t.Fatalf("six-character locator = %q", grid.GridLocator)
	}
	grid, ok = ParseTNC2("N0CALL>APRS:[IO91 comment")
	if !ok || grid.GridLocator != "IO91" {
		t.Fatalf("four-character locator = %q", grid.GridLocator)
	}
}

func TestParseMessageMetadataAndThirdPartyPacket(t *testing.T) {
	message, ok := ParseTNC2("N0CALL>APRS::N1CALL   :Hello{123}ABC")
	if !ok || !message.IsMessage || message.APRSMessage == nil {
		t.Fatalf("message not decoded: %+v", message)
	}
	if message.APRSMessage.Addressee != "N1CALL" || message.APRSMessage.Text != "Hello" ||
		message.APRSMessage.Number != "123" || message.APRSMessage.ReplyAck != "ABC" {
		t.Fatalf("message details = %+v", message.APRSMessage)
	}

	bulletin, ok := ParseTNC2("N0CALL>APRS::BLN3     :Snow expected")
	if !ok || bulletin.Type != "bulletin" || bulletin.APRSMessage == nil {
		t.Fatalf("bulletin not decoded: %+v", bulletin)
	}
	announcement, ok := ParseTNC2("N0CALL>APRS::BLNQ     :Event notice")
	if !ok || announcement.Type != "announcement" {
		t.Fatalf("announcement not decoded: %+v", announcement)
	}

	status, ok := ParseTNC2("N0CALL>APRS:>operational")
	if !ok || status.StatusText != "operational" {
		t.Fatalf("status text = %q", status.StatusText)
	}
	query, ok := ParseTNC2("N0CALL>APRS:?APRS")
	if !ok || query.Query != "APRS" {
		t.Fatalf("query text = %q", query.Query)
	}

	thirdParty, ok := ParseTNC2("IGATE>APRS:}N0CALL>APRS:>status")
	if !ok || thirdParty.Type != "third-party" || thirdParty.ThirdParty == nil || thirdParty.ThirdParty.Type != "status" {
		t.Fatalf("third-party packet not decoded: %+v", thirdParty)
	}
}

func TestParseExtendedWeatherAndTelemetry(t *testing.T) {
	weather := ParseWeather("_10090556c220s004g005t077r001p002P003h00b09900L250l123")
	if weather == nil || weather.WindDirection == nil || *weather.WindDirection != 220 ||
		weather.RainSinceMidnight == nil || *weather.RainSinceMidnight != 3 ||
		weather.Luminosity == nil || *weather.Luminosity != 1123 || weather.Humidity == nil || *weather.Humidity != 100 {
		t.Fatalf("extended weather fields = %+v", weather)
	}

	telemetry := ParseTelemetry("T#MIC199,45.7,2.3,190.0,91.0,-7.3,00001100")
	if telemetry == nil || telemetry.SequenceLabel != "MIC199" || len(telemetry.AnalogValues) != 5 || telemetry.AnalogValues[0] != 45.7 {
		t.Fatalf("fractional telemetry = %+v", telemetry)
	}

	base91 := ParseTelemetry("comment |ss1122334455!\"|")
	if base91 == nil || !base91.Base91 || base91.Sequence != base91Value("ss") || len(base91.Analog) != 5 || base91.Digital != "10000000" {
		t.Fatalf("base91 telemetry = %+v", base91)
	}

	definition := ParseTelemetry("EQNS.0,1,0,0,2,0")
	if definition == nil || len(definition.Coefficients) != 2 || definition.Coefficients[1][1] != 2 {
		t.Fatalf("telemetry coefficients = %+v", definition)
	}

	telemetryMessage, ok := ParseTNC2("N0CALL>APRS::N0CALL   :PARM.Battery,Temperature")
	if !ok || telemetryMessage.Type != "message" || telemetryMessage.Telemetry == nil ||
		len(telemetryMessage.Telemetry.Parameters) != 2 {
		t.Fatalf("telemetry parameter message = %+v", telemetryMessage)
	}
}

func base91Value(value string) int {
	result := 0
	for _, char := range value {
		result = result*91 + int(char-'!')
	}
	return result
}

func TestParsePharmacyWeatherFromAPRSISAndRF(t *testing.T) {
	packet := "SV2SNY-2>APAGW,TCPIP,qAS,SV2SNY:@101123z4037.93N/02257.06E_000/000g000t073r000p000P000h22b10210 THESSALONIKI KAMARA PHARMACY"
	fromIS, ok := ParseTNC2(packet)
	if !ok || fromIS.Position == nil || fromIS.Weather == nil {
		t.Fatalf("APRS-IS weather packet not parsed: %+v", fromIS)
	}
	if fromIS.Source != "SV2SNY-2" || fromIS.Position.Timestamp != "101123z" ||
		fromIS.Weather.TemperatureC == nil || fromIS.Weather.Humidity == nil ||
		fromIS.Weather.PressureHpa == nil || fromIS.Weather.RainSinceMidnight == nil {
		t.Fatalf("weather fields incomplete: %+v", fromIS)
	}
	if *fromIS.Weather.Humidity != 22 || *fromIS.Weather.PressureHpa != 1021 {
		t.Fatalf("decoded weather = %+v", fromIS.Weather)
	}

	wire, err := NewDecoder(bytes.NewReader(EncodePacket(fromIS))).Next()
	if err != nil {
		t.Fatal(err)
	}
	fromRF, ok := Parse(wire)
	if !ok || fromRF.Type != fromIS.Type || fromRF.Source != fromIS.Source || fromRF.Position == nil ||
		fromRF.Weather == nil || *fromRF.Weather.PressureHpa != *fromIS.Weather.PressureHpa {
		t.Fatalf("RF/APRS-IS interpretations differ: is=%+v rf=%+v", fromIS, fromRF)
	}
}

func TestObjectWeatherRetainsObjectType(t *testing.T) {
	message, ok := ParseTNC2("N0CALL>APRS:;WXSTN    *101123z4037.93N/02257.06E_000/000g000t073h22b10210")
	if !ok || message.Type != "object" || message.Object == nil || message.Weather == nil {
		t.Fatalf("weather object was not parsed consistently: %+v", message)
	}
}

func TestParseNMEAPositionReports(t *testing.T) {
	tests := []struct {
		payload    string
		latitude   float64
		longitude  float64
		altitudeFt int
	}{
		{"$GPGGA,102705,5157.9762,N,00029.3256,W,1,04,2.0,75.7,M,47.6,M,,*62", 51.96627, -0.48876, 248},
		{"$GPGLL,2554.459,N,08020.187,W,154027.281,A", 25.90765, -80.33645, 0},
		{"$GPRMC,063909,A,3349.4302,N,11700.3721,W,43.022,89.3,291099,13.6,E*52", 33.8238367, -117.0062017, 0},
	}
	for _, test := range tests {
		message, ok := ParseTNC2("N0CALL>APRS:" + test.payload)
		if !ok || message.Type != "raw-gps" || message.Position == nil {
			t.Fatalf("NMEA position not parsed: %+v", message)
		}
		if math.Abs(message.Position.Latitude-test.latitude) > 0.00001 ||
			math.Abs(message.Position.Longitude-test.longitude) > 0.00001 {
			t.Fatalf("position = %+v; want %.7f, %.7f", message.Position, test.latitude, test.longitude)
		}
		if test.altitudeFt != 0 && (message.Position.AltitudeFt == nil || *message.Position.AltitudeFt != test.altitudeFt) {
			t.Fatalf("altitude = %+v; want %d ft", message.Position.AltitudeFt, test.altitudeFt)
		}
		if position := ParsePosition("$GPGGA,102705,5157.9762,N,00029.3256,W,1,04,2.0,75.7"); position != nil {
			t.Fatalf("truncated GGA report accepted: %+v", position)
		}
	}
}

func TestPositionExtensionsAndDF(t *testing.T) {
	position := ParsePosition("!4903.50N/07201.75W>088/036/A=001234 comment")
	if position == nil || position.Course == nil || *position.Course != 88 || position.SpeedKnots == nil ||
		*position.SpeedKnots != 36 || position.AltitudeFt == nil || *position.AltitudeFt != 1234 {
		t.Fatalf("position extensions = %+v", position)
	}

	df, ok := ParseTNC2("N0CALL>APRS:!4903.50N/07201.75W\\088/036/270/729")
	if !ok || df.Type != "df" || df.Position == nil || df.Position.DFBearing == nil ||
		*df.Position.DFBearing != 270 || *df.Position.DFNumber != 7 ||
		*df.Position.DFRange != 2 || *df.Position.DFQuality != 9 {
		t.Fatalf("DF report not parsed: %+v", df)
	}
}

func TestPositionlessWeatherCapturesStationInfo(t *testing.T) {
	weather := ParseWeather("_10090556c220s004g005t077r000p000P000h50b09900wRSW")
	if weather == nil || weather.Software != "" || weather.Unit != "RSW" {
		t.Fatalf("weather station metadata = %+v", weather)
	}
}
