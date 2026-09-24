package graphjob

import (
	"fmt"
	"strings"
	"time"
)

const logTimeLayout = "2006-01-02 15:04:05"

// MakeLog mirrors ProjectJobTaskLogDO.makeLog: "{yyyy-MM-dd HH:mm:ss} INFO {content}".
func MakeLog(t time.Time, content string) string {
	return fmt.Sprintf("%s %s %s", t.Format(logTimeLayout), "INFO", content)
}

// TaskStartLog mirrors ProjectJobTaskLogDO.taskStartLog.
func TaskStartLog(t time.Time, jobID, taskID string) string {
	return MakeLog(t, fmt.Sprintf("the jobId=%s, taskId=%s start ...", jobID, taskID))
}

// TaskSucceedLog mirrors ProjectJobTaskLogDO.taskSucceedLog.
func TaskSucceedLog(t time.Time, jobID, taskID string) string {
	return MakeLog(t, fmt.Sprintf("the jobId=%s, taskId=%s succeed", jobID, taskID))
}

// TaskStoppedLog mirrors ProjectJobTaskLogDO.taskStoppedLog.
func TaskStoppedLog(t time.Time, jobID, taskID string) string {
	return MakeLog(t, fmt.Sprintf("the jobId=%s, taskId=%s stopped", jobID, taskID))
}

// TaskFailedLog mirrors ProjectJobTaskLogDO.taskFailedLog.
func TaskFailedLog(t time.Time, jobID, taskID, reason string) string {
	return MakeLog(t, fmt.Sprintf("the jobId=%s, taskId=%s failed: %s", jobID, taskID, reason))
}

// TransitionLogs mirrors JobTaskLogEventListener: the log lines written when a
// task moves from -> to. Java's switch falls through from STAGING/INITIALIZED
// into the RUNNING branch, which is reproduced here (and deduplicated).
func TransitionLogs(t time.Time, jobID, taskID, from, to string, reasons []string) []string {
	from, to = NormalizeTaskStatus(from), NormalizeTaskStatus(to)
	if from == to {
		return nil
	}
	var logs []string
	switch from {
	case TaskStaging, TaskInitialized, "":
		switch to {
		case TaskRunning:
			logs = append(logs, TaskStartLog(t, jobID, taskID))
		case TaskSucceed:
			logs = append(logs, TaskStartLog(t, jobID, taskID), TaskSucceedLog(t, jobID, taskID))
		case TaskStopped:
			logs = append(logs, TaskStoppedLog(t, jobID, taskID))
		case TaskFailed:
			logs = append(logs, TaskStartLog(t, jobID, taskID))
			for _, r := range reasons {
				logs = append(logs, TaskFailedLog(t, jobID, taskID, r))
			}
		}
		fallthrough
	case TaskRunning:
		switch to {
		case TaskStopped:
			logs = append(logs, TaskStoppedLog(t, jobID, taskID))
		case TaskSucceed:
			logs = append(logs, TaskSucceedLog(t, jobID, taskID))
		case TaskFailed:
			for _, r := range reasons {
				logs = append(logs, TaskFailedLog(t, jobID, taskID, r))
			}
		}
	}
	return Dedupe(logs)
}

// Dedupe removes duplicated lines keeping order.
func Dedupe(lines []string) []string {
	seen := make(map[string]bool, len(lines))
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		if seen[l] {
			continue
		}
		seen[l] = true
		out = append(out, l)
	}
	return out
}

// DistinctSpecifyLogs mirrors GraphServiceImpl.distinctSpecifyLogs: keep only
// the first non-failed line containing marker.
func DistinctSpecifyLogs(logs []string, marker string) []string {
	out := make([]string, 0, len(logs))
	seen := false
	for _, l := range logs {
		if strings.Contains(l, marker) && !strings.Contains(l, "failed") {
			if !seen {
				out = append(out, l)
			}
			seen = true
			continue
		}
		out = append(out, l)
	}
	return out
}
