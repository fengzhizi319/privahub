// Package migration provides database schema migration and seed data initialization.
package migration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// AllModels returns all database models for auto-migration.
func AllModels() []interface{} {
	return []interface{}{
		// Core
		&model.InstDO{},
		&model.NodeDO{},
		&model.NodeRouteDO{},
		&model.ProjectDO{},
		&model.ProjectInstDO{},
		&model.ProjectNodeDO{},

		// Graph
		&model.ProjectGraphDO{},
		&model.ProjectGraphNodeDO{},
		&model.ProjectGraphNodeKusciaParamsDO{},

		// Job
		&model.ProjectJobDO{},
		&model.ProjectJobTaskDO{},
		&model.ProjectJobTaskLogDO{},

		// Datatable
		&model.ProjectDatatableDO{},
		&model.ProjectFedTableDO{},
		&model.TeeNodeDatatableManagementDO{},
		&model.FeatureTableDO{},
		&model.ProjectFeatureTableDO{},

		// Datasource
		&model.DatasourceDO{},
		&model.DatasourceNodeDO{},
		&model.ProjectGraphDomainDatasourceDO{},

		// Model/Serving
		&model.ProjectModelDO{},
		&model.ProjectModelPackDO{},
		&model.ProjectModelServingDO{},

		// User/Auth
		&model.UserAccountsDO{},
		&model.UserTokensDO{},
		&model.SysResourceDO{},
		&model.SysRoleDO{},
		&model.SysRoleResourceRelDO{},
		&model.SysUserPermissionRelDO{},
		&model.SysUserNodeRelDO{},

		// Vote
		&model.VoteRequestDO{},
		&model.VoteInviteDO{},
		&model.TeeDownloadApprovalConfigDO{},
		&model.NodeRouteApprovalConfigDO{},
		&model.ProjectApprovalConfigDO{},

		// Misc
		&model.ProjectRuleDO{},
		&model.ProjectReportDO{},
		&model.ProjectReadDataDO{},
		&model.ProjectResultDO{},
		&model.EdgeDataSyncLogDO{},
		&model.ProjectScheduleTaskDO{},
		&model.CertificateDO{},
	}
}

// AutoMigrate runs GORM auto-migration for all models.
func AutoMigrate(db *gorm.DB, log *zap.Logger) error {
	log.Info("Running database auto-migration...")
	if err := db.AutoMigrate(AllModels()...); err != nil {
		return err
	}
	log.Info("Database auto-migration completed")
	return nil
}

// SeedData initializes default data if not exists.
func SeedData(ctx context.Context, db *gorm.DB, log *zap.Logger) error {
	log.Info("Seeding initial data...")

	// Seed default institutions
	if err := seedInstitutions(ctx, db); err != nil {
		return err
	}

	// Seed default nodes
	if err := seedNodes(ctx, db); err != nil {
		return err
	}

	// Seed default admin user
	if err := seedAdminUser(ctx, db); err != nil {
		return err
	}

	// Seed default roles and resources
	if err := seedRBAC(ctx, db); err != nil {
		return err
	}

	// Seed default datasources
	if err := seedDatasources(ctx, db); err != nil {
		return err
	}

	// Seed default projects
	if err := seedProjects(ctx, db); err != nil {
		return err
	}

	// Seed default datatables
	if err := seedDatatables(ctx, db); err != nil {
		return err
	}

	// Seed default graphs
	if err := seedGraphs(ctx, db); err != nil {
		return err
	}

	log.Info("Initial data seeding completed")
	return nil
}

