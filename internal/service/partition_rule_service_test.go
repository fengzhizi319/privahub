package service

import (
	"testing"
	"time"
)

func TestPartitionRule_EmptyInputs(t *testing.T) {
	svc := NewPartitionRuleService()

	// Empty table name
	result, err := svc.ReadPartitionRuleAnalysis("", DataSourceTypeODPS, "dt=maxpt", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "" {
		t.Errorf("expected empty result for empty table name, got %q", result)
	}

	// Empty rule
	result, err = svc.ReadPartitionRuleAnalysis("my_table", DataSourceTypeODPS, "", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "" {
		t.Errorf("expected empty result for empty rule, got %q", result)
	}
}

func TestPartitionRule_NonODPSReturnsEmpty(t *testing.T) {
	svc := NewPartitionRuleService()

	for _, dsType := range []DataSourceType{DataSourceTypeOSS, DataSourceTypeHTTP, DataSourceTypeLocalFS} {
		result, err := svc.ReadPartitionRuleAnalysis("table", dsType, "dt=maxpt", "", nil)
		if err != nil {
			t.Fatalf("unexpected error for %s: %v", dsType, err)
		}
		if result != "" {
			t.Errorf("expected empty result for %s, got %q", dsType, result)
		}
	}
}

func TestPartitionRule_MaxPtReplacement(t *testing.T) {
	svc := NewPartitionRuleService()

	result, err := svc.ReadPartitionRuleAnalysis("my_table", DataSourceTypeODPS, "dt=maxpt", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "dt=MAX_PT('my_table')"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestPartitionRule_MaxPtCaseInsensitive(t *testing.T) {
	svc := NewPartitionRuleService()

	result, err := svc.ReadPartitionRuleAnalysis("tbl", DataSourceTypeODPS, "dt=MAXPT", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "dt=MAX_PT('tbl')"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestPartitionRule_DateReplacement(t *testing.T) {
	svc := NewPartitionRuleService()

	result, err := svc.ReadPartitionRuleAnalysis("tbl", DataSourceTypeODPS, "dt=${yyyymmdd}", "20240315", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "dt='20240315'"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestPartitionRule_DateReplacementWithOffset(t *testing.T) {
	svc := NewPartitionRuleService()

	tests := []struct {
		rule     string
		baseDate string
		expected string
	}{
		{"dt=${yyyymmdd+1}", "20240315", "dt='20240316'"},
		{"dt=${yyyymmdd-1}", "20240315", "dt='20240314'"},
		{"dt=${yyyymmdd+30}", "20240101", "dt='20240131'"},
		{"dt=${yyyymmdd-0}", "20240315", "dt='20240315'"},
	}

	for _, tt := range tests {
		result, err := svc.ReadPartitionRuleAnalysis("tbl", DataSourceTypeODPS, tt.rule, tt.baseDate, nil)
		if err != nil {
			t.Fatalf("unexpected error for rule %q: %v", tt.rule, err)
		}
		if result != tt.expected {
			t.Errorf("rule %q: expected %q, got %q", tt.rule, tt.expected, result)
		}
	}
}

func TestPartitionRule_DateReplacementDefaultNow(t *testing.T) {
	svc := NewPartitionRuleService()

	// When scheduleExpectStartDate is empty, uses time.Now()
	result, err := svc.ReadPartitionRuleAnalysis("tbl", DataSourceTypeODPS, "dt=${yyyymmdd}", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	today := time.Now().Format("20060102")
	expected := "dt='" + today + "'"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestPartitionRule_ParenthesesRejected(t *testing.T) {
	svc := NewPartitionRuleService()

	_, err := svc.ReadPartitionRuleAnalysis("tbl", DataSourceTypeODPS, "dt=func(x)", "", nil)
	if err == nil {
		t.Error("expected error for parentheses in rule")
	}
}

func TestPartitionRule_InvalidPartitionColumn(t *testing.T) {
	svc := NewPartitionRuleService()

	partitionCols := map[string]bool{"dt": true, "region": true}

	// Valid column
	_, err := svc.ReadPartitionRuleAnalysis("tbl", DataSourceTypeODPS, "dt=${yyyymmdd}", "20240101", partitionCols)
	if err != nil {
		t.Fatalf("unexpected error for valid column: %v", err)
	}

	// Invalid column
	_, err = svc.ReadPartitionRuleAnalysis("tbl", DataSourceTypeODPS, "invalid_col=${yyyymmdd}", "20240101", partitionCols)
	if err == nil {
		t.Error("expected error for invalid partition column")
	}
}

func TestPartitionRule_CompoundCondition(t *testing.T) {
	svc := NewPartitionRuleService()

	partitionCols := map[string]bool{"dt": true, "region": true}

	result, err := svc.ReadPartitionRuleAnalysis("tbl", DataSourceTypeODPS,
		"dt=${yyyymmdd} AND region='us'", "20240315", partitionCols)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "dt='20240315' AND region='us'"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestPartitionRule_EmptyPartitionColumnsSkipsValidation(t *testing.T) {
	svc := NewPartitionRuleService()

	// nil partition columns should skip validation
	result, err := svc.ReadPartitionRuleAnalysis("tbl", DataSourceTypeODPS, "any_col=${yyyymmdd}", "20240101", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "any_col='20240101'" {
		t.Errorf("expected any_col='20240101', got %q", result)
	}
}

func TestExtractColumnName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"dt='20240101'", "dt"},
		{"a in ('x','y')", "a"},
		{"col > 5", "col"},
		{"col < 10", "col"},
		{"name between 'a' and 'z'", "name"},
		{"nooperator", ""},
		{"", ""},
	}

	for _, tt := range tests {
		got := extractColumnName(tt.input)
		if got != tt.expected {
			t.Errorf("extractColumnName(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
