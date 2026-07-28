package service

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Partition rule constants matching Java PartitionConstants.
const (
	// partitionMaxPt matches "maxpt" keyword in partition rules.
	partitionMaxPt = `(?i)maxpt`
	// odpsPartitionMaxPt is the ODPS function replacement for maxpt.
	odpsPartitionMaxPt = "MAX_PT"
	// partitionDateReg matches ${yyyymmdd}, ${yyyymmdd+N}, ${yyyymmdd-N} patterns.
	partitionDateReg = `\$\{yyyymmdd([+-]\d+)?\}`
)

// DataSourceType represents the type of data source.
type DataSourceType string

const (
	DataSourceTypeODPS    DataSourceType = "ODPS"
	DataSourceTypeOSS     DataSourceType = "OSS"
	DataSourceTypeHTTP    DataSourceType = "HTTP"
	DataSourceTypeLocalFS DataSourceType = "LOCAL_FS"
)

// PartitionRuleService handles partition rule analysis for scheduled data reads.
type PartitionRuleService struct{}

// NewPartitionRuleService creates a new PartitionRuleService.
func NewPartitionRuleService() *PartitionRuleService {
	return &PartitionRuleService{}
}

// ReadPartitionRuleAnalysis parses a partition rule and returns the resolved SQL condition.
// Only ODPS type is supported; other types return empty string.
func (s *PartitionRuleService) ReadPartitionRuleAnalysis(
	tableName string,
	dsType DataSourceType,
	inputRule string,
	scheduleExpectStartDate string,
	partitionColumns map[string]bool,
) (string, error) {
	if tableName == "" || inputRule == "" {
		return "", nil
	}

	if dsType != DataSourceTypeODPS {
		return "", nil
	}

	result, err := s.odpsReadPartitionRuleAnalysis(tableName, inputRule, scheduleExpectStartDate)
	if err != nil {
		return "", err
	}

	if err := s.isValidPartitionCondition(result, partitionColumns); err != nil {
		return "", err
	}

	return result, nil
}

// odpsReadPartitionRuleAnalysis processes ODPS-specific partition rules.
func (s *PartitionRuleService) odpsReadPartitionRuleAnalysis(tableName, inputRule, scheduleExpectStartDate string) (string, error) {
	// Reject rules with parentheses (unsupported)
	if strings.Contains(inputRule, "(") || strings.Contains(inputRule, ")") {
		return "", fmt.Errorf("inputRule format error: parentheses not supported")
	}

	// Step 1: Replace maxpt with ODPS MAX_PT function
	result := s.odpsMaxPtReplacement(tableName, inputRule)

	// Step 2: Replace date placeholders with actual dates
	result = s.odpsDateReplacement(result, scheduleExpectStartDate)

	return result, nil
}

// odpsMaxPtReplacement replaces "maxpt" with MAX_PT('tableName').
func (s *PartitionRuleService) odpsMaxPtReplacement(tableName, inputRule string) string {
	re := regexp.MustCompile(partitionMaxPt)
	replacement := fmt.Sprintf("%s('%s')", odpsPartitionMaxPt, tableName)
	return re.ReplaceAllString(inputRule, replacement)
}

// odpsDateReplacement replaces ${yyyymmdd}, ${yyyymmdd+N}, ${yyyymmdd-N} with actual dates.
func (s *PartitionRuleService) odpsDateReplacement(inputRule, scheduleExpectStartDate string) string {
	re := regexp.MustCompile(partitionDateReg)

	baseDate := time.Now()
	if scheduleExpectStartDate != "" {
		if parsed, err := time.Parse("20060102", scheduleExpectStartDate); err == nil {
			baseDate = parsed
		}
	}

	return re.ReplaceAllStringFunc(inputRule, func(match string) string {
		// Extract the offset group (e.g., "+1", "-2", or empty)
		submatches := re.FindStringSubmatch(match)
		offsetStr := ""
		if len(submatches) > 1 {
			offsetStr = submatches[1]
		}

		targetDate := baseDate
		if offsetStr != "" {
			offset, err := strconv.Atoi(offsetStr)
			if err == nil {
				targetDate = baseDate.AddDate(0, 0, offset)
			}
		}

		return "'" + targetDate.Format("20060102") + "'"
	})
}

// isValidPartitionCondition validates that all column references in the condition
// exist in the provided partition columns set.
func (s *PartitionRuleService) isValidPartitionCondition(partitionCondition string, partitionColumns map[string]bool) error {
	if len(partitionColumns) == 0 {
		return nil
	}

	// Split by AND/OR (case-insensitive)
	splitRe := regexp.MustCompile(`(?i)\s+(AND|OR)\s+`)
	conditions := splitRe.Split(partitionCondition, -1)

	for _, condition := range conditions {
		condition = strings.TrimSpace(condition)
		if condition == "" {
			continue
		}

		// Extract column name (left side of operator)
		colName := extractColumnName(condition)
		if colName == "" {
			continue
		}

		if !partitionColumns[colName] {
			return fmt.Errorf("partition column '%s' not found in table partition columns", colName)
		}
	}

	return nil
}

// extractColumnName extracts the column name from a condition expression.
// e.g., "dt='20240101'" -> "dt", "a in ('x','y')" -> "a"
func extractColumnName(condition string) string {
	// Find the first operator: =, <, >, in, between
	operators := []string{"=", "<", ">", " in ", " between ", " IN ", " BETWEEN "}
	minIdx := len(condition)
	for _, op := range operators {
		idx := strings.Index(condition, op)
		if idx >= 0 && idx < minIdx {
			minIdx = idx
		}
	}
	if minIdx == len(condition) {
		return ""
	}
	return strings.TrimSpace(condition[:minIdx])
}
