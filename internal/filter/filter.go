package filter

import (
	"math"
	"strconv"
	"strings"

	"aprsrpi/internal/aprs"
)

// Match evaluates the common APRS filter families used for station gating.
// Empty filters allow the packet; comma-separated terms are ORed.
func Match(expression string, message aprs.Message) bool {
	expression = strings.TrimSpace(expression)
	if expression == "" {
		return true
	}
	for _, group := range strings.Split(expression, ",") {
		matched := true
		for _, term := range strings.Split(group, "&") {
			term = strings.TrimSpace(term)
			negated := strings.HasPrefix(term, "!")
			if negated {
				term = strings.TrimSpace(strings.TrimPrefix(term, "!"))
			}
			result := matchTerm(term, message)
			if negated {
				result = !result
			}
			if !result {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func matchTerm(term string, message aprs.Message) bool {
	if term == "" {
		return false
	}
	term = strings.ToLower(term)
	switch term[0] {
	case 't':
		return len(term) > 1 && strings.Contains(term[1:], messageType(message))
	case 'p':
		return callsignList(term[1:], message.Source)
	case 'b':
		return callsignList(term[1:], message.Destination)
	case 'r', 'g':
		return radiusTerm(term[1:], message)
	case 's':
		return len(term) > 1 && message.Symbol != "" && strings.Contains(term[1:], strings.ToLower(message.Symbol))
	case 'o':
		return message.Type == "object" || message.Type == "item"
	case 'q':
		return qConstruct(term[1:], message.Path)
	default:
		return false
	}
}

func qConstruct(value, path string) bool {
	value = strings.TrimPrefix(value, "/")
	if value == "" {
		return false
	}
	for _, item := range strings.Split(value, "/") {
		if strings.EqualFold(strings.TrimSpace(item), "any") {
			return true
		}
		if strings.Contains(strings.ToLower(path), strings.ToLower(strings.TrimSpace(item))) {
			return true
		}
	}
	return false
}

func messageType(message aprs.Message) string {
	if message.IsMessage {
		return "m"
	}
	if message.Position != nil {
		return "p"
	}
	if message.Type == "object" || message.Type == "item" {
		return "o"
	}
	return "u"
}
func callsignList(value, call string) bool {
	for _, item := range strings.Split(value, "/") {
		if strings.EqualFold(strings.TrimSpace(item), call) {
			return true
		}
	}
	return false
}
func radiusTerm(value string, message aprs.Message) bool {
	if message.Position == nil {
		return false
	}
	value = strings.TrimPrefix(value, "/")
	parts := strings.Split(value, "/")
	if len(parts) != 3 {
		return false
	}
	lat, e1 := strconv.ParseFloat(parts[0], 64)
	lon, e2 := strconv.ParseFloat(parts[1], 64)
	radius, e3 := strconv.ParseFloat(parts[2], 64)
	if e1 != nil || e2 != nil || e3 != nil {
		return false
	}
	distance := haversineKm(lat, lon, message.Position.Latitude, message.Position.Longitude)
	return distance <= radius
}
func haversineKm(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadius = 6371.0
	dLat := radians(lat2 - lat1)
	dLon := radians(lon2 - lon1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(radians(lat1))*math.Cos(radians(lat2))*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * earthRadius * math.Asin(math.Sqrt(a))
}
func radians(value float64) float64 { return value * math.Pi / 180 }
