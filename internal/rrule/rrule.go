package rrule

import (
	"fmt"
	"strings"
	"time"

	"github.com/teambition/rrule-go"
)

// ParseRRule parses an RFC 5545 RRULE string and returns the RRule object
func ParseRRule(ruleStr string, dtstart time.Time) (*rrule.RRule, error) {
	// Handle RRULE: prefix if present
	ruleStr = strings.TrimPrefix(ruleStr, "RRULE:")

	opt, err := rrule.StrToROption(ruleStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse RRULE: %w", err)
	}

	// Database stores TIMESTAMP without timezone, but pgx reads it as UTC.
	// The actual values are local time, so we need to reinterpret them.
	// Create a new time with the same clock values but in local timezone.
	opt.Dtstart = time.Date(
		dtstart.Year(), dtstart.Month(), dtstart.Day(),
		dtstart.Hour(), dtstart.Minute(), dtstart.Second(), dtstart.Nanosecond(),
		time.Local,
	)
	return rrule.NewRRule(*opt)
}

// NextOccurrence returns the next occurrence after the given time
// Returns nil if there are no more occurrences
func NextOccurrence(ruleStr string, dtstart time.Time, after time.Time) (*time.Time, error) {
	rule, err := ParseRRule(ruleStr, dtstart)
	if err != nil {
		return nil, err
	}

	next := rule.After(after, false)
	if next.IsZero() {
		return nil, nil
	}
	return &next, nil
}

// NextOccurrenceStrict returns the next occurrence strictly after the given time
// Use this when you need to skip the current occurrence
func NextOccurrenceStrict(ruleStr string, dtstart time.Time, after time.Time) (*time.Time, error) {
	rule, err := ParseRRule(ruleStr, dtstart)
	if err != nil {
		return nil, err
	}

	// Ensure 'after' is in local timezone for consistent comparison
	afterLocal := after.In(time.Local)

	// Keep searching until we find a time strictly after 'after'
	current := afterLocal
	for i := 0; i < 1000; i++ { // Safety limit
		next := rule.After(current, false)
		if next.IsZero() {
			return nil, nil
		}
		if next.After(afterLocal) {
			return &next, nil
		}
		// Move forward to search for the next one
		current = next.Add(time.Second)
	}

	return nil, nil
}

// NextOccurrences returns the next n occurrences after the given time
func NextOccurrences(ruleStr string, dtstart time.Time, after time.Time, count int) ([]time.Time, error) {
	rule, err := ParseRRule(ruleStr, dtstart)
	if err != nil {
		return nil, err
	}

	// Get occurrences starting from 'after' time
	iterator := rule.Iterator()
	var results []time.Time

	for {
		next, ok := iterator()
		if !ok {
			break
		}
		if next.After(after) {
			results = append(results, next)
			if len(results) >= count {
				break
			}
		}
	}

	return results, nil
}

// BuildRRule creates an RRULE string from components
type RRuleBuilder struct {
	Freq       rrule.Frequency
	Interval   int
	ByHour     []int
	ByMinute   []int
	ByWeekday  []rrule.Weekday
	ByMonthDay []int
	ByMonth    []int
	Count      int
	Until      *time.Time
}

// Common frequencies
const (
	FreqHourly  = rrule.HOURLY
	FreqDaily   = rrule.DAILY
	FreqWeekly  = rrule.WEEKLY
	FreqMonthly = rrule.MONTHLY
	FreqYearly  = rrule.YEARLY
)

// Weekday constants
var (
	Monday    = rrule.MO
	Tuesday   = rrule.TU
	Wednesday = rrule.WE
	Thursday  = rrule.TH
	Friday    = rrule.FR
	Saturday  = rrule.SA
	Sunday    = rrule.SU
)

func (b *RRuleBuilder) Build(dtstart time.Time) (*rrule.RRule, error) {
	opt := rrule.ROption{
		Freq:     b.Freq,
		Interval: b.Interval,
		Dtstart:  dtstart,
	}

	if len(b.ByHour) > 0 {
		opt.Byhour = b.ByHour
	}
	if len(b.ByMinute) > 0 {
		opt.Byminute = b.ByMinute
	}
	if len(b.ByWeekday) > 0 {
		opt.Byweekday = b.ByWeekday
	}
	if len(b.ByMonthDay) > 0 {
		opt.Bymonthday = b.ByMonthDay
	}
	if len(b.ByMonth) > 0 {
		opt.Bymonth = b.ByMonth
	}
	if b.Count > 0 {
		opt.Count = b.Count
	}
	if b.Until != nil {
		opt.Until = *b.Until
	}

	return rrule.NewRRule(opt)
}

