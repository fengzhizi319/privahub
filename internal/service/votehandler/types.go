// Package votehandler implements the Java SecretPad vote/approval system:
// VoteTypeHandler (PROJECT_CREATE / PROJECT_ARCHIVE / PROJECT_NODE_ADD /
// NODE_ROUTE / TEE_DOWNLOAD), MessageServiceImpl, ApprovalServiceImpl, the
// P2P project / node services and the two vote status monitors
// (VoteRequestStatusMonitor / VoteInviteStatusMonitor).
package votehandler

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/fengzhizi319/privahub/internal/service"
)

// Vote types (Java VoteTypeEnum).
const (
	VoteTypeTeeDownload    = "TEE_DOWNLOAD"
	VoteTypeNodeRoute      = "NODE_ROUTE"
	VoteTypeProjectCreate  = "PROJECT_CREATE"
	VoteTypeProjectArchive = "PROJECT_ARCHIVE"
	VoteTypeProjectNodeAdd = "PROJECT_NODE_ADD"
)

// Vote status codes stored in vote_request.status (Java VoteStatusEnum).
const (
	StatusReviewing    int8 = 0
	StatusApproved     int8 = 1
	StatusRejected     int8 = 2
	StatusNotInitiated int8 = 3
)

// Vote status / invite action names (Java VoteStatusEnum names).
const (
	ActionReviewing    = "REVIEWING"
	ActionApproved     = "APPROVED"
	ActionRejected     = "REJECTED"
	ActionNotInitiated = "NOT_INITIATED"
)

// Vote execute status (Java VoteExecuteEnum).
const (
	ExecExecuting = "EXECUTING"
	ExecCommitted = "COMMITTED"
	ExecSuccess   = "SUCCESS"
	ExecObserver  = "OBSERVER"
	ExecFailed    = "FAILED"
)

// Project status (Java ProjectStatusEnum).
const (
	ProjectReviewing int8 = 0
	ProjectApproved  int8 = 1
	ProjectArchived  int8 = 2
)

// Sync data types (Java VoteSyncTypeEnum).
const (
	SyncVoteRequest           = "VOTE_REQUEST"
	SyncVoteInvite            = "VOTE_INVITE"
	SyncProjectApprovalConfig = "PROJECT_APPROVAL_CONFIG"
	SyncProject               = "PROJECT"
	SyncProjectNode           = "PROJECT_NODE"
	SyncProjectInst           = "PROJECT_INST"
)

// resultKindFedTable is Java ResultKind.FedTable.getName().
const resultKindFedTable = "table"

// teeExecutor marks a TEE node route vote (Java executors.contains("tee")).
const teeExecutor = "tee"

// VoteStatusName returns the Java VoteStatusEnum name of a status code ("" when unknown).
func VoteStatusName(code int8) string {
	switch code {
	case StatusReviewing:
		return ActionReviewing
	case StatusApproved:
		return ActionApproved
	case StatusRejected:
		return ActionRejected
	case StatusNotInitiated:
		return ActionNotInitiated
	}
	return ""
}

// ProjectStatusName returns the Java ProjectStatusEnum name ("" when unknown).
func ProjectStatusName(code int8) string {
	switch code {
	case ProjectReviewing:
		return "REVIEWING"
	case ProjectApproved:
		return "APPROVED"
	case ProjectArchived:
		return "ARCHIVED"
	}
	return ""
}

// --- signed vote messages (Java model/approval, Kuscia approval proto JSON names) ---

// VoteRequestBody is the base64-encoded body of a VoteRequestMessage.
type VoteRequestBody struct {
	VoteRequestID     string   `json:"vote_request_id"`
	Type              string   `json:"type"`
	Initiator         string   `json:"initiator"`
	VoteCounter       string   `json:"vote_counter"`
	Voters            []string `json:"voters"`
	Executors         []string `json:"executors"`
	ApprovedThreshold int      `json:"approved_threshold"`
	ApprovedAction    string   `json:"approved_action,omitempty"`
	RejectedAction    string   `json:"rejected_action,omitempty"`
}

