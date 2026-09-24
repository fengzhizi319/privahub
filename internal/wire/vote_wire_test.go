package wire

import (
	"path/filepath"
	"testing"

	"github.com/fengzhizi319/privahub/internal/dao/repository"
	"github.com/fengzhizi319/privahub/internal/service"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestWireVote_TeePullerAndStop(t *testing.T) {
	t.Setenv("PRIVAHUB_VOTE_MONITOR_INITIAL_DELAY", "1h")
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "w.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	ns := service.NewNodeService(repository.NewNodeRepo(db), repository.NewNodeRouteRepo(db), nil, db)
	env := service.NewEnvService("center", "kuscia-system", "", nil)
	v := wireVote(db, nil, ns, service.NewCertificateService(ns), nil, env)
	if v.Service == nil || v.P2P == nil || v.Monitor == nil {
		t.Fatalf("vote wiring = %+v", v)
	}
	if v.Service.Deps().Kuscia != nil {
		t.Error("no kuscia client configured: KusciaOps must be nil (offline mode)")
	}
	if v.Service.Deps().Routes == nil {
		t.Error("route creator not wired")
	}
	tee := service.NewTeePullService(nil)
	v.Service.SetTeeResultPuller(tee)
	if v.Service.Deps().TeePuller != tee {
		t.Error("tee puller not installed")
	}
	v.Stop()
	var nilWiring *VoteWiring
	nilWiring.Stop()
}
