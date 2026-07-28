package model

// VoteRequestDO represents a multi-party vote request.
type VoteRequestDO struct {
	BaseDO
	VoteID            string `gorm:"uniqueIndex:upk_vote_id;type:varchar(64);not null" json:"vote_id"`
	Initiator         string `gorm:"type:varchar(64);not null" json:"initiator"`
	Type              string `gorm:"type:varchar(16);not null" json:"type"`
	Voters            string `gorm:"type:text;not null" json:"voters"`
	VoteCounter       string `gorm:"type:varchar(64);not null" json:"vote_counter"`
	Executors         string `gorm:"type:text;not null" json:"executors"`
	ApprovedThreshold int    `gorm:"not null" json:"approved_threshold"`
	RequestMsg        string `gorm:"type:text" json:"request_msg"`
	Status            int8   `gorm:"not null" json:"status"`
	ExecuteStatus     string `gorm:"type:varchar(16);not null;default:'COMMITTED'" json:"execute_status"`
	Msg               string `gorm:"type:text" json:"msg"`
	PartyVoteInfo     string `gorm:"type:text" json:"party_vote_info"`
	Description       string `gorm:"type:varchar(64);not null" json:"description"`
}

func (VoteRequestDO) TableName() string { return "vote_request" }

// VoteInviteDO represents a vote invitation to a participant.
type VoteInviteDO struct {
	BaseDO
	VoteID            string `gorm:"uniqueIndex:upk_vote_invite_participant_id;type:varchar(64);not null" json:"vote_id"`
	Initiator         string `gorm:"type:varchar(64);not null" json:"initiator"`
	VoteParticipantID string `gorm:"uniqueIndex:upk_vote_invite_participant_id;type:varchar(64);not null" json:"vote_participant_id"`
	Type              string `gorm:"type:varchar(16);not null" json:"type"`
	VoteMsg           string `gorm:"type:text" json:"vote_msg"`
	Action            string `gorm:"type:varchar(16);default:'REVIEWING'" json:"action"` // REVIEWING / AGREE / REJECT
	Reason            string `gorm:"type:varchar(64)" json:"reason"`
	Description       string `gorm:"type:varchar(64);not null" json:"description"`
}

func (VoteInviteDO) TableName() string { return "vote_invite" }

// TeeDownloadApprovalConfigDO stores TEE download approval vote config.
type TeeDownloadApprovalConfigDO struct {
	BaseDO
	VoteID          string `gorm:"uniqueIndex:upk_tee_download_approval_config_vote_id;type:varchar(64);not null" json:"vote_id"`
	TaskID          string `gorm:"type:varchar(64);not null" json:"task_id"`
	JobID           string `gorm:"type:varchar(64);not null" json:"job_id"`
	ResourceID      string `gorm:"type:varchar(64);not null" json:"resource_id"`
	ResourceType    string `gorm:"type:varchar(16);not null" json:"resource_type"`
	ProjectID       string `gorm:"type:varchar(64);not null" json:"project_id"`
	GraphID         string `gorm:"type:varchar(64);not null" json:"graph_id"`
	AllParticipants string `gorm:"type:text;not null" json:"all_participants"`
}

func (TeeDownloadApprovalConfigDO) TableName() string { return "tee_download_approval_config" }

// NodeRouteApprovalConfigDO stores node route approval vote config.
type NodeRouteApprovalConfigDO struct {
	BaseDO
	VoteID          string `gorm:"uniqueIndex:upk_node_route_approval_config_vote_id;type:varchar(64);not null" json:"vote_id"`
	IsSingle        int8   `gorm:"not null" json:"is_single"`
	SrcNodeID       string `gorm:"type:varchar(64);not null" json:"src_node_id"`
	SrcNodeAddr     string `gorm:"type:varchar(64);not null" json:"src_node_addr"`
	DesNodeID       string `gorm:"type:varchar(64);not null" json:"des_node_id"`
	DesNodeAddr     string `gorm:"type:varchar(64);not null" json:"des_node_addr"`
	AllParticipants string `gorm:"type:text;not null" json:"all_participants"`
}

func (NodeRouteApprovalConfigDO) TableName() string { return "node_route_approval_config" }

// ProjectApprovalConfigDO stores project creation/join approval vote config.
type ProjectApprovalConfigDO struct {
	BaseDO
	VoteID       string `gorm:"uniqueIndex:upk_project_approval_config_vote_id;type:varchar(64);not null" json:"vote_id"`
	Initiator    string `gorm:"type:varchar(64);not null" json:"initiator"`
	Type         string `gorm:"type:varchar(16);not null" json:"type"`
	Parties      string `gorm:"type:text" json:"parties"`
	ProjectID    string `gorm:"type:varchar(64)" json:"project_id"`
	InviteNodeID string `gorm:"type:varchar(64)" json:"invite_node_id"`
}

func (ProjectApprovalConfigDO) TableName() string { return "project_approval_config" }
