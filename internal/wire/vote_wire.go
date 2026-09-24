package wire

import (
	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/internal/service/votehandler"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// VoteWiring holds the vote / approval / message / P2P services and the
// vote status monitors (Java VoteRequestStatusMonitor + VoteInviteStatusMonitor).
//
// The sync subsystem plugs the EDGE→center channel in with
// Vote.Service.SetDbSyncSender(sender), and the TEE result pull with
// Vote.Service.SetTeeResultPuller(puller).
type VoteWiring struct {
	Service *votehandler.Service
	P2P     *votehandler.P2PService
	Monitor *votehandler.Monitor
}

// wireVote builds the vote system on top of the node services and starts the
// monitors. strictSignatures (production) makes vote signature verification
// fail closed when Kuscia is unavailable.
func wireVote(db *gorm.DB, log *zap.Logger, nodeService *service.NodeService, certs *service.CertificateService,
	kusciaClient *kuscia.Client, env *service.EnvService, strictSignatures bool) *VoteWiring {
	if log == nil {
		log = zap.NewNop()
	}
	deps := &votehandler.Deps{
		DB:     db,
		Env:    env,
		Kuscia: votehandler.NewKusciaOps(nodeService, certs, kusciaClient),
		Log:    log.Named("vote"),

		StrictSignatures: strictSignatures,
	}
	if nodeService != nil {
		deps.Routes = nodeService.Routes()
	}
	svc := votehandler.NewService(deps)
	monitor := votehandler.NewMonitor(svc, votehandler.MonitorOptionsFromEnv())
	monitor.Start()
	return &VoteWiring{Service: svc, P2P: votehandler.NewP2PService(deps), Monitor: monitor}
}

// Stop stops the vote monitors.
func (v *VoteWiring) Stop() {
	if v != nil && v.Monitor != nil {
		v.Monitor.Stop()
	}
}
