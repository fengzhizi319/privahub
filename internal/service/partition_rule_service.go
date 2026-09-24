package service

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/fengzhizi319/privahub/pkg/kuscia"
)

// Partition rule constants matching Java PartitionConstants.
const (
	// partitionMaxPt is the (case-sensitive) rule keyword for the latest partition.
	partitionMaxPt = "maxpt"
	// odpsPartitionMaxPt is the ODPS function replacing maxpt.
	odpsPartitionMaxPt = "max_pt"
	// partitionDateReg matches ${yyyymmdd}, ${yyyymmdd+N}, ${yyyymmdd-N}.
	partitionDateReg = `\$\{yyyymmdd([+-]\d*)?\}`
)

var (
	partitionDateRe    = regexp.MustCompile(partitionDateReg)
	partitionSplitRe   = regexp.MustCompile(`(?i)\s+(AND|OR)\s+`)
	partitionValueRe   = regexp.MustCompile(`^[a-zA-Z0-9_()]+$`)
	partitionOperators = []string{">=", "<=", "!=", "=", ">", "<"}
)

// DataSourceType represents the type of data source.
type DataSourceType string

// Data source types understood by the partition rule analysis.
const (
	DataSourceTypeODPS    DataSourceType = "ODPS"
	DataSourceTypeOSS     DataSourceType = "OSS"
	DataSourceTypeHTTP    DataSourceType = "HTTP"
	DataSourceTypeLocalFS DataSourceType = "LOCAL_FS"
	DataSourceTypeMySQL   DataSourceType = "MYSQL"
)

// PartitionRuleService mirrors Java ReadPartitionRuleAnalysisServiceImpl: it
// renders the read-partition rule of a scheduled ODPS read into the SQL
// partition condition passed to SecretFlow (task_input_config
// sf_input_partitions_spec).
//
// API (for the graph/job converter):
//
//	cond, err := svc.ReadPartitionRuleAnalysis(tableName, DataSourceTypeODPS,
//	    rule, scheduleExpectStartDate /* "yyyyMMdd" or "" = today */,
//	    PartitionColumnsFromDomainData(&domainData))
//
// Rules: "maxpt" -> max_pt('<table>'); ${yyyymmdd[+-N]} -> 'yyyyMMdd' relative
// to scheduleExpectStartDate; parentheses are rejected; every AND/OR condition
// must be "<partition column><op><value>" with op in = != > < >= <= and value
// [a-zA-Z0-9_()]+ (quotes stripped) or containing max_pt. Non-ODPS types
// return "". An empty partitionColumns set skips the column validation (Java
// always passes the ODPS partition columns; callers that cannot resolve them
// yet get lenient behaviour).
type PartitionRuleService struct{}

// NewPartitionRuleService creates a new PartitionRuleService.
func NewPartitionRuleService() *PartitionRuleService {
	return &PartitionRuleService{}
}

// PartitionColumnsFromDomainData returns the partition column set of a Kuscia
// DomainData (partition.fields[].name).
func PartitionColumnsFromDomainData(d *kuscia.DomainData) map[string]bool {
	cols := map[string]bool{}
	if d == nil || d.Partition == nil {
		return cols
	}
	for _, f := range d.Partition.Fields {
		cols[f.Name] = true
	}
	return cols
}

// ReadPartitionRuleAnalysis parses a partition rule and returns the resolved condition.
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
	if !strings.EqualFold(string(dsType), string(DataSourceTypeODPS)) {
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
	if strings.ContainsAny(inputRule, "()") {
		return "", fmt.Errorf("inputRule format error")
	}
	result := s.odpsMaxPtReplacement(tableName, inputRule)
	return s.odpsDateReplacement(result, scheduleExpectStartDate)
}

// odpsMaxPtReplacement replaces "maxpt" with max_pt('tableName').
func (s *PartitionRuleService) odpsMaxPtReplacement(tableName, inputRule string) string {
	return strings.ReplaceAll(inputRule, partitionMaxPt, odpsPartitionMaxPt+"('"+tableName+"')")
}

// odpsDateReplacement replaces ${yyyymmdd}, ${yyyymmdd+N}, ${yyyymmdd-N} with
// quoted dates relative to scheduleExpectStartDate (yyyyMMdd; empty = today).
func (s *PartitionRuleService) odpsDateReplacement(inputRule, scheduleExpectStartDate string) (string, error) {
	base := time.Now()
	if scheduleExpectStartDate != "" {
		parsed, err := time.Parse("20060102", scheduleExpectStartDate)
		if err != nil {
			return "", fmt.Errorf("invalid scheduleExpectStartDate %q: %w", scheduleExpectStartDate, err)
		}
		base = parsed
	}
	var convErr error
	out := partitionDateRe.ReplaceAllStringFunc(inputRule, func(match string) string {
		sub := partitionDateRe.FindStringSubmatch(match)
		target := base
		if len(sub) > 1 && sub[1] != "" {
			days := 0
			if len(sub[1]) > 1 {
				n, err := strconv.Atoi(sub[1][1:])
				if err != nil {
					convErr = err
					return match
				}
				days = n
			}
			if sub[1][0] == '-' {
				days = -days
			}
			target = base.AddDate(0, 0, days)
		}
		return "'" + target.Format("20060102") + "'"
	})
	if convErr != nil {
		return "", convErr
	}
	return out, nil
}

// splitCondition splits "col<op>value" on its single comparison operator.
func splitCondition(cond string) (string, string, bool) {
	var parts []string
	last := 0
	for i := 0; i < len(cond); {
		matched := ""
		for _, op := range partitionOperators {
			if strings.HasPrefix(cond[i:], op) {
				matched = op
				break
			}
		}
		if matched == "" {
			i++
			continue
		}
		parts = append(parts, cond[last:i])
		i += len(matched)
		last = i
	}
	parts = append(parts, cond[last:])
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// isValidPartitionCondition mirrors Java isValidPartitionCondition.
func (s *PartitionRuleService) isValidPartitionCondition(partitionCondition string, partitionColumns map[string]bool) error {
	if len(partitionColumns) == 0 {
		return nil
	}
	for _, condition := range partitionSplitRe.Split(partitionCondition, -1) {
		col, val, ok := splitCondition(strings.TrimSpace(condition))
		if !ok || !partitionColumns[strings.TrimSpace(col)] {
			return fmt.Errorf("Invalid partition condition col must partition columns")
		}
		val = strings.TrimSpace(val)
		val = strings.TrimSuffix(strings.TrimPrefix(val, "'"), "'")
		if !strings.Contains(val, odpsPartitionMaxPt) && !partitionValueRe.MatchString(val) {
			return fmt.Errorf("Invalid partition condition value")
		}
	}
	return nil
}
