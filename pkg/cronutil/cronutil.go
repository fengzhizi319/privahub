// Package cronutil ports the Java SecretPad CronUtils used by scheduled
// (periodic) graph jobs.
//
// The frontend describes a schedule with a structured config:
//
//	scheduleCycle  "D" (daily) | "W" (weekly) | "M" (monthly)
//	scheduleDate   W: comma separated ISO weekdays, 1=Mon .. 7=Sun
//	               M: comma separated days of month 1..31, "end"/"L" = last day
//	               D: ignored
//	scheduleTime   "HH:mm" (an optional ":ss" suffix is tolerated)
//	startTime      "yyyy-MM-dd HH:mm:ss"
//	endTime        "yyyy-MM-dd HH:mm:ss"
//
// BuildCronExpressions turns that config into Quartz style 6-field cron
// expressions (the same textual form the Java backend persisted), and
// TriggerTimes enumerates every concrete trigger datetime of those
// expressions within [startTime, endTime].
//
// Only the subset of Quartz syntax produced by BuildCronExpressions is
// supported by Parse: fixed second/minute/hour, day-of-month lists with "L",
// "?"/"*" wildcards, and day-of-week lists by name (MON..SUN) or Quartz number
// (1=SUN..7=SAT). Month must be "*".
package cronutil

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Schedule cycles (mirror Java ScheduledConstants).
const (
	CycleDay   = "D"
	CycleWeek  = "W"
	CycleMonth = "M"
)

// DateTimeLayout is the Java "yyyy-MM-dd HH:mm:ss" layout.
const DateTimeLayout = "2006-01-02 15:04:05"

// MaxTriggerTimes bounds the number of trigger datetimes a single schedule
// may expand to (daily for ~27 years); protects against runaway ranges.
const MaxTriggerTimes = 10000

// maxScanDays bounds the day-by-day search in Expr.Next (8 years covers any
// valid expression, e.g. "31st of month" or leap-day style gaps).
const maxScanDays = 366 * 8

// ErrInvalidConfig is returned (wrapped) for any invalid schedule config.
var ErrInvalidConfig = errors.New("invalid schedule config")

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidConfig, fmt.Sprintf(format, args...))
}

// ParseDateTime parses a "yyyy-MM-dd HH:mm:ss" string in loc (time.Local when nil).
func ParseDateTime(s string, loc *time.Location) (time.Time, error) {
	if loc == nil {
		loc = time.Local
	}
	t, err := time.ParseInLocation(DateTimeLayout, strings.TrimSpace(s), loc)
	if err != nil {
		return time.Time{}, invalid("time %q must be yyyy-MM-dd HH:mm:ss", s)
	}
	return t, nil
}

// ParseScheduleTime parses "HH:mm" (or "HH:mm:ss") into hour, minute, second.
func ParseScheduleTime(s string) (hour, minute, second int, err error) {
	parts := strings.Split(strings.TrimSpace(s), ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, 0, 0, invalid("scheduleTime %q must be HH:mm", s)
	}
	vals := make([]int, 3)
	limits := []int{23, 59, 59}
	for i, p := range parts {
		v, convErr := strconv.Atoi(strings.TrimSpace(p))
		if convErr != nil || v < 0 || v > limits[i] {
			return 0, 0, 0, invalid("scheduleTime %q must be HH:mm", s)
		}
		vals[i] = v
	}
	return vals[0], vals[1], vals[2], nil
}

var quartzWeekdayNames = []string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"}