// VoteRequestMessage is stored in vote_request.request_msg / vote_invite.vote_msg.
type VoteRequestMessage struct {
	CertChain            []string `json:"cert_chain,omitempty"`
	Body                 string   `json:"body"`
	VoteRequestSignature string   `json:"vote_request_signature,omitempty"`
}

// VoteReplyBody is the base64-encoded body of a VoteReplyMessage.
type VoteReplyBody struct {
	VoteRequestID string `json:"vote_request_id"`
	Voter         string `json:"voter"`
	Action        string `json:"action"` // APPROVE | REJECT
}

// VoteReplyMessage is stored in vote_invite.vote_msg after a reply.
type VoteReplyMessage struct {
	CertChain []string `json:"cert_chain,omitempty"`
	Body      string   `json:"body"`
	Signature string   `json:"voter_signature"`
}

// VoteResult bundles a request and its replies (TEE download proof).
type VoteResult struct {
	VoteInvite  []VoteReplyMessage `json:"vote_invite"`
	VoteRequest VoteRequestMessage `json:"vote_request"`
}

// PartyVoteInfo is one element of vote_request.party_vote_info (JSON set keyed by partyId).
type PartyVoteInfo struct {
	PartyID string `json:"partyId"`
	Action  string `json:"action"`
	Reason  string `json:"reason,omitempty"`
}

// ParticipantNodeInstVO mirrors Java ParticipantNodeInstVO.
type ParticipantNodeInstVO struct {
	InitiatorNodeID   string       `json:"initiatorNodeId"`
	InitiatorNodeName string       `json:"initiatorNodeName,omitempty"`
	Invitees          []NodeInstVO `json:"invitees"`
}

// NodeInstVO mirrors Java ParticipantNodeInstVO.NodeInstVO.
type NodeInstVO struct {
	InviteeID   string `json:"inviteeId"`
	InviteeName string `json:"inviteeName,omitempty"`
	InstID      string `json:"instId,omitempty"`
	InstName    string `json:"instName,omitempty"`
}

// VoteConfig is the union of the Java AbstractVoteConfig subclasses; the
// fields used depend on the voteType. encoding/json matches keys
// case-insensitively, so both "projectId" and "projectID" bind ProjectID.
type VoteConfig struct {
	// NODE_ROUTE (NodeRouteVoteConfig)
	SrcNodeID   string `json:"srcNodeId,omitempty"`
	DesNodeID   string `json:"desNodeId,omitempty"`
	SrcNodeAddr string `json:"srcNodeAddr,omitempty"`
	DesNodeAddr string `json:"desNodeAddr,omitempty"`
	IsSingle    *bool  `json:"isSingle,omitempty"`
	// TEE_DOWNLOAD (TeeDownLoadVoteConfig)
	JobID        string `json:"jobID,omitempty"`
	TaskID       string `json:"taskID,omitempty"`
	ResourceType string `json:"resourceType,omitempty"`
	ResourceID   string `json:"resourceID,omitempty"`
	GraphID      string `json:"graphID,omitempty"`
	// PROJECT_* (ProjectCreateApprovalConfig / ProjectArchiveConfig) + TEE projectID
	ProjectID              string                  `json:"projectId,omitempty"`
	Participants           []string                `json:"participants,omitempty"`
	ParticipantNodeInstVOS []ParticipantNodeInstVO `json:"participantNodeInstVOS,omitempty"`
	// PROJECT_NODE_ADD (Go extension): the node to add to the project.
	InviteNodeID string `json:"inviteNodeId,omitempty"`
}

// DbSyncRequest mirrors Java DbSyncRequest {syncDataType, projectNodesInfo}.
type DbSyncRequest struct {
	SyncDataType     string      `json:"syncDataType"`
	ProjectNodesInfo interface{} `json:"projectNodesInfo"`
}

