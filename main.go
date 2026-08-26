package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"time"

	"aprsrpi/internal/aprs"
	"aprsrpi/internal/aprsis"
	"aprsrpi/internal/bot"
	"aprsrpi/internal/config"
	"aprsrpi/internal/filter"
	"aprsrpi/internal/gateway"
	"aprsrpi/internal/logging"
	"aprsrpi/internal/policy"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	settings := loadConfig()
	closeLog, err := logging.SetupWithLevel(settings.LogFile, settings.LogLevel)
	if err != nil {
		log.Fatalf("configure logging: %v", err)
	}
	defer closeLog()
	hub := gateway.NewHub()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	radio := &radioWriter{}
	seenRF := policy.NewCache(30 * time.Second)
	seenDigi := policy.NewCache(30 * time.Second)
	limiter := policy.NewLimiter()
	windowLimiter := policy.NewWindowLimiter()
	heard := policy.NewHeard(time.Duration(settings.IGate.HeardTimeoutMinutes) * time.Minute)
	isClient := aprsis.New(aprsis.Config{Enabled: settings.APRSIS.Enabled, Server: settings.APRSIS.Server, Callsign: settings.APRSIS.Callsign, Passcode: settings.APRSIS.Passcode, Filter: settings.APRSIS.Filter})
	if settings.APRSIS.Enabled {
		go stationBeacon(ctx, isClient, settings.Station)
	}
	go isClient.Run(ctx, func(line string) {
		logging.Debugf("aprs-is raw=%q", line)
		if message, ok := aprs.ParseTNC2(line); ok {
			hub.Publish(message)
			logging.Infof("aprs-is receive source=%s destination=%s type=%s payload=%q", message.Source, message.Destination, message.Type, message.Payload)
			logging.Debugf("aprs-is parsed source=%q destination=%q path=%q payload=%q position=%+v weather=%+v telemetry=%+v", message.Source, message.Destination, message.Path, message.Payload, message.Position, message.Weather, message.Telemetry)
			if settings.IGate.Enabled && policy.AllowInternetMessage(message, heard, settings.IGate.MessageGate) && limiter.Allow("is-to-rf", time.Minute/6) && windowLimiter.Allow("is-to-rf-minute", settings.IGate.MaxPerMinute, time.Minute) && windowLimiter.Allow("is-to-rf-five-minute", settings.IGate.MaxPerFiveMinutes, 5*time.Minute) {
				gated := aprs.Message{Source: settings.APRSIS.Callsign, Destination: "APRS", Payload: "}" + aprs.TNC2(message)}
				if err := radio.Send(aprs.EncodePacket(gated)); err != nil {
					logging.Warnf("internet-to-RF gate failed: %v", err)
				} else {
					logging.Infof("internet-to-RF gate sent source=%s destination=%s", message.Source, message.Destination)
				}
			} else {
				logging.Debugf("internet packet not gated source=%s destination=%s type=%s", message.Source, message.Destination, message.Type)
			}
		}
	})
	go receiveLoop(ctx, hub, settings, radio, isClient, seenRF, seenDigi, limiter, windowLimiter, heard)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/messages", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(hub.Snapshot())
	})
	mux.HandleFunc("/api/events", hub.Events)
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"kiss": settings.KISS.Endpoint, "mode": "live", "bot": settings.Bot.Callsign})
	})
	mux.Handle("/", http.FileServer(http.Dir(settings.WebRoot)))
	address := settings.HTTPAddress
	log.Printf("APRS kiosk listening on %s; KISS endpoint %s", address, settings.KISS.Endpoint)
	log.Fatal(http.ListenAndServe(address, mux))
}

func stationBeacon(ctx context.Context, client *aprsis.Client, station config.StationConfig) {
	if station.Latitude == 0 && station.Longitude == 0 {
		logging.Warnf("station beacon disabled: latitude/longitude are not configured")
		return
	}
	interval := time.Duration(station.BeaconMinutes) * time.Minute
	send := func() {
		payload := fmt.Sprintf("!%s%s%s%s%s", formatCoordinate(station.Latitude, true), station.SymbolTable, formatCoordinate(station.Longitude, false), station.SymbolCode, station.Comment)
		message := aprs.Message{Source: station.Callsign, Destination: "APRS", Payload: payload}
		if err := client.Send(aprs.TNC2(message)); err != nil {
			logging.Warnf("station beacon failed callsign=%s: %v", station.Callsign, err)
		} else {
			logging.Infof("station beacon sent callsign=%s", station.Callsign)
		}
	}
	send()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		send()
	}
}

func formatCoordinate(value float64, latitude bool) string {
	direction := "E"
	if latitude {
		direction = "N"
	}
	if value < 0 {
		if latitude {
			direction = "S"
		} else {
			direction = "W"
		}
		value = -value
	}
	degrees := int(value)
	minutes := (value - float64(degrees)) * 60
	if latitude {
		return fmt.Sprintf("%02d%05.2f%s", degrees, minutes, direction)
	}
	return fmt.Sprintf("%03d%05.2f%s", degrees, minutes, direction)
}

