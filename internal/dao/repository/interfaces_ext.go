package repository

import (
	"context"

	"github.com/fengzhizi319/privahub/internal/dao/model"
)

// GraphRepository provides access to DAG graph data.
type GraphRepository interface {
	BaseRepository[model.ProjectGraphDO]
	FindByProjectAndGraphID(ctx context.Context, projectID, graphID string) (*model.ProjectGraphDO, error)
	FindByProjectID(ctx context.Context, projectID string) ([]model.ProjectGraphDO, error)
}

// GraphNodeRepository provides access to DAG graph node data.
type GraphNodeRepository interface {
	BaseRepository[model.ProjectGraphNodeDO]
	FindByGraphID(ctx context.Context, projectID, graphID string) ([]model.ProjectGraphNodeDO, error)
	FindByGraphNodeID(ctx context.Context, projectID, graphID, graphNodeID string) (*model.ProjectGraphNodeDO, error)
	DeleteByGraphID(ctx context.Context, projectID, graphID string) error
}

// JobRepository provides access to Kuscia job data.
type JobRepository interface {
	BaseRepository[model.ProjectJobDO]
	FindByJobID(ctx context.Context, jobID string) (*model.ProjectJobDO, error)
	FindByProjectID(ctx context.Context, projectID string) ([]model.ProjectJobDO, error)
	FindByProjectAndJobID(ctx context.Context, projectID, jobID string) (*model.ProjectJobDO, error)
	UpdateStatus(ctx context.Context, jobID, status, errMsg string) error
}

// TaskRepository provides access to Kuscia task data.
type TaskRepository interface {
	BaseRepository[model.ProjectJobTaskDO]
	FindByJobID(ctx context.Context, jobID string) ([]model.ProjectJobTaskDO, error)
	FindByProjectAndJobID(ctx context.Context, projectID, jobID string) ([]model.ProjectJobTaskDO, error)
	FindByTaskID(ctx context.Context, taskID string) (*model.ProjectJobTaskDO, error)
	UpdateStatus(ctx context.Context, taskID, status, errMsg string) error
}

// TaskLogRepository provides access to task execution logs.
type TaskLogRepository interface {
	Create(ctx context.Context, log *model.ProjectJobTaskLogDO) error
	FindByTaskID(ctx context.Context, projectID, jobID, taskID string) ([]model.ProjectJobTaskLogDO, error)
}

// DatatableRepository provides access to project datatable associations.
type DatatableRepository interface {
	BaseRepository[model.ProjectDatatableDO]
	FindByProjectID(ctx context.Context, projectID string) ([]model.ProjectDatatableDO, error)
	FindByNodeID(ctx context.Context, nodeID string) ([]model.ProjectDatatableDO, error)
	FindByProjectAndNodeID(ctx context.Context, projectID, nodeID string) ([]model.ProjectDatatableDO, error)
	FindByProjectNodeDatatable(ctx context.Context, projectID, nodeID, datatableID string) (*model.ProjectDatatableDO, error)
	FindAll(ctx context.Context) ([]model.ProjectDatatableDO, error)
}

// FedTableRepository provides access to federated table data.
type FedTableRepository interface {
	BaseRepository[model.ProjectFedTableDO]
	FindByProjectID(ctx context.Context, projectID string) ([]model.ProjectFedTableDO, error)
	FindByFedTableID(ctx context.Context, fedTableID string) (*model.ProjectFedTableDO, error)
}

// ModelPackRepository provides access to model pack data.
type ModelPackRepository interface {
	BaseRepository[model.ProjectModelPackDO]
	FindByModelID(ctx context.Context, modelID string) (*model.ProjectModelPackDO, error)
	FindByProjectID(ctx context.Context, projectID string) ([]model.ProjectModelPackDO, error)
}

// ModelServingRepository provides access to model serving data.
type ModelServingRepository interface {
	BaseRepository[model.ProjectModelServingDO]
	FindByServingID(ctx context.Context, servingID string) (*model.ProjectModelServingDO, error)
	FindByProjectID(ctx context.Context, projectID string) ([]model.ProjectModelServingDO, error)
}

// UserAccountsRepository provides access to user account data.
type UserAccountsRepository interface {
	BaseRepository[model.UserAccountsDO]
	FindByName(ctx context.Context, name string) (*model.UserAccountsDO, error)
	UpdatePassword(ctx context.Context, name, passwordHash string) error
}

// UserTokensRepository provides access to user session tokens.
type UserTokensRepository interface {
	BaseRepository[model.UserTokensDO]
	FindByToken(ctx context.Context, token string) (*model.UserTokensDO, error)
	FindByName(ctx context.Context, name string) ([]model.UserTokensDO, error)
	DeleteByName(ctx context.Context, name string) error
}

// VoteRequestRepository provides access to vote request data.
type VoteRequestRepository interface {
	BaseRepository[model.VoteRequestDO]
	FindByVoteID(ctx context.Context, voteID string) (*model.VoteRequestDO, error)
	FindByInitiator(ctx context.Context, initiator string) ([]model.VoteRequestDO, error)
	FindByVoter(ctx context.Context, voter string) ([]model.VoteRequestDO, error)
}

// VoteInviteRepository provides access to vote invitation data.
type VoteInviteRepository interface {
	BaseRepository[model.VoteInviteDO]
	FindByVoteID(ctx context.Context, voteID string) ([]model.VoteInviteDO, error)
	FindByParticipant(ctx context.Context, participantID string) ([]model.VoteInviteDO, error)
	FindByVoteAndParticipant(ctx context.Context, voteID, participantID string) (*model.VoteInviteDO, error)
}

// SysResourceRepository provides access to system resource data.
type SysResourceRepository interface {
	BaseRepository[model.SysResourceDO]
	FindByResourceCode(ctx context.Context, code string) (*model.SysResourceDO, error)
	FindByResourceType(ctx context.Context, resourceType string) ([]model.SysResourceDO, error)
}

// SysRoleRepository provides access to system role data.
type SysRoleRepository interface {
	BaseRepository[model.SysRoleDO]
	FindByRoleCode(ctx context.Context, roleCode string) (*model.SysRoleDO, error)
}

// SysUserPermissionRepository provides access to user permission data.
type SysUserPermissionRepository interface {
	FindByUserKey(ctx context.Context, userKey string) ([]model.SysUserPermissionRelDO, error)
	Create(ctx context.Context, rel *model.SysUserPermissionRelDO) error
	DeleteByUserKey(ctx context.Context, userKey string) error
}

// SysUserNodeRepository provides access to user-node relation data.
type SysUserNodeRepository interface {
	FindByUserID(ctx context.Context, userID string) ([]model.SysUserNodeRelDO, error)
	Create(ctx context.Context, rel *model.SysUserNodeRelDO) error
	DeleteByUserID(ctx context.Context, userID string) error
}
