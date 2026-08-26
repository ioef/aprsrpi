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
	"aprsrpi/internal/policy"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	settings := loadConfig()
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
	go isClient.Run(ctx, func(line string) {
		if message, ok := aprs.ParseTNC2(line); ok {
			hub.Publish(message)
			if settings.IGate.Enabled && policy.AllowInternetMessage(message, heard, settings.IGate.MessageGate) && limiter.Allow("is-to-rf", time.Minute/6) && windowLimiter.Allow("is-to-rf-minute", settings.IGate.MaxPerMinute, time.Minute) && windowLimiter.Allow("is-to-rf-five-minute", settings.IGate.MaxPerFiveMinutes, 5*time.Minute) {
				gated := aprs.Message{Source: settings.APRSIS.Callsign, Destination: "APRS", Payload: "}" + aprs.TNC2(message)}
				if err := radio.Send(aprs.EncodePacket(gated)); err != nil {
					log.Printf("internet-to-RF gate: %v", err)
				}
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

func receiveLoop(ctx context.Context, hub *gateway.Hub, settings config.Config, radio *radioWriter, isClient *aprsis.Client, seenRF, seenDigi *policy.Cache, limiter *policy.Limiter, windowLimiter *policy.WindowLimiter, heard *policy.Heard) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		device, err := openKISS(settings.KISS.Endpoint, settings.KISS.Baud)
		if err != nil {
			log.Printf("KISS connection failed: %v", err)
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
				hub.Publish(message)
				heard.Mark(message.Source)
				fingerprint := message.Source + "|" + message.Destination + "|" + message.Payload
				if settings.APRSIS.Enabled && filter.Match(settings.IGate.RFFilter, message) && !seenRF.Seen(fingerprint) {
					if err := isClient.Send(rfUpload(message, settings.APRSIS.Callsign)); err != nil {
						log.Printf("RF-to-Internet gate: %v", err)
					}
				}
				if settings.Digipeater.Enabled && !seenDigi.Seen(fingerprint) && limiter.Allow("rf-digi", time.Duration(settings.Digipeater.RateLimitSeconds)*time.Second) && windowLimiter.Allow("rf-digi-minute", settings.Digipeater.MaxPerMinute, time.Minute) && windowLimiter.Allow("rf-digi-five-minute", settings.Digipeater.MaxPerFiveMinutes, 5*time.Minute) {
					if repeated, ok := aprs.Digipeat(frame, settings.Digipeater.Callsign, settings.Digipeater.Aliases, settings.Digipeater.MaxHops); ok {
						if err := radio.Send(repeated); err != nil {
							log.Printf("digipeater transmit: %v", err)
						}
					}
				}
				if err := bot.Handle(radio, message, bot.Config{Callsign: settings.Bot.Callsign, Location: settings.Bot.Location, WeatherCity: settings.Bot.WeatherCity, Repeaters: settings.Bot.Repeaters, Sunrise: settings.Bot.Sunrise, Sunset: settings.Bot.Sunset, OpenWeatherAPIKey: settings.Bot.OpenWeatherAPIKey}); err != nil {
					log.Printf("bot reply failed: %v", err)
				}
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
	_, err := r.device.Write(frame)
	return err
}
func (r *radioWriter) Write(frame []byte) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.device == nil {
		return 0, fmt.Errorf("radio is not connected")
	}
	return r.device.Write(frame)
}

func rfUpload(message aprs.Message, igateCall string) string {
	if igateCall == "" {
		igateCall = "SV2JLD"
	}
	message.Path = strings.TrimSpace(message.Path)
	if message.Path != "" {
		message.Path += " > qAR > " + igateCall
	} else {
		message.Path = "qAR > " + igateCall
	}
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
	return config.WithDefaults(settings)
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