// --- view objects (Java camelCase contract) ---

// PartyVoteStatus mirrors Java PartyVoteStatus / ProjectArchivePartyVoteStatus.
type PartyVoteStatus struct {
	ParticipantID           string `json:"participantID"`
	ParticipantName         string `json:"participantName"`
	Action                  string `json:"action"`
	Reason                  string `json:"reason"`
	ProjectCreateVoteAction string `json:"projectCreateVoteAction,omitempty"`
}

// MessageVO mirrors Java MessageVO.
type MessageVO struct {
	Type                  string      `json:"type"`
	Status                string      `json:"status"`
	InitiatingTypeMessage interface{} `json:"initiatingTypeMessage"`
	VoteTypeMessage       interface{} `json:"voteTypeMessage"`
	MessageName           string      `json:"messageName"`
	CreateTime            string      `json:"createTime"`
	VoteID                string      `json:"voteID"`
}

// MessageListVO mirrors Java MessageListVO (AbstractPageResponse fields included).
type MessageListVO struct {
	Messages  []MessageVO `json:"messages"`
	PageSize  int         `json:"pageSize"`
	PageNum   int         `json:"pageNum"`
	Total     int64       `json:"total"`
	TotalPage int64       `json:"totalPage"`
}

// InitiatorMessage mirrors Java InitiatorMessage.
type InitiatorMessage struct {
	PartyVoteStatuses []PartyVoteStatus `json:"partyVoteStatuses"`
}

// ReceiverMessage mirrors Java ReceiverMessage.
type ReceiverMessage struct {
	Reason            string `json:"reason"`
	ParticipantID     string `json:"participantID"`
	InitiatorNodeID   string `json:"initiatorNodeID"`
	InitiatorNodeName string `json:"initiatorNodeName"`
}

// ProjectApprovalCustomizedMessage mirrors the Java class of the same name.
type ProjectApprovalCustomizedMessage struct {
	ProjectID   string `json:"projectId"`
	ComputeFunc string `json:"computeFunc"`
	ComputeMode string `json:"computeMode"`
}

// TeeListCustomizedMessage mirrors the Java class of the same name.
type TeeListCustomizedMessage struct {
	ProjectID string `json:"projectID"`
}

// MessageDetailBase mirrors the Java MessageDetailVO base fields.
type MessageDetailBase struct {
	MessageName string `json:"messageName"`
	Type        string `json:"type"`
	Status      string `json:"status"`
}

// NodeRouteMessageDetail mirrors Java NodeRoteMessageDetail.
type NodeRouteMessageDetail struct {
	MessageDetailBase
	NodeID            string `json:"nodeID"`
	NodeName          string `json:"nodeName"`
	InitiatorNodeID   string `json:"initiatorNodeID"`
	InitiatorNodeName string `json:"initiatorNodeName"`
	URL               string `json:"url"`
	IsSingle          bool   `json:"isSingle"`
	Reason            string `json:"reason"`
}

// ProjectSimpleVO mirrors Java ProjectSimpleVO.
type ProjectSimpleVO struct {
	ProjectName string `json:"projectName"`
	GmtCreated  string `json:"gmtCreated"`
	ComputeMode string `json:"computeMode"`
	Description string `json:"description"`
}

// TableColumnVO mirrors Java TableColumnVO.
type TableColumnVO struct {
	ColName    string `json:"colName"`
	ColType    string `json:"colType"`
	ColComment string `json:"colComment"`
}

