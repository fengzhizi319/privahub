package cronutil

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

var shanghai = time.FixedZone("CST", 8*3600)

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	tm, err := ParseDateTime(s, shanghai)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return tm
}

func fmtTimes(ts []Trigger) []string {
	out := make([]string, 0, len(ts))
	for _, t := range ts {
		out = append(out, t.Time.Format(DateTimeLayout))
	}
	return out
}

func TestBuildCronExpressions(t *testing.T) {
	const s, e = "2024-01-01 00:00:00", "2024-12-31 23:59:59"
	tests := []struct {
		name, cycle, date, tm string
		want                  []string
		wantErr               bool
	}{
		{"daily", "D", "", "10:30", []string{"0 30 10 * * ?"}, false},
		{"daily lower-case cycle", "d", "ignored", "02:05", []string{"0 5 2 * * ?"}, false},
		{"daily with seconds", "D", "", "02:05:07", []string{"7 5 2 * * ?"}, false},
		{"weekly mon", "W", "1", "08:00", []string{"0 0 8 ? * mon"}, false},
		{"weekly sun is 7", "W", "7", "08:00", []string{"0 0 8 ? * sun"}, false},
		{"weekly list dedup", "W", "1, 3,5,3", "23:59", []string{"0 59 23 ? * mon", "0 59 23 ? * wed", "0 59 23 ? * fri"}, false},
		{"weekly 0 invalid", "W", "0", "08:00", nil, true},
		{"weekly 8 invalid", "W", "8", "08:00", nil, true},
		{"weekly empty invalid", "W", "", "08:00", nil, true},
		{"monthly days", "M", "1,15", "12:00", []string{"0 0 12 1 * ?", "0 0 12 15 * ?"}, false},
		{"monthly end", "M", "end", "12:00", []string{"0 0 12 L * ?"}, false},
		{"monthly L alias", "M", "L,end", "12:00", []string{"0 0 12 L * ?"}, false},
		{"monthly 32 invalid", "M", "32", "12:00", nil, true},
		{"monthly abc invalid", "M", "abc", "12:00", nil, true},
		{"bad cycle", "HOUR", "", "12:00", nil, true},
		{"bad time", "D", "", "25:00", nil, true},
		{"bad time minute", "D", "", "10:60", nil, true},
		{"bad time format", "D", "", "1030", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BuildCronExpressions(tt.cycle, tt.date, tt.tm, s, e, shanghai)
			if tt.wantErr {
				if err == nil || !errors.Is(err, ErrInvalidConfig) {
					t.Fatalf("want ErrInvalidConfig, got %v (%v)", err, got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v want %v", got, tt.want)
			}
		})
	}
}

func TestBuildCronExpressions_RangeValidation(t *testing.T) {
	if _, err := BuildCronExpressions("D", "", "10:00", "2024-01-02 00:00:00", "2024-01-01 00:00:00", shanghai); err == nil {
		t.Fatal("expected end-before-start error")
	}
	if _, err := BuildCronExpressions("D", "", "10:00", "2024/01/01", "2024-01-01 00:00:00", shanghai); err == nil {
		t.Fatal("expected start format error")
	}
	if _, err := BuildCronExpressions("D", "", "10:00", "2024-01-01 00:00:00", "tomorrow", shanghai); err == nil {
		t.Fatal("expected end format error")
	}
	// Equal start/end is allowed.
	if _, err := BuildCronExpressions("D", "", "10:00", "2024-01-01 00:00:00", "2024-01-01 00:00:00", shanghai); err != nil {
		t.Fatal(err)
	}
}

func TestParse(t *testing.T) {
	valid := []string{"0 0 2 * * ?", "0 0 8 ? * mon", "0 0 8 ? * MON,FRI", "0 0 8 ? * 2", "0 0 12 L * ?", "0 0 12 1,15,L * ?", "0 0 2 * * ? *", "0 0 2 * * *"}
	for _, v := range valid {
		if _, err := Parse(v); err != nil {
			t.Errorf("Parse(%q) error: %v", v, err)
		}
	}
	invalidExprs := []string{"", "0 0 2 * *", "0 0 2 * 1 ?", "* 0 2 * * ?", "0 0 24 * * ?", "0 60 2 * * ?", "0 0 2 ? * ?", "0 0 2 1 * mon", "0 0 2 32 * ?", "0 0 2 ? * funday", "0 0 2 * * ? 2025"}
	for _, v := range invalidExprs {
		if _, err := Parse(v); err == nil {
			t.Errorf("Parse(%q) expected error", v)
		}
	}
}

func TestParse_QuartzNumericWeekday(t *testing.T) {
	e, err := Parse("0 0 8 ? * 1") // Quartz 1 = Sunday
	if err != nil {
		t.Fatal(err)
	}
	next, ok := e.Next(mustTime(t, "2024-01-01 00:00:00")) // Monday
	if !ok || next.Format(DateTimeLayout) != "2024-01-07 08:00:00" {
		t.Fatalf("got %v", next)
	}
}

func TestExprNext(t *testing.T) {
	tests := []struct {
		expr, after, want string
	}{
		{"0 30 10 * * ?", "2024-01-01 00:00:00", "2024-01-01 10:30:00"},
		{"0 30 10 * * ?", "2024-01-01 10:30:00", "2024-01-02 10:30:00"}, // strictly after
		{"0 30 10 * * ?", "2024-12-31 11:00:00", "2025-01-01 10:30:00"},
		{"0 0 8 ? * mon", "2024-01-03 00:00:00", "2024-01-08 08:00:00"},
		{"0 0 8 ? * sun", "2024-01-06 09:00:00", "2024-01-07 08:00:00"},
		{"0 0 12 L * ?", "2024-02-01 00:00:00", "2024-02-29 12:00:00"}, // leap year
		{"0 0 12 L * ?", "2023-02-01 00:00:00", "2023-02-28 12:00:00"},
		{"0 0 12 L * ?", "2024-04-30 12:00:00", "2024-05-31 12:00:00"},
		{"0 0 12 31 * ?", "2024-02-01 00:00:00", "2024-03-31 12:00:00"}, // skips short months
		{"0 0 12 30 * ?", "2024-01-31 00:00:00", "2024-03-30 12:00:00"}, // Feb has no 30th
		{"0 0 0 29 * ?", "2023-02-01 00:00:00", "2023-03-29 00:00:00"},
	}
	for _, tt := range tests {
		e, err := Parse(tt.expr)
		if err != nil {
			t.Fatal(err)
		}
		got, ok := e.Next(mustTime(t, tt.after))
		if !ok || got.Format(DateTimeLayout) != tt.want {
			t.Errorf("Next(%q, %s) = %v,%v want %s", tt.expr, tt.after, got, ok, tt.want)
		}
	}
}

func TestExpand(t *testing.T) {
	now := mustTime(t, "2024-01-01 00:00:00")
	tests := []struct {
		name string
		cfg  Config
		want []string
	}{
		{
			name: "daily 3 days inclusive start and end",
			cfg:  Config{StartTime: "2024-01-01 10:00:00", EndTime: "2024-01-03 10:00:00", ScheduleCycle: "D", ScheduleTime: "10:00"},
			want: []string{"2024-01-01 10:00:00", "2024-01-02 10:00:00", "2024-01-03 10:00:00"},
		},
		{
			name: "daily end before time on last day",
			cfg:  Config{StartTime: "2024-01-01 00:00:00", EndTime: "2024-01-03 09:00:00", ScheduleCycle: "D", ScheduleTime: "10:00"},
			want: []string{"2024-01-01 10:00:00", "2024-01-02 10:00:00"},
		},
		{
			name: "daily start after time on first day",
			cfg:  Config{StartTime: "2024-01-01 11:00:00", EndTime: "2024-01-03 23:00:00", ScheduleCycle: "D", ScheduleTime: "10:00"},
			want: []string{"2024-01-02 10:00:00", "2024-01-03 10:00:00"},
		},
		{
			name: "daily across month boundary",
			cfg:  Config{StartTime: "2024-01-30 00:00:00", EndTime: "2024-02-02 00:00:00", ScheduleCycle: "D", ScheduleTime: "08:15"},
			want: []string{"2024-01-30 08:15:00", "2024-01-31 08:15:00", "2024-02-01 08:15:00"},
		},
		{
			name: "weekly mon and wed over two weeks, merged ascending",
			cfg:  Config{StartTime: "2024-01-01 00:00:00", EndTime: "2024-01-14 23:59:59", ScheduleCycle: "W", ScheduleDate: "3,1", ScheduleTime: "09:00"},
			want: []string{"2024-01-01 09:00:00", "2024-01-03 09:00:00", "2024-01-08 09:00:00", "2024-01-10 09:00:00"},
		},
		{
			name: "weekly sunday",
			cfg:  Config{StartTime: "2024-01-01 00:00:00", EndTime: "2024-01-31 00:00:00", ScheduleCycle: "W", ScheduleDate: "7", ScheduleTime: "20:00"},
			want: []string{"2024-01-07 20:00:00", "2024-01-14 20:00:00", "2024-01-21 20:00:00", "2024-01-28 20:00:00"},
		},
		{
			name: "monthly 1st and end",
			cfg:  Config{StartTime: "2024-01-01 00:00:00", EndTime: "2024-03-31 23:00:00", ScheduleCycle: "M", ScheduleDate: "1,end", ScheduleTime: "12:00"},
			want: []string{"2024-01-01 12:00:00", "2024-01-31 12:00:00", "2024-02-01 12:00:00", "2024-02-29 12:00:00", "2024-03-01 12:00:00", "2024-03-31 12:00:00"},
		},
		{
			name: "monthly 31 and end dedup",
			cfg:  Config{StartTime: "2024-01-01 00:00:00", EndTime: "2024-04-30 23:00:00", ScheduleCycle: "M", ScheduleDate: "31,end", ScheduleTime: "06:00"},
			want: []string{"2024-01-31 06:00:00", "2024-02-29 06:00:00", "2024-03-31 06:00:00", "2024-04-30 06:00:00"},
		},
		{
			name: "range with no matches",
			cfg:  Config{StartTime: "2024-01-02 00:00:00", EndTime: "2024-01-05 00:00:00", ScheduleCycle: "W", ScheduleDate: "7", ScheduleTime: "08:00"},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, got, err := Expand(tt.cfg, now, shanghai)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(fmtTimes(got), tt.want) && !(len(got) == 0 && len(tt.want) == 0) {
				t.Errorf("got %v want %v", fmtTimes(got), tt.want)
			}
		})
	}
}

