// Copyright 2026 Privahub Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at http://www.apache.org/licenses/LICENSE-2.0
package service

import (
	"strings"
	"time"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"gorm.io/gorm"
)

// applyJobListFilters runs before both COUNT and LIMIT, so search results and
// pagination describe the same full project dataset. Values are SQL parameters;
// wildcard escaping makes user-entered '%' and '_' literal search characters.
func applyJobListFilters(q *gorm.DB, projectID, search, status, start, end string) (*gorm.DB, error) {
	var from, until time.Time
	var err error
	if start != "" {
		from, err = time.Parse(time.RFC3339, start)
		if err != nil {
			return nil, padErr(errcode.ParamError)
		}
	}
	if end != "" {
		until, err = time.Parse(time.RFC3339, end)
		if err != nil {
			return nil, padErr(errcode.ParamError)
		}
	}
	if !from.IsZero() && !until.IsZero() && until.Before(from) {
		return nil, padErr(errcode.ParamError)
	}
	if !from.IsZero() {
		q = q.Where("gmt_create >= ?", from.UTC())
	}
	if !until.IsZero() {
		q = q.Where("gmt_create <= ?", until.UTC())
	}
	if status = strings.ToUpper(strings.TrimSpace(status)); status != "" {
		statuses := map[string][]string{
			"SUCCESS": {"SUCCESS", "SUCCEED", "SUCCEEDED"}, "RUNNING": {"RUNNING", "INITIALIZED"},
			"FAILED": {"FAILED", "FAILURE", "ERROR"}, "STOPPED": {"STOPPED", "TERMINATED"}, "STAGING": {"STAGING", "PENDING", "WAITING"},
		}
		allowed, ok := statuses[status]
		if !ok {
			return nil, padErr(errcode.ParamError)
		}
		q = q.Where("UPPER(status) IN ?", allowed)
	}
	if keyword := strings.TrimSpace(search); keyword != "" {
		if len(keyword) > 256 {
			return nil, padErr(errcode.ParamError)
		}
		pattern := "%" + strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(strings.ToLower(keyword)) + "%"
		graphs := q.Session(&gorm.Session{NewDB: true}).Model(&model.ProjectGraphDO{}).Select("graph_id").Where("project_id = ? AND LOWER(name) LIKE ? ESCAPE '!'", projectID, pattern)
		q = q.Where("(LOWER(job_id) LIKE ? ESCAPE '!' OR LOWER(name) LIKE ? ESCAPE '!' OR graph_id IN (?))", pattern, pattern, graphs)
	}
	return q, nil
}