// BuildCronExpressions mirrors Java CronUtils.buildCronExpression.
//
// Differences from Java (intentional):
//   - Daily cycles produce one "every day" expression instead of one
//     expression per calendar day in the range (identical trigger set, but
//     Java's per-day loop could drop the last day).
//   - Weekday / month-day lists are validated and de-duplicated.
func BuildCronExpressions(cycle, scheduleDate, scheduleTime, startTime, endTime string, loc *time.Location) ([]string, error) {
	start, err := ParseDateTime(startTime, loc)
	if err != nil {
		return nil, err
	}
	end, err := ParseDateTime(endTime, loc)
	if err != nil {
		return nil, err
	}
	if end.Before(start) {
		return nil, invalid("end time must be greater than start time")
	}
	hour, minute, second, err := ParseScheduleTime(scheduleTime)
	if err != nil {
		return nil, err
	}
	// Java always uses second 0; keep the parsed seconds (0 for HH:mm).
	prefix := fmt.Sprintf("%d %d %d", second, minute, hour)

	var exprs []string
	seen := map[string]bool{}
	add := func(e string) {
		if !seen[e] {
			seen[e] = true
			exprs = append(exprs, e)
		}
	}

	switch strings.ToUpper(strings.TrimSpace(cycle)) {
	case CycleDay:
		add(prefix + " * * ?")
	case CycleWeek:
		items := splitList(scheduleDate)
		if len(items) == 0 {
			return nil, invalid("scheduleDate is required for weekly cycle")
		}
		for _, it := range items {
			d, convErr := strconv.Atoi(it)
			if convErr != nil || d < 1 || d > 7 {
				return nil, invalid("weekly scheduleDate %q must be 1(Mon)..7(Sun)", it)
			}
			// ISO 1=Mon..7=Sun -> Go Weekday (Sunday=0).
			add(fmt.Sprintf("%s ? * %s", prefix, quartzWeekdayNames[d%7]))
		}
	case CycleMonth:
		items := splitList(scheduleDate)
		if len(items) == 0 {
			return nil, invalid("scheduleDate is required for monthly cycle")
		}
		for _, it := range items {
			if strings.EqualFold(it, "end") || strings.EqualFold(it, "L") {
				add(prefix + " L * ?")
				continue
			}
			d, convErr := strconv.Atoi(it)
			if convErr != nil || d < 1 || d > 31 {
				return nil, invalid("monthly scheduleDate %q must be 1..31 or end", it)
			}
			add(fmt.Sprintf("%s %d * ?", prefix, d))
		}
	default:
		return nil, invalid("invalid schedule cycle: %q", cycle)
	}
	return exprs, nil
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Expr is a parsed Quartz expression (supported subset, see package doc).
type Expr struct {
	Second, Minute, Hour int
	anyDOM               bool // "*" in day-of-month
	noDOM                bool // "?" in day-of-month
	doms                 map[int]bool
	lastDOM              bool
	anyDOW               bool
	noDOW                bool
	dows                 map[time.Weekday]bool
}

// Parse parses the Quartz subset produced by BuildCronExpressions.
func Parse(expr string) (*Expr, error) {
	f := strings.Fields(expr)
	if len(f) != 6 && len(f) != 7 {
		return nil, invalid("cron %q must have 6 or 7 fields", expr)
	}
	e := &Expr{doms: map[int]bool{}, dows: map[time.Weekday]bool{}}
	var err error
	if e.Second, err = fixedField(f[0], 59); err != nil {
		return nil, invalid("cron %q second: %v", expr, err)
	}
	if e.Minute, err = fixedField(f[1], 59); err != nil {
		return nil, invalid("cron %q minute: %v", expr, err)
	}
	if e.Hour, err = fixedField(f[2], 23); err != nil {
		return nil, invalid("cron %q hour: %v", expr, err)
	}
	if f[4] != "*" {
		return nil, invalid("cron %q month must be *", expr)
	}
	if len(f) == 7 && f[6] != "*" {
		return nil, invalid("cron %q year must be *", expr)
	}
	switch f[3] {
	case "*":
		e.anyDOM = true
	case "?":
		e.noDOM = true
	default:
		for _, it := range splitList(f[3]) {
			if strings.EqualFold(it, "L") {
				e.lastDOM = true
				continue
			}
			d, convErr := strconv.Atoi(it)
			if convErr != nil || d < 1 || d > 31 {
				return nil, invalid("cron %q day-of-month %q", expr, it)
			}
			e.doms[d] = true
		}
	}
	switch f[5] {
	case "*":
		e.anyDOW = true
	case "?":
		e.noDOW = true
	default:
		for _, it := range splitList(f[5]) {
			wd, ok := parseQuartzWeekday(it)
			if !ok {
				return nil, invalid("cron %q day-of-week %q", expr, it)
			}
			e.dows[wd] = true
		}
	}
	if e.noDOM && e.noDOW {
		return nil, invalid("cron %q: day-of-month and day-of-week cannot both be ?", expr)
	}
	if !e.noDOM && !e.noDOW && !(e.anyDOM || e.anyDOW) {
		return nil, invalid("cron %q: one of day-of-month / day-of-week must be ?", expr)
	}
	return e, nil
}

func fixedField(s string, max int) (int, error) {
	v, err := strconv.Atoi(s)
	if err != nil || v < 0 || v > max {
		return 0, fmt.Errorf("%q must be 0..%d", s, max)
	}
	return v, nil
}

func parseQuartzWeekday(s string) (time.Weekday, bool) {
	ls := strings.ToLower(s)
	for i, n := range quartzWeekdayNames {
		if ls == n {
			return time.Weekday(i), true
		}
	}
	// Quartz numeric: 1=SUN .. 7=SAT.
	if v, err := strconv.Atoi(s); err == nil && v >= 1 && v <= 7 {
		return time.Weekday(v - 1), true
	}
	return 0, false
}

func (e *Expr) matchDay(day time.Time) bool {
	domOK := func() bool {
		if e.anyDOM {
			return true
		}
		if e.doms[day.Day()] {
			return true
		}
		return e.lastDOM && day.AddDate(0, 0, 1).Day() == 1
	}
	dowOK := func() bool {
		return e.anyDOW || e.dows[day.Weekday()]
	}
	switch {
	case e.noDOW:
		return domOK()
	case e.noDOM:
		return dowOK()
	default: // at least one side is "*"
		return domOK() && dowOK()
	}
}

// Next returns the first trigger time strictly after `after` (in after's
// location), or false when none exists within the scan horizon.
func (e *Expr) Next(after time.Time) (time.Time, bool) {
	loc := after.Location()
	y, m, d := after.Date()
	for i := 0; i < maxScanDays; i++ {
		day := time.Date(y, m, d+i, 0, 0, 0, 0, loc)
		if !e.matchDay(day) {
			continue
		}
		cand := time.Date(day.Year(), day.Month(), day.Day(), e.Hour, e.Minute, e.Second, 0, loc)
		if cand.After(after) {
			return cand, true
		}
	}
	return time.Time{}, false
}

// MatchingTimes mirrors Java CronUtils.getMatchingDates: all trigger times of
// expr within [start, end] that are not before now.
//
// Differences from Java (intentional): start is inclusive, and past trigger
// times are skipped instead of aborting the enumeration (Java returned an
// empty list whenever the first match was already in the past).
func MatchingTimes(expr string, start, end, now time.Time) ([]time.Time, error) {
	e, err := Parse(expr)
	if err != nil {
		return nil, err
	}
	var out []time.Time
	cur := start.Add(-time.Nanosecond)
	for {
		next, ok := e.Next(cur)
		if !ok || next.After(end) {
			break
		}
		cur = next
		if next.Before(now) {
			continue
		}
		out = append(out, next)
		if len(out) > MaxTriggerTimes {
			return nil, invalid("schedule expands to more than %d trigger times", MaxTriggerTimes)
		}
	}
	return out, nil
}

// Trigger is one concrete trigger datetime and the expression producing it.
type Trigger struct {
	Time time.Time
	Cron string
}

// TriggerTimes enumerates the de-duplicated, ascending trigger times of all
// expressions within [start, end], skipping those before now. When two
// expressions produce the same datetime the first expression wins (Java
// behaviour: duplicate dates are skipped).
func TriggerTimes(exprs []string, start, end, now time.Time) ([]Trigger, error) {
	seen := map[int64]bool{}
	var out []Trigger
	for _, expr := range exprs {
		times, err := MatchingTimes(expr, start, end, now)
		if err != nil {
			return nil, err
		}
		for _, t := range times {
			k := t.Unix()
			if seen[k] {
				continue
			}
			seen[k] = true
			out = append(out, Trigger{Time: t, Cron: expr})
		}
		if len(out) > MaxTriggerTimes {
			return nil, invalid("schedule expands to more than %d trigger times", MaxTriggerTimes)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Time.Before(out[j].Time) })
	return out, nil
}

// Config is the structured cron config sent by the frontend.
type Config struct {
	StartTime     string `json:"startTime"`
	EndTime       string `json:"endTime"`
	ScheduleCycle string `json:"scheduleCycle"`
	ScheduleDate  string `json:"scheduleDate"`
	ScheduleTime  string `json:"scheduleTime"`
}

// Expand validates cfg and returns its cron expressions plus all trigger
// times in [startTime, endTime] not before now.
func Expand(cfg Config, now time.Time, loc *time.Location) ([]string, []Trigger, error) {
	if loc == nil {
		loc = time.Local
	}
	exprs, err := BuildCronExpressions(cfg.ScheduleCycle, cfg.ScheduleDate, cfg.ScheduleTime, cfg.StartTime, cfg.EndTime, loc)
	if err != nil {
		return nil, nil, err
	}
	start, _ := ParseDateTime(cfg.StartTime, loc)
	end, _ := ParseDateTime(cfg.EndTime, loc)
	triggers, err := TriggerTimes(exprs, start, end, now.In(loc))
	if err != nil {
		return nil, nil, err
	}
	return exprs, triggers, nil
}