func TestExpand_SkipsPastTriggers(t *testing.T) {
	now := mustTime(t, "2024-01-02 12:00:00")
	cfg := Config{StartTime: "2024-01-01 00:00:00", EndTime: "2024-01-04 23:00:00", ScheduleCycle: "D", ScheduleTime: "10:00"}
	_, got, err := Expand(cfg, now, shanghai)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"2024-01-03 10:00:00", "2024-01-04 10:00:00"}
	if !reflect.DeepEqual(fmtTimes(got), want) {
		t.Errorf("got %v want %v", fmtTimes(got), want)
	}
}

func TestExpand_TriggerCronRecorded(t *testing.T) {
	now := mustTime(t, "2024-01-01 00:00:00")
	cfg := Config{StartTime: "2024-01-01 00:00:00", EndTime: "2024-01-07 23:00:00", ScheduleCycle: "W", ScheduleDate: "1,2", ScheduleTime: "10:00"}
	exprs, got, err := Expand(cfg, now, shanghai)
	if err != nil {
		t.Fatal(err)
	}
	if len(exprs) != 2 || len(got) != 2 {
		t.Fatalf("exprs=%v got=%v", exprs, got)
	}
	if got[0].Cron != "0 0 10 ? * mon" || got[1].Cron != "0 0 10 ? * tue" {
		t.Errorf("unexpected crons: %+v", got)
	}
}