// TeeDownloadMessageDetail mirrors Java TeeDownLoadMessageDetail.
type TeeDownloadMessageDetail struct {
	MessageDetailBase
	PartyVoteStatuses []PartyVoteStatus `json:"partyVoteStatuses"`
	GraphName         string            `json:"graphName"`
	TableColumns      []TableColumnVO   `json:"tableColumns"`
	Project           ProjectSimpleVO   `json:"project"`
	// GraphDetailVO is the job graph (Java projectService.getProjectJob(...).getGraph()).
	GraphDetailVO *service.GraphDetailVO `json:"graphDetailVO"`
	TaskID        string                 `json:"taskID"`
	ResourceID    string                 `json:"resourceID"`
	Parties       []string               `json:"parties"`
}

// ProjectCreateMessageDetail mirrors the Java class of the same name (also
// used for PROJECT_ARCHIVE).
type ProjectCreateMessageDetail struct {
	MessageDetailBase
	InitiatorID            string                  `json:"initiatorId"`
	InitiatorName          string                  `json:"initiatorName"`
	ProjectName            string                  `json:"projectName"`
	PartyVoteStatuses      []PartyVoteStatus       `json:"partyVoteStatuses"`
	ComputeMode            string                  `json:"computeMode"`
	ComputeFunc            string                  `json:"computeFunc"`
	ProjectDesc            string                  `json:"projectDesc"`
	GmtCreated             string                  `json:"gmtCreated"`
	ParticipantNodeInstVOS []ParticipantNodeInstVO `json:"participantNodeInstVOS"`
}

// PullStatusVO mirrors Java PullStatusVO.
type PullStatusVO struct {
	ResourceID   string        `json:"resourceID"`
	ResourceType string        `json:"resourceType"`
	TaskID       string        `json:"taskID"`
	GraphID      string        `json:"graphID"`
	JobID        string        `json:"jobID"`
	Parties      []Participant `json:"parties"`
}

// Participant mirrors Java approval.Participant.
type Participant struct {
	NodeID    string     `json:"nodeID"`
	Status    string     `json:"status"`
	NodeName  string     `json:"nodeName"`
	VoteInfos []VoteInfo `json:"voteInfos,omitempty"`
	// Go extension: the TEE pull record ("{nodeId}-{resourceId}") written by
	// the TEE_DOWNLOAD approved callback (RUNNING / SUCCESS / FAILED).
	PullDatatableID string `json:"pullDatatableId,omitempty"`
	PullStatus      string `json:"pullStatus,omitempty"`
	PullErrMsg      string `json:"pullErrMsg,omitempty"`
}

// VoteInfo mirrors Java Participant.VoteInfo.
type VoteInfo struct {
	VoteID string `json:"voteID"`
	NodeID string `json:"nodeID"`
	Action string `json:"action"`
	Reason string `json:"reason"`
}

// --- helpers ---

func toJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func parseStringList(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var out []string
	if strings.HasPrefix(s, "[") {
		if err := json.Unmarshal([]byte(s), &out); err == nil {
			return out
		}
	}
	// legacy comma-separated storage
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parsePartyVoteInfos(s string) []PartyVoteInfo {
	var out []PartyVoteInfo
	if strings.TrimSpace(s) != "" {
		_ = json.Unmarshal([]byte(s), &out)
	}
	return out
}

// upsertPartyVoteInfo replaces the entry of the same partyId (Java Set remove+add).
func upsertPartyVoteInfo(infos []PartyVoteInfo, info PartyVoteInfo) []PartyVoteInfo {
	out := make([]PartyVoteInfo, 0, len(infos)+1)
	for _, i := range infos {
		if i.PartyID != info.PartyID {
			out = append(out, i)
		}
	}
	return append(out, info)
}

func parseParticipantNodeInfo(s string) []ParticipantNodeInstVO {
	var out []ParticipantNodeInstVO
	if strings.TrimSpace(s) != "" {
		_ = json.Unmarshal([]byte(s), &out)
	}
	return out
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func without(list []string, v string) []string {
	out := make([]string, 0, len(list))
	for _, x := range list {
		if x != v {
			out = append(out, x)
		}
	}
	return out
}

func rfc3339(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}