func (b *RRuleBuilder) String() string {
	var parts []string

	// Frequency
	freqMap := map[rrule.Frequency]string{
		rrule.HOURLY:  "HOURLY",
		rrule.DAILY:   "DAILY",
		rrule.WEEKLY:  "WEEKLY",
		rrule.MONTHLY: "MONTHLY",
		rrule.YEARLY:  "YEARLY",
	}
	parts = append(parts, fmt.Sprintf("FREQ=%s", freqMap[b.Freq]))

	// Interval
	if b.Interval > 1 {
		parts = append(parts, fmt.Sprintf("INTERVAL=%d", b.Interval))
	}

	// ByHour
	if len(b.ByHour) > 0 {
		hours := make([]string, len(b.ByHour))
		for i, h := range b.ByHour {
			hours[i] = fmt.Sprintf("%d", h)
		}
		parts = append(parts, fmt.Sprintf("BYHOUR=%s", strings.Join(hours, ",")))
	}

	// ByMinute
	if len(b.ByMinute) > 0 {
		mins := make([]string, len(b.ByMinute))
		for i, m := range b.ByMinute {
			mins[i] = fmt.Sprintf("%d", m)
		}
		parts = append(parts, fmt.Sprintf("BYMINUTE=%s", strings.Join(mins, ",")))
	}

	// ByWeekday
	if len(b.ByWeekday) > 0 {
		days := make([]string, len(b.ByWeekday))
		dayMap := map[rrule.Weekday]string{
			rrule.MO: "MO",
			rrule.TU: "TU",
			rrule.WE: "WE",
			rrule.TH: "TH",
			rrule.FR: "FR",
			rrule.SA: "SA",
			rrule.SU: "SU",
		}
		for i, d := range b.ByWeekday {
			days[i] = dayMap[d]
		}
		parts = append(parts, fmt.Sprintf("BYDAY=%s", strings.Join(days, ",")))
	}

	// ByMonthDay
	if len(b.ByMonthDay) > 0 {
		days := make([]string, len(b.ByMonthDay))
		for i, d := range b.ByMonthDay {
			days[i] = fmt.Sprintf("%d", d)
		}
		parts = append(parts, fmt.Sprintf("BYMONTHDAY=%s", strings.Join(days, ",")))
	}

	// ByMonth
	if len(b.ByMonth) > 0 {
		months := make([]string, len(b.ByMonth))
		for i, m := range b.ByMonth {
			months[i] = fmt.Sprintf("%d", m)
		}
		parts = append(parts, fmt.Sprintf("BYMONTH=%s", strings.Join(months, ",")))
	}

	// Count
	if b.Count > 0 {
		parts = append(parts, fmt.Sprintf("COUNT=%d", b.Count))
	}

	// Until
	if b.Until != nil {
		parts = append(parts, fmt.Sprintf("UNTIL=%s", b.Until.UTC().Format("20060102T150405Z")))
	}

	return strings.Join(parts, ";")
}

// HumanReadable returns a human-readable description of the RRULE
func HumanReadable(ruleStr string, dtstart time.Time) string {
	rule, err := ParseRRule(ruleStr, dtstart)
	if err != nil {
		return ruleStr
	}
	return rule.String()
}

// HumanReadableChinese returns a Chinese description of the RRULE
func HumanReadableChinese(ruleStr string) string {
	ruleStr = strings.TrimPrefix(ruleStr, "RRULE:")

	// Parse key parts
	parts := strings.Split(ruleStr, ";")
	info := make(map[string]string)
	for _, p := range parts {
		kv := strings.SplitN(p, "=", 2)
		if len(kv) == 2 {
			info[kv[0]] = kv[1]
		}
	}

	var result strings.Builder

	// Frequency
	freq := info["FREQ"]
	interval := info["INTERVAL"]
	if interval == "" || interval == "1" {
		switch freq {
		case "HOURLY":
			result.WriteString("每小時")
		case "DAILY":
			result.WriteString("每天")
		case "WEEKLY":
			result.WriteString("每週")
		case "MONTHLY":
			result.WriteString("每月")
		case "YEARLY":
			result.WriteString("每年")
		}
	} else {
		switch freq {
		case "HOURLY":
			result.WriteString(fmt.Sprintf("每 %s 小時", interval))
		case "DAILY":
			result.WriteString(fmt.Sprintf("每 %s 天", interval))
		case "WEEKLY":
			result.WriteString(fmt.Sprintf("每 %s 週", interval))
		case "MONTHLY":
			result.WriteString(fmt.Sprintf("每 %s 月", interval))
		case "YEARLY":
			result.WriteString(fmt.Sprintf("每 %s 年", interval))
		}
	}

	// ByDay
	if byDay := info["BYDAY"]; byDay != "" {
		dayMap := map[string]string{
			"MO": "一", "TU": "二", "WE": "三", "TH": "四",
			"FR": "五", "SA": "六", "SU": "日",
		}
		days := strings.Split(byDay, ",")
		var chDays []string
		for _, d := range days {
			if ch, ok := dayMap[d]; ok {
				chDays = append(chDays, "週"+ch)
			}
		}
		if len(chDays) > 0 {
			result.WriteString(" " + strings.Join(chDays, "、"))
		}
	}

	// ByHour
	if byHour := info["BYHOUR"]; byHour != "" {
		hours := strings.Split(byHour, ",")
		if len(hours) > 3 {
			result.WriteString(fmt.Sprintf(" %s:00-%s:00", hours[0], hours[len(hours)-1]))
		} else {
			result.WriteString(fmt.Sprintf(" %s 點", strings.Join(hours, "、")))
		}
	}

	// Count
	if count := info["COUNT"]; count != "" {
		result.WriteString(fmt.Sprintf("，共 %s 次", count))
	}

	// Until
	if until := info["UNTIL"]; until != "" {
		if t, err := time.Parse("20060102T150405Z", until); err == nil {
			result.WriteString(fmt.Sprintf("，直到 %s", t.Local().Format("2006-01-02")))
		}
	}

	if result.Len() == 0 {
		return "一次性"
	}
	return result.String()
}

// IsRecurring checks if the RRULE string represents a recurring event
func IsRecurring(ruleStr string) bool {
	return ruleStr != "" && strings.Contains(strings.ToUpper(ruleStr), "FREQ=")
}
