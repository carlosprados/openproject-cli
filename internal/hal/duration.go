package hal

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var isoDuration = regexp.MustCompile(`^P(?:(\d+(?:\.\d+)?)D)?(?:T(?:(\d+(?:\.\d+)?)H)?(?:(\d+(?:\.\d+)?)M)?(?:(\d+(?:\.\d+)?)S)?)?$`)

// ParseHours accepts the duration spellings a human or an LLM is likely to
// type and returns decimal hours: "1.5", "1,5", "90m", "1h30m", "1h 30m",
// "2h", "PT1H30M". Days in ISO input ("P1D") count as 24h, as OpenProject does.
func ParseHours(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}
	if f, err := strconv.ParseFloat(strings.Replace(s, ",", ".", 1), 64); err == nil {
		return f, nil
	}
	upper := strings.ToUpper(s)
	if strings.HasPrefix(upper, "P") {
		if m := isoDuration.FindStringSubmatch(upper); m != nil && upper != "P" && upper != "PT" {
			f := func(i int) float64 { v, _ := strconv.ParseFloat(m[i], 64); return v }
			return f(1)*24 + f(2) + f(3)/60 + f(4)/3600, nil
		}
		return 0, fmt.Errorf("invalid ISO 8601 duration %q", s)
	}
	d, err := time.ParseDuration(strings.ReplaceAll(strings.ToLower(s), " ", ""))
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q (use 1.5, 90m, 1h30m or PT1H30M)", s)
	}
	return d.Hours(), nil
}

// ISOHours renders decimal hours as an ISO 8601 duration ("PT1H30M").
func ISOHours(h float64) string {
	mins := int(math.Round(h * 60))
	hh, mm := mins/60, mins%60
	switch {
	case mins == 0:
		return "PT0S"
	case mm == 0:
		return fmt.Sprintf("PT%dH", hh)
	case hh == 0:
		return fmt.Sprintf("PT%dM", mm)
	}
	return fmt.Sprintf("PT%dH%dM", hh, mm)
}

// HoursString renders an ISO duration from the API as "1.5h" for tables;
// non-duration input is returned unchanged.
func HoursString(v any) string {
	s, ok := v.(string)
	if !ok || s == "" {
		return String(v)
	}
	h, err := ParseHours(s)
	if err != nil {
		return s
	}
	return strconv.FormatFloat(math.Round(h*100)/100, 'f', -1, 64) + "h"
}
