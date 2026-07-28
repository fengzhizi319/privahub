package service

import (
	"context"
	"testing"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/dao/repository"
	"github.com/fengzhizi319/privahub/pkg/auth"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	err = db.AutoMigrate(
		&model.UserAccountsDO{},
		&model.UserTokensDO{},
		&model.ProjectJobDO{},
		&model.ProjectJobTaskDO{},
		&model.ProjectJobTaskLogDO{},
	)
	if err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

func TestAuthService_Login_Success(t *testing.T) {
	db := setupTestDB(t)

	admin := &model.UserAccountsDO{
		Name:         "admin",
		PasswordHash: HashPassword("12345678"),
		OwnerType:    "CENTER",
		OwnerID:      "kuscia-system",
	}
	if err := db.Create(admin).Error; err != nil {
		t.Fatalf("failed to seed admin: %v", err)
	}

	jwtManager := auth.NewJWTManager("test-secret", 3600_000_000_000, 86400_000_000_000)
	svc := NewAuthService(
		repository.NewUserAccountsRepo(db),
		repository.NewUserTokensRepo(db),
		jwtManager,
	)

	resp, err := svc.Login(context.Background(), &LoginRequest{
		Username: "admin",
		Password: "12345678",
	})
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	if resp.AccessToken == "" {
		t.Error("expected non-empty access token")
	}
	if resp.User.Name != "admin" {
		t.Errorf("expected user name 'admin', got %q", resp.User.Name)
	}
}

func TestAuthService_Login_WrongPassword(t *testing.T) {
	db := setupTestDB(t)

	admin := &model.UserAccountsDO{
		Name:         "admin",
		PasswordHash: HashPassword("12345678"),
		OwnerType:    "CENTER",
		OwnerID:      "kuscia-system",
	}
	if err := db.Create(admin).Error; err != nil {
		t.Fatalf("failed to seed admin: %v", err)
	}

	jwtManager := auth.NewJWTManager("test-secret", 3600_000_000_000, 86400_000_000_000)
	svc := NewAuthService(
		repository.NewUserAccountsRepo(db),
		repository.NewUserTokensRepo(db),
		jwtManager,
	)

	_, err := svc.Login(context.Background(), &LoginRequest{
		Username: "admin",
		Password: "wrong-password",
	})
	if err == nil {
		t.Fatal("expected error for wrong password")
	}
}

func TestJobService_CreateAndList(t *testing.T) {
	db := setupTestDB(t)

	svc := NewJobService(
		repository.NewJobRepo(db),
		repository.NewTaskRepo(db),
		repository.NewTaskLogRepo(db),
		nil,
		nil,
		nil,
	)

	job, err := svc.CreateJob(context.Background(), &CreateJobRequest{
		ProjectID: "proj-1",
		Name:      "test-job",
		GraphID:   "graph-1",
	})
	if err != nil {
		t.Fatalf("CreateJob failed: %v", err)
	}
	if job.JobID == "" {
		t.Error("expected non-empty job ID")
	}
	if job.Status != "PENDING" {
		t.Errorf("expected status PENDING, got %q", job.Status)
	}

	listResp, err := svc.ListJobs(context.Background(), &JobListRequest{
		ProjectID: "proj-1",
		Page:      1,
		Size:      10,
	})
	if err != nil {
		t.Fatalf("ListJobs failed: %v", err)
	}
	if listResp.Total != 1 {
		t.Errorf("expected 1 job, got %d", listResp.Total)
	}
}

func TestJobService_StopJob(t *testing.T) {
	db := setupTestDB(t)

	svc := NewJobService(
		repository.NewJobRepo(db),
		repository.NewTaskRepo(db),
		repository.NewTaskLogRepo(db),
		nil,
		nil,
		nil,
	)

	job, _ := svc.CreateJob(context.Background(), &CreateJobRequest{
		ProjectID: "proj-1",
		Name:      "stop-test",
	})

	err := svc.StopJob(context.Background(), &StopJobRequest{
		ProjectID: "proj-1",
		JobID:     job.JobID,
	})
	if err != nil {
		t.Fatalf("StopJob failed: %v", err)
	}

	detail, err := svc.GetJob(context.Background(), &GetJobRequest{
		ProjectID: "proj-1",
		JobID:     job.JobID,
	})
	if err != nil {
		t.Fatalf("GetJob failed: %v", err)
	}
	if detail.Status != "STOPPED" {
		t.Errorf("expected status STOPPED, got %q", detail.Status)
	}
}

func TestMapKusciaState(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Pending", "PENDING"},
		{"Running", "RUNNING"},
		{"Succeeded", "SUCCEEDED"},
		{"Failed", "FAILED"},
		{"Stopped", "STOPPED"},
		{"Unknown", "Unknown"},
	}
	for _, tt := range tests {
		got := mapKusciaState(tt.input)
		if got != tt.expected {
			t.Errorf("mapKusciaState(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestIsTerminalStatus(t *testing.T) {
	terminal := []string{"SUCCEEDED", "FAILED", "STOPPED"}
	nonTerminal := []string{"PENDING", "RUNNING", "IDLE"}

	for _, s := range terminal {
		if !isTerminalStatus(s) {
			t.Errorf("expected %q to be terminal", s)
		}
	}
	for _, s := range nonTerminal {
		if isTerminalStatus(s) {
			t.Errorf("expected %q to be non-terminal", s)
		}
	}
}
