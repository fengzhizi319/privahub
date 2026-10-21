package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/fengzhizi319/privahub/internal/audit"
	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/go-sql-driver/mysql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestMysqlDSNFromEnv(t *testing.T) {
	const dsn = "user@tcp(127.0.0.1:3306)/privahub?parseTime=true"
	cases := []struct {
		name    string
		dsn     string
		pwd     string
		want    string
		wantPwd string
	}{
		{
			name:    "MYSQL_PWD injected into passwordless DSN",
			dsn:     dsn,
			pwd:     "secret",
			want:    "user:secret@tcp(127.0.0.1:3306)/privahub?parseTime=true",
			wantPwd: "secret",
		},
		{
			name: "existing password kept, MYSQL_PWD ignored",
			dsn:  "user:kept@tcp(127.0.0.1:3306)/privahub",
			pwd:  "secret", wantPwd: "kept",
			want: "user:kept@tcp(127.0.0.1:3306)/privahub",
		},
		{
			name: "no MYSQL_PWD leaves DSN unchanged",
			dsn:  dsn,
			want: dsn,
		},
		{
			name: "unparsable DSN returned unchanged",
			dsn:  "not a : mysql dsn @ all/",
			pwd:  "secret",
			want: "not a : mysql dsn @ all/",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("MYSQL_PWD", tc.pwd)
			got := mysqlDSNFromEnv(tc.dsn)
			if got != tc.want {
				t.Fatalf("mysqlDSNFromEnv(%q) = %q, want %q", tc.dsn, got, tc.want)
			}
			if tc.wantPwd != "" {
				cfg, err := mysql.ParseDSN(got)
				if err != nil {
					t.Fatal(err)
				}
				if cfg.Passwd != tc.wantPwd {
					t.Fatalf("parsed password = %q, want %q", cfg.Passwd, tc.wantPwd)
				}
			}
		})
	}
}

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
