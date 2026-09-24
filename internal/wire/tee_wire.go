package wire

import (
	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/pkg/config"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
)

// wireTee connects the TEE graph execution (trusted flow converter settings,
// per-node Kuscia pool), the TEE datatable authorization of project
// datatable/add and the TEE result pull service.
func wireTee(
	cfg *config.Config,
	pool *kuscia.ClientPool,
	graph *service.GraphService,
	project *service.ProjectService,
	datatable *service.DatatableService,
) *service.TeePullService {
	tc := cfg.Data.Tee
	graph.SetTeeConfig(service.TeeGraphConfig{
		DomainID:       tc.DomainID,
		CapsuleManager: tc.CapsuleManager,
		CertDir:        tc.CertDir,
	}).WithKusciaPool(pool)
	project.SetTeeAuthorizer(datatable)
	return service.NewTeePullService(datatable)
}
