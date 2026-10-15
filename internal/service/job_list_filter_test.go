// Copyright 2026 Privahub Authors
// Licensed under the Apache License, Version 2.0.
package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/fengzhizi319/privahub/internal/dao/model"
)

func TestJobListFiltersAcrossPages(t *testing.T) {
	f := newGraphFixture(t)
	f.seedPSIProject()
	created := time.Date(2026, 10, 15, 1, 0, 0, 0, time.UTC)
	if err := f.db.Create(&model.ProjectGraphDO{ProjectID: "p1", GraphID: "filter-graph", Name: "SecureBoost"}).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 15; i++ {
		status := "SUCCEED"
		if i == 0 {
			status = "FAILED"
		}
		row := model.ProjectJobDO{BaseDO: model.BaseDO{GmtCreate: created.Add(time.Duration(i) * time.Minute)}, ProjectID: "p1", GraphID: "filter-graph", JobID: fmt.Sprintf("filter-%02d", i), Name: fmt.Sprintf("run_%02d", i), Status: status}
		if err := f.db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	// This record lives beyond the first unfiltered page; filtering must precede LIMIT.
	for _, search := range []string{"filter-00", "run_00", "SecureBoost"} {
		result, err := f.job.ListJobs(context.Background(), &JobListRequest{ProjectID: "p1", SearchKey: search, Status: "FAILED", PageNum: 1, PageSize: 2})
		if err != nil || result.Total != 1 || len(result.Data) != 1 || result.Data[0].JobID != "filter-00" {
			t.Fatalf("search %s: %+v %v", search, result, err)
		}
	}
	result, err := f.job.ListJobs(context.Background(), &JobListRequest{ProjectID: "p1", Status: "SUCCESS", PageNum: 2, PageSize: 2})
	if err != nil || result.Total != 14 || len(result.Data) != 2 {
		t.Fatalf("filtered pagination %+v %v", result, err)
	}
	result, err = f.job.ListJobs(context.Background(), &JobListRequest{ProjectID: "p1", StartTime: "2026-10-15T01:00:00Z", EndTime: "2026-10-15T01:02:00Z"})
	if err != nil || result.Total != 3 {
		t.Fatalf("date boundaries %+v %v", result, err)
	}
	for _, search := range []string{"%", "' OR 1=1 --", "run_99"} {
		result, err = f.job.ListJobs(context.Background(), &JobListRequest{ProjectID: "p1", SearchKey: search})
		if err != nil || result.Total != 0 {
			t.Fatalf("literal search %q: %+v %v", search, result, err)
		}
	}
	result, err = f.job.ListJobs(context.Background(), &JobListRequest{ProjectID: "different-project", SearchKey: "SecureBoost"})
	if err != nil || result.Total != 0 {
		t.Fatalf("project boundary %+v %v", result, err)
	}
	for _, req := range []*JobListRequest{{ProjectID: "p1", Status: "DROP TABLE"}, {ProjectID: "p1", StartTime: "invalid"}, {ProjectID: "p1", StartTime: "2026-10-16T00:00:00Z", EndTime: "2026-10-15T00:00:00Z"}} {
		if _, err = f.job.ListJobs(context.Background(), req); err == nil {
			t.Fatalf("invalid request accepted %+v", req)
		}
	}
}

func TestScheduledExecutionFilters(t *testing.T) {
	f := newSchedFixture(t)
	if err := f.svc.CreateScheduler(f.ctx, dailyCreate("filtered-schedule")); err != nil {
		t.Fatal(err)
	}
	task := f.tasks(t)[0]
	if err := f.db.Model(&model.ProjectScheduleTaskDO{}).Where("id = ?", task.ID).Update("status", model.ScheduledStatusRunning).Error; err != nil {
		t.Fatal(err)
	}
	result, err := f.svc.ListProjectJob(f.ctx, &ScheduleListProjectJobRequest{ProjectID: "p1", GraphID: "g1", SearchKey: task.ScheduleJobID, Status: "RUNNING", PageNum: 1, PageSize: 1})
	if err != nil || result.Total != 1 || len(result.Data) != 1 {
		t.Fatalf("execution filter %+v %v", result, err)
	}
	result, err = f.svc.ListProjectJob(f.ctx, &ScheduleListProjectJobRequest{ProjectID: "p1", GraphID: "g1", SearchKey: "no-match"})
	if err != nil || result.Total != 0 {
		t.Fatalf("empty filter %+v %v", result, err)
	}
}
