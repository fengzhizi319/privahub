// Package graphjob holds the pure (DB/Kuscia independent) building blocks of
// the graph execution chain: status enums, node definition helpers, DAG
// helpers and the SecretFlow task_input_config converter. It mirrors Java
// SecretPad's service/graph (ComponentTools, GraphBuilder, GraphUtils,
// JobUtils) and KusciaJobConverter.
package graphjob

import "strings"

// Job statuses (Java GraphJobStatus enum names).
const (
	JobRunning = "RUNNING"
	JobStopped = "STOPPED"
	JobSucceed = "SUCCEED"
	JobFailed  = "FAILED"
)

// Task statuses (Java GraphNodeTaskStatus enum names).
const (
	TaskStaging     = "STAGING"
	TaskInitialized = "INITIALIZED"
	TaskRunning     = "RUNNING"
	TaskStopped     = "STOPPED"
	TaskSucceed     = "SUCCEED"
	TaskFailed      = "FAILED"
)

// JobStatusFromKuscia mirrors GraphJobStatus.formKusciaJobStatus.
func JobStatusFromKuscia(state string) string {
	switch state {
	case "Succeeded":
		return JobSucceed
	case "Failed":
		return JobFailed
	case "Stopped", "Cancelled":
		return JobStopped
	default:
		return JobRunning
	}
}

// TaskStatusFromKuscia mirrors GraphNodeTaskStatus.formKusciaTaskStatus.
func TaskStatusFromKuscia(state string) string {
	switch state {
	case "Succeeded":
		return TaskSucceed
	case "Failed":
		return TaskFailed
	case "Running":
		return TaskRunning
	case "Stopped", "Cancelled":
		return TaskStopped
	default:
		return TaskInitialized
	}
}

// IsKusciaFinishedState reports a terminal Kuscia job state (Java isFinishedState).
func IsKusciaFinishedState(state string) bool {
	return state == "Failed" || state == "Succeeded"
}

// IsJobFinished mirrors ProjectJobDO.isFinished.
func IsJobFinished(status string) bool {
	switch NormalizeJobStatus(status) {
	case JobSucceed, JobFailed, JobStopped:
		return true
	}
	return false
}

// NormalizeJobStatus maps legacy Go job statuses to Java enum names.
func NormalizeJobStatus(s string) string {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "SUCCEEDED", "SUCCEED", "SUCCESS":
		return JobSucceed
	case "FAILED":
		return JobFailed
	case "STOPPED", "CANCELLED":
		return JobStopped
	case "":
		return ""
	default: // PENDING / RUNNING / INITIALIZED ...
		return JobRunning
	}
}

// NormalizeTaskStatus maps legacy Go task statuses to Java enum names.
func NormalizeTaskStatus(s string) string {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "SUCCEEDED", "SUCCEED", "SUCCESS":
		return TaskSucceed
	case "FAILED":
		return TaskFailed
	case "STOPPED", "CANCELLED":
		return TaskStopped
	case "RUNNING":
		return TaskRunning
	case "PENDING", "INITIALIZED":
		return TaskInitialized
	case "IDLE", "STAGING":
		return TaskStaging
	case "":
		return ""
	default:
		return TaskInitialized
	}
}

// LegacyJobStatusMapping lists (old -> new) values for the job status DB migration.
var LegacyJobStatusMapping = map[string]string{
	"PENDING":   JobRunning,
	"SUCCEEDED": JobSucceed,
	"SUCCESS":   JobSucceed,
	"Running":   JobRunning,
	"Succeeded": JobSucceed,
	"Failed":    JobFailed,
	"Stopped":   JobStopped,
}

// LegacyTaskStatusMapping lists (old -> new) values for the task status DB migration.
var LegacyTaskStatusMapping = map[string]string{
	"PENDING":   TaskInitialized,
	"SUCCEEDED": TaskSucceed,
	"SUCCESS":   TaskSucceed,
	"IDLE":      TaskStaging,
	"Running":   TaskRunning,
	"Succeeded": TaskSucceed,
	"Failed":    TaskFailed,
	"Stopped":   TaskStopped,
	"Pending":   TaskInitialized,
}

// Result kinds (Java ResultKind enum names) stored in project_result.kind.
const (
	ResultFedTable = "FedTable"
	ResultModel    = "Model"
	ResultRule     = "Rule"
	ResultReport   = "Report"
	ResultReadData = "READ_DATA"
)

// ResultKindFromDatatable mirrors ResultKind.fromDatatable.
func ResultKindFromDatatable(t string) string {
	switch strings.ToLower(t) {
	case "table":
		return ResultFedTable
	case "model":
		return ResultModel
	case "rule":
		return ResultRule
	case "report":
		return ResultReport
	case "read_data":
		return ResultReadData
	}
	return ""
}

// OutputTypeFromResultKind mirrors GraphNodeOutputVO.typeFromResultKind.
func OutputTypeFromResultKind(kind string) string {
	switch kind {
	case ResultFedTable:
		return "table"
	case ResultModel:
		return "model"
	case ResultRule:
		return "rule"
	case ResultReport:
		return "report"
	case ResultReadData:
		return "read_data"
	}
	return ""
}
