package wire

import (
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	v1 "github.com/fengzhizi319/privahub/internal/controller/http/v1"
	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/pkg/config"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
)

// DataWiring groups the datasource / datatable / data / feature-table /
// cloud-log / partition-rule services (config section `data`, pkg/config/data.go).
type DataWiring struct {
	IPFilter             *service.IPFilter
	OssService           *service.OssService
	DataService          *service.DataService
	CloudLogService      *service.CloudLogService
	PartitionRuleService *service.PartitionRuleService
}

// wireData configures the data-management services from cfg.Data and
// connects them (datasource relatedDatas <- datatable service, per-node Kuscia
// routing via the client pool, TEE job status poller).
func wireData(
	db *gorm.DB,
	cfg *config.Config,
	log *zap.Logger,
	kc *kuscia.Client,
	pool *kuscia.ClientPool,
	env *service.EnvService,
	datatableSvc *service.DatatableService,
	datasourceSvc *service.DatasourceService,
	featureSvc *service.FeatureTableService,
	dataHandler *v1.DataHandler,
) *DataWiring {
	dc := cfg.Data
	ipFilter := service.NewIPFilter(&dc.IPBlock)
	oss := service.NewOssServiceWithFilter(ipFilter)

	datatableSvc.WithEnv(env).WithIPFilter(ipFilter).WithLogger(log).WithKusciaPool(pool).
		WithTee(dc.Tee.DomainID, dc.Tee.CapsuleManager, dc.Tee.MaxParallelism)
	datasourceSvc.WithEnv(env).WithIPFilter(ipFilter).WithLogger(log).WithKusciaPool(pool).
		WithDatatableFinder(datatableSvc)
	featureSvc.WithIPFilter(ipFilter)

	dataSvc := service.NewDataService(db, kc, env, dc.Dir).WithKusciaPool(pool)
	cloudLog := service.NewCloudLogService(db, env, dc.CloudLog.SLS)
	dataHandler.WithServices(dataSvc, featureSvc, cloudLog)

	datatableSvc.StartTeeJobSync(10 * time.Second)

	return &DataWiring{
		IPFilter:             ipFilter,
		OssService:           oss,
		DataService:          dataSvc,
		CloudLogService:      cloudLog,
		PartitionRuleService: service.NewPartitionRuleService(),
	}
}