func seedInstitutions(ctx context.Context, db *gorm.DB) error {
	insts := []model.InstDO{
		{InstID: "alice", Name: "alice"},
		{InstID: "bob", Name: "bob"},
		{InstID: "admin", Name: "admin"},
	}

	for _, inst := range insts {
		var count int64
		db.WithContext(ctx).Model(&model.InstDO{}).Where("inst_id = ?", inst.InstID).Count(&count)
		if count == 0 {
			if err := db.WithContext(ctx).Create(&inst).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func seedNodes(ctx context.Context, db *gorm.DB) error {
	nodes := []model.NodeDO{
		{
			NodeID:        "alice",
			Name:          "alice",
			ControlNodeID: "alice",
			Auth:          "alice",
			Description:   "alice node",
			Type:          "embedded",
			NetAddress:    "127.0.0.1:28080",
			Mode:          1,
			MasterNodeID:  "master",
		},
		{
			NodeID:        "bob",
			Name:          "bob",
			ControlNodeID: "bob",
			Auth:          "bob",
			Description:   "bob node",
			Type:          "embedded",
			NetAddress:    "127.0.0.1:38080",
			Mode:          1,
			MasterNodeID:  "master",
		},
	}

	for _, node := range nodes {
		var count int64
		db.WithContext(ctx).Model(&model.NodeDO{}).Where("node_id = ?", node.NodeID).Count(&count)
		if count == 0 {
			if err := db.WithContext(ctx).Create(&node).Error; err != nil {
				return err
			}
		}
	}

	// Seed default routes
	routes := []model.NodeRouteDO{
		{RouteID: "1", SrcNodeID: "alice", DstNodeID: "bob", SrcNetAddress: "127.0.0.1:28080", DstNetAddress: "127.0.0.1:38080"},
		{RouteID: "2", SrcNodeID: "bob", DstNodeID: "alice", SrcNetAddress: "127.0.0.1:38080", DstNetAddress: "127.0.0.1:28080"},
	}

	for _, route := range routes {
		var count int64
		db.WithContext(ctx).Model(&model.NodeRouteDO{}).Where("src_node_id = ? AND dst_node_id = ?", route.SrcNodeID, route.DstNodeID).Count(&count)
		if count == 0 {
			if err := db.WithContext(ctx).Create(&route).Error; err != nil {
				return err
			}
		}
	}

	return nil
}

func seedAdminUser(ctx context.Context, db *gorm.DB) error {
	var count int64
	db.WithContext(ctx).Model(&model.UserAccountsDO{}).Where("name = ?", "admin").Count(&count)
	if count > 0 {
		return nil
	}

	// Default password: 12345678 (SHA-256 hash)
	passwordHash := hashPassword("12345678")

	admin := &model.UserAccountsDO{
		Name:         "admin",
		PasswordHash: passwordHash,
		OwnerType:    "CENTER",
		OwnerID:      "kuscia-system",
	}

	return db.WithContext(ctx).Create(admin).Error
}

func seedRBAC(ctx context.Context, db *gorm.DB) error {
	// Seed default role
	var roleCount int64
	db.WithContext(ctx).Model(&model.SysRoleDO{}).Where("role_code = ?", "ADMIN").Count(&roleCount)
	if roleCount == 0 {
		role := &model.SysRoleDO{
			RoleCode: "ADMIN",
			RoleName: "Administrator",
		}
		if err := db.WithContext(ctx).Create(role).Error; err != nil {
			return err
		}
	}

	// Seed ALL_INTERFACE_RESOURCE
	var resCount int64
	db.WithContext(ctx).Model(&model.SysResourceDO{}).Where("resource_code = ?", "ALL_INTERFACE_RESOURCE").Count(&resCount)
	if resCount == 0 {
		resource := &model.SysResourceDO{
			ResourceType: "API",
			ResourceCode: "ALL_INTERFACE_RESOURCE",
			ResourceName: "ALL_INTERFACE_RESOURCE",
		}
		if err := db.WithContext(ctx).Create(resource).Error; err != nil {
			return err
		}
	}

	// Assign admin role to admin user
	var permCount int64
	db.WithContext(ctx).Model(&model.SysUserPermissionRelDO{}).Where("user_key = ? AND target_code = ?", "admin", "ADMIN").Count(&permCount)
	if permCount == 0 {
		perm := &model.SysUserPermissionRelDO{
			UserType:   "USER",
			UserKey:    "admin",
			TargetType: "ROLE",
			TargetCode: "ADMIN",
		}
		if err := db.WithContext(ctx).Create(perm).Error; err != nil {
			return err
		}
	}

	return nil
}

func hashPassword(password string) string {
	hash := sha256.Sum256([]byte(password))
	return hex.EncodeToString(hash[:])
}

func seedDatasources(ctx context.Context, db *gorm.DB) error {
	datasources := []model.DatasourceDO{
		{
			DatasourceID:   "ds-alice",
			Name:           "alice-default-ds",
			Type:           "LOCAL_FS",
			Status:         "Available",
			OwnerID:        "alice",
			ConnectionInfo: `{"path":"/home/kuscia/var/storage"}`,
			Description:    "Default local filesystem datasource for Alice",
		},
		{
			DatasourceID:   "ds-bob",
			Name:           "bob-default-ds",
			Type:           "LOCAL_FS",
			Status:         "Available",
			OwnerID:        "bob",
			ConnectionInfo: `{"path":"/home/kuscia/var/storage"}`,
			Description:    "Default local filesystem datasource for Bob",
		},
	}

	for _, ds := range datasources {
		var count int64
		db.WithContext(ctx).Model(&model.DatasourceDO{}).Where("datasource_id = ?", ds.DatasourceID).Count(&count)
		if count == 0 {
			if err := db.WithContext(ctx).Create(&ds).Error; err != nil {
				return err
			}
			db.WithContext(ctx).Create(&model.DatasourceNodeDO{
				DatasourceID: ds.DatasourceID,
				NodeID:       ds.OwnerID,
			})
		}
	}
	return nil
}

func seedProjects(ctx context.Context, db *gorm.DB) error {
	var count int64
	db.WithContext(ctx).Model(&model.ProjectDO{}).Where("project_id = ?", "p-default").Count(&count)
	if count == 0 {
		project := &model.ProjectDO{
			ProjectID:   "p-default",
			Name:        "SecretFlow Demo Project",
			ComputeMode: "MPC",
			Status:      0,
			Description: "Default MPC Project for Alice and Bob",
		}
		if err := db.WithContext(ctx).Create(project).Error; err != nil {
			return err
		}
		nodes := []string{"alice", "bob"}
		for _, nodeID := range nodes {
			db.WithContext(ctx).Create(&model.ProjectNodeDO{
				ProjectID: "p-default",
				NodeID:    nodeID,
			})
		}
	}
	return nil
}

func seedDatatables(ctx context.Context, db *gorm.DB) error {
	datatables := []model.ProjectDatatableDO{
		{
			ProjectID:    "p-default",
			NodeID:       "alice",
			DatatableID:  "alice_table",
			Source:       "IMPORTED",
			TableConfigs: `{"tablename":"alice_table","datasourceId":"ds-alice","relativeUri":"alice.csv","columns":[{"colName":"id","colType":"string"},{"colName":"age","colType":"int"},{"colName":"education","colType":"string"},{"colName":"income","colType":"float"}]}`,
		},
		{
			ProjectID:    "p-default",
			NodeID:       "bob",
			DatatableID:  "bob_table",
			Source:       "IMPORTED",
			TableConfigs: `{"tablename":"bob_table","datasourceId":"ds-bob","relativeUri":"bob.csv","columns":[{"colName":"id","colType":"string"},{"colName":"y","colType":"int"},{"colName":"balance","colType":"float"}]}`,
		},
	}

	for _, dt := range datatables {
		var count int64
		db.WithContext(ctx).Model(&model.ProjectDatatableDO{}).
			Where("project_id = ? AND node_id = ? AND datatable_id = ?", dt.ProjectID, dt.NodeID, dt.DatatableID).
			Count(&count)
		if count == 0 {
			if err := db.WithContext(ctx).Create(&dt).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func seedGraphs(ctx context.Context, db *gorm.DB) error {
	var count int64
	db.WithContext(ctx).Model(&model.ProjectGraphDO{}).Where("graph_id = ?", "g-demo").Count(&count)
	if count == 0 {
		graph := &model.ProjectGraphDO{
			ProjectID:      "p-default",
			GraphID:        "g-demo",
			Name:           "SecretFlow Demo Pipeline",
			Edges:          `[{"edgeId":"e1","source":"n1","target":"n2"}]`,
			NodeMaxIndex:   2,
			MaxParallelism: 1,
		}
		if err := db.WithContext(ctx).Create(graph).Error; err != nil {
			return err
		}
		nodes := []model.ProjectGraphNodeDO{
			{
				ProjectID:   "p-default",
				GraphID:     "g-demo",
				GraphNodeID: "n1",
				CodeName:    "data_prep/csv_data_import",
				Label:       "Data Import",
				X:           150,
				Y:           150,
				Inputs:      "[]",
				Outputs:     `["n1-output-0"]`,
				NodeDef:     `{"domain":"data_prep","name":"csv_data_import","version":"1.0.0"}`,
			},
			{
				ProjectID:   "p-default",
				GraphID:     "g-demo",
				GraphNodeID: "n2",
				CodeName:    "data_prep/psi",
				Label:       "Private Set Intersection",
				X:           400,
				Y:           150,
				Inputs:      `["n1-output-0"]`,
				Outputs:     `["n2-output-0"]`,
				NodeDef:     `{"domain":"data_prep","name":"psi","version":"1.0.0"}`,
			},
		}
		for _, node := range nodes {
			db.WithContext(ctx).Create(&node)
		}
	}
	return nil
}
