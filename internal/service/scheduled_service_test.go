package service

import (
	"testing"
)

func TestBuildCronExpression(t *testing.T) {
	tests := []struct {
		name     string
		input    CronCompat
		expected string
	}{
		{
			name:     "daily at 10:30",
			input:    CronCompat{ScheduleCycle: "DAY", ScheduleTime: "10:30"},
			expected: "0 30 10 * * *",
		},
		{
			name:     "hourly at minute 15",
			input:    CronCompat{ScheduleCycle: "HOUR", ScheduleTime: "00:15"},
			expected: "0 15 * * * *",
		},
		{
			name:     "weekly on Monday at 08:00",
			input:    CronCompat{ScheduleCycle: "WEEK", ScheduleTime: "08:00", ScheduleDate: "1"},
			expected: "0 0 8 * * 1",
		},
		{
			name:     "monthly on day 15 at 12:00",
			input:    CronCompat{ScheduleCycle: "MONTH", ScheduleTime: "12:00", ScheduleDate: "15"},
			expected: "0 0 12 15 * *",
		},
		{
			name:     "default cycle (DAY) with empty time",
			input:    CronCompat{ScheduleCycle: "", ScheduleTime: ""},
			expected: "0 0 0 * * *",
		},
		{
			name:     "case insensitive cycle",
			input:    CronCompat{ScheduleCycle: "day", ScheduleTime: "14:45"},
			expected: "0 45 14 * * *",
		},
		{
			name:     "time with spaces",
			input:    CronCompat{ScheduleCycle: "DAY", ScheduleTime: " 09 : 15 "},
			expected: "0 15 9 * * *",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildCronExpression(tt.input)
			if result != tt.expected {
				t.Errorf("buildCronExpression(%+v) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
