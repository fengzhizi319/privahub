package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fengzhizi319/privahub/pkg/kuscia"
)

func TestPartitionRule_EmptyAndNonODPS(t *testing.T) {
	s := NewPartitionRuleService()
	r, err := s.ReadPartitionRuleAnalysis("", DataSourceTypeODPS, "dt=maxpt", "", nil)
	require.NoError(t, err)
	assert.Empty(t, r)
	r, err = s.ReadPartitionRuleAnalysis("t", DataSourceTypeODPS, "", "", nil)
	require.NoError(t, err)
	assert.Empty(t, r)
	for _, ty := range []DataSourceType{DataSourceTypeOSS, DataSourceTypeHTTP, DataSourceTypeLocalFS, DataSourceTypeMySQL} {
		r, err = s.ReadPartitionRuleAnalysis("t", ty, "dt=maxpt", "", nil)
		require.NoError(t, err)
		assert.Empty(t, r)
	}
}

func TestPartitionRule_MaxPtAndDates(t *testing.T) {
	s := NewPartitionRuleService()
	cols := map[string]bool{"dt": true, "region": true}
	r, err := s.ReadPartitionRuleAnalysis("tbl", "odps", "dt=maxpt", "20240315", cols)
	require.NoError(t, err)
	assert.Equal(t, "dt=max_pt('tbl')", r)
	// keyword is case sensitive (Java String.replaceAll("maxpt"))
	r, err = s.ReadPartitionRuleAnalysis("tbl", DataSourceTypeODPS, "dt=MAXPT", "20240315", nil)
	require.NoError(t, err)
	assert.Equal(t, "dt=MAXPT", r)

	r, err = s.ReadPartitionRuleAnalysis("tbl", DataSourceTypeODPS, "dt=${yyyymmdd-1} AND region='us'", "20240301", cols)
	require.NoError(t, err)
	assert.Equal(t, "dt='20240229' AND region='us'", r)
	r, err = s.ReadPartitionRuleAnalysis("tbl", DataSourceTypeODPS, "dt>=${yyyymmdd+2} or dt<${yyyymmdd}", "20241231", cols)
	require.NoError(t, err)
	assert.Equal(t, "dt>='20250102' or dt<'20241231'", r)
	r, err = s.ReadPartitionRuleAnalysis("tbl", DataSourceTypeODPS, "dt=${yyyymmdd}", "", cols)
	require.NoError(t, err)
	assert.Equal(t, "dt='"+time.Now().Format("20060102")+"'", r)
}

func TestPartitionRule_Validation(t *testing.T) {
	s := NewPartitionRuleService()
	cols := map[string]bool{"dt": true}
	_, err := s.ReadPartitionRuleAnalysis("tbl", DataSourceTypeODPS, "dt in (${yyyymmdd})", "20240101", cols)
	assert.Error(t, err, "parentheses rejected")
	_, err = s.ReadPartitionRuleAnalysis("tbl", DataSourceTypeODPS, "other=${yyyymmdd}", "20240101", cols)
	assert.Error(t, err, "unknown column")
	_, err = s.ReadPartitionRuleAnalysis("tbl", DataSourceTypeODPS, "dt='a b'", "20240101", cols)
	assert.Error(t, err, "invalid value")
	_, err = s.ReadPartitionRuleAnalysis("tbl", DataSourceTypeODPS, "dt=1=2", "20240101", cols)
	assert.Error(t, err, "two operators")
	_, err = s.ReadPartitionRuleAnalysis("tbl", DataSourceTypeODPS, "dt=${yyyymmdd}", "2024-01-01", cols)
	assert.Error(t, err, "bad schedule date")
	// empty column set skips validation
	r, err := s.ReadPartitionRuleAnalysis("tbl", DataSourceTypeODPS, "any='x y'", "20240101", nil)
	require.NoError(t, err)
	assert.Equal(t, "any='x y'", r)
}

func TestPartitionColumnsFromDomainData(t *testing.T) {
	assert.Empty(t, PartitionColumnsFromDomainData(nil))
	cols := PartitionColumnsFromDomainData(&kuscia.DomainData{Partition: &kuscia.Partition{Type: "odps", Fields: []kuscia.DataColumn{{Name: "dt"}}}})
	assert.True(t, cols["dt"])
}