func TestExpand_TooMany(t *testing.T) {
	now := mustTime(t, "2000-01-01 00:00:00")
	cfg := Config{StartTime: "2000-01-01 00:00:00", EndTime: "2099-01-01 00:00:00", ScheduleCycle: "D", ScheduleTime: "10:00"}
	if _, _, err := Expand(cfg, now, shanghai); err == nil {
		t.Fatal("expected too many trigger times error")
	}
}

func TestExpand_InvalidConfig(t *testing.T) {
	now := mustTime(t, "2024-01-01 00:00:00")
	bad := []Config{
		{StartTime: "", EndTime: "2024-01-02 00:00:00", ScheduleCycle: "D", ScheduleTime: "10:00"},
		{StartTime: "2024-01-01 00:00:00", EndTime: "2024-01-02 00:00:00", ScheduleCycle: "", ScheduleTime: "10:00"},
		{StartTime: "2024-01-01 00:00:00", EndTime: "2024-01-02 00:00:00", ScheduleCycle: "D", ScheduleTime: ""},
		{StartTime: "2024-01-01 00:00:00", EndTime: "2024-01-02 00:00:00", ScheduleCycle: "W", ScheduleTime: "10:00"},
		{StartTime: "2024-01-01 00:00:00", EndTime: "2024-01-02 00:00:00", ScheduleCycle: "D", ScheduleTime: "0 0 2 * * ?"},
	}
	for i, c := range bad {
		if _, _, err := Expand(c, now, shanghai); !errors.Is(err, ErrInvalidConfig) {
			t.Errorf("case %d: expected ErrInvalidConfig, got %v", i, err)
		}
	}
}