func receiveLoop(ctx context.Context, hub *gateway.Hub, settings config.Config, radio *radioWriter, isClient *aprsis.Client, seenRF, seenDigi *policy.Cache, limiter *policy.Limiter, windowLimiter *policy.WindowLimiter, heard *policy.Heard) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		device, err := openKISS(settings.KISS.Endpoint, settings.KISS.Baud)
		if err != nil {
			logging.Errorf("KISS connection failed: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}
		log.Printf("KISS connected: %s", settings.KISS.Endpoint)
		radio.Set(device)
		decoder := aprs.NewDecoder(device)
		for {
			frame, err := decoder.Next()
			if err != nil {
				log.Printf("KISS disconnected: %v", err)
				break
			}
			if message, ok := aprs.Parse(frame); ok {
				logging.Debugf("rf raw_kiss_frame=%x", frame)
				hub.Publish(message)
				logging.Infof("rf receive source=%s destination=%s type=%s payload=%q", message.Source, message.Destination, message.Type, message.Payload)
				logging.Debugf("rf parsed source=%q destination=%q path=%q payload=%q position=%+v weather=%+v telemetry=%+v", message.Source, message.Destination, message.Path, message.Payload, message.Position, message.Weather, message.Telemetry)
				heard.Mark(message.Source)
				fingerprint := message.Source + "|" + message.Destination + "|" + message.Payload
				rfDuplicate := seenRF.Seen(fingerprint)
				if settings.APRSIS.Enabled && filter.Match(settings.IGate.RFFilter, message) && !rfDuplicate {
					packet := rfUpload(message, settings.APRSIS.Callsign)
					if err := isClient.Send(packet); err != nil {
						logging.Warnf("rf-to-aprs-is failed source=%s destination=%s: %v", message.Source, message.Destination, err)
					} else {
						logging.Infof("rf-to-aprs-is sent source=%s destination=%s", message.Source, message.Destination)
					}
				} else {
					logging.Debugf("RF packet not uploaded enabled=%t filter=%q duplicate=%t", settings.APRSIS.Enabled, settings.IGate.RFFilter, rfDuplicate)
				}
				if settings.Digipeater.Enabled && !seenDigi.Seen(fingerprint) && limiter.Allow("rf-digi", time.Duration(settings.Digipeater.RateLimitSeconds)*time.Second) && windowLimiter.Allow("rf-digi-minute", settings.Digipeater.MaxPerMinute, time.Minute) && windowLimiter.Allow("rf-digi-five-minute", settings.Digipeater.MaxPerFiveMinutes, 5*time.Minute) {
					if repeated, ok := aprs.Digipeat(frame, settings.Digipeater.Callsign, settings.Digipeater.Aliases, settings.Digipeater.MaxHops); ok {
						if err := radio.Send(repeated); err != nil {
							logging.Warnf("digipeater transmit failed: %v", err)
						} else {
							logging.Infof("digipeater transmit source=%s", message.Source)
						}
					}
				}
				if err := bot.Handle(radio, message, bot.Config{Callsign: settings.Bot.Callsign, Location: settings.Bot.Location, WeatherCity: settings.Bot.WeatherCity, Repeaters: settings.Bot.Repeaters, Sunrise: settings.Bot.Sunrise, Sunset: settings.Bot.Sunset, OpenWeatherAPIKey: settings.Bot.OpenWeatherAPIKey}); err != nil {
					logging.Warnf("bot reply failed: %v", err)
				}
			} else {
				logging.Warnf("rf packet rejected: invalid KISS/AX.25 frame length=%d", len(frame))
			}
		}
		radio.Clear(device)
		_ = device.Close()
		time.Sleep(2 * time.Second)
	}
}

type radioWriter struct {
	mu     sync.RWMutex
	device io.Writer
}

func (r *radioWriter) Set(device io.Writer) { r.mu.Lock(); r.device = device; r.mu.Unlock() }
func (r *radioWriter) Clear(device io.Writer) {
	r.mu.Lock()
	if r.device == device {
		r.device = nil
	}
	r.mu.Unlock()
}
func (r *radioWriter) Send(frame []byte) error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.device == nil {
		return fmt.Errorf("radio is not connected")
	}
	logging.Debugf("kiss tx frame=%x", frame)
	_, err := r.device.Write(frame)
	return err
}
func (r *radioWriter) Write(frame []byte) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.device == nil {
		return 0, fmt.Errorf("radio is not connected")
	}
	logging.Debugf("kiss tx frame=%x", frame)
	return r.device.Write(frame)
}

func rfUpload(message aprs.Message, igateCall string) string {
	if igateCall == "" {
		igateCall = "SV2JLD"
	}
	path := make([]string, 0)
	items := strings.Split(message.Path, " > ")
	skipNext := false
	for _, item := range items {
		if skipNext {
			skipNext = false
			continue
		}
		item = strings.TrimSpace(item)
		upper := strings.ToUpper(strings.TrimSuffix(item, "*"))
		if item == "" || upper == "TCPIP" || upper == "TCPXX" || upper == "RFONLY" || upper == "NOGATE" {
			continue
		}
		if strings.HasPrefix(upper, "Q") {
			skipNext = true
			continue
		}
		path = append(path, item)
	}
	path = append(path, "qAR", igateCall)
	message.Path = strings.Join(path, " > ")
	return aprs.TNC2(message)
}

func loadConfig() config.Config {
	path := env("APRSRPI_CONFIG", "/etc/aprsrpi/config.json")
	settings, err := config.Load(path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Fatal(err)
		}
		log.Printf("config not found at %s; using defaults", path)
	}
	settings = config.WithDefaults(settings)
	if value := os.Getenv("APRSRPI_LOG_LEVEL"); value != "" {
		settings.LogLevel = value
	}
	if value := os.Getenv("APRSRPI_LOG_FILE"); value != "" {
		settings.LogFile = value
	}
	return settings
}
func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
func openKISS(endpoint string, baud int) (io.ReadWriteCloser, error) {
	if strings.HasPrefix(endpoint, "tcp://") {
		return net.Dial("tcp", strings.TrimPrefix(endpoint, "tcp://"))
	}
	path := strings.TrimPrefix(strings.TrimPrefix(endpoint, "serial://"), "bluetooth://")
	if err := exec.Command("stty", "-F", path, strconv.Itoa(baud), "raw", "-echo").Run(); err != nil {
		return nil, err
	}
	return os.OpenFile(path, os.O_RDWR, 0)
}
