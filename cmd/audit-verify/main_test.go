package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/fengzhizi319/privahub/internal/audit"
	"github.com/fengzhizi319/privahub/internal/dao/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestRunValidAndBroken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.AuditLogDO{}, &model.AuditChainTailDO{}, &model.AuditSegmentDO{}); err != nil {
		t.Fatal(err)
	}
	s, err := audit.New(db, nil, audit.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s.Start()
	for i := 0; i < 5; i++ {
		s.Record(context.Background(), audit.Event{Actor: "u", Action: "x", Result: "success"})
	}
	if err := s.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if code := run([]string{"-driver", "sqlite", "-dsn", path}); code != 0 {
		t.Fatalf("valid chain exit %d", code)
	}
	db.Model(&model.AuditLogDO{}).Where("seq = 3").Update("actor", "mallory")
	if code := run([]string{"-driver", "sqlite", "-dsn", path, "-json"}); code != 2 {
		t.Fatalf("broken chain exit %d", code)
	}
}
