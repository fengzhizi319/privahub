package votehandler

import (
	"strings"
	"unicode/utf8"
)

// Request validation mirroring the Java bean-validation annotations of the
// approval / message controllers. Every function returns "" when the request
// is valid, otherwise the message Java's SecretpadExceptionHandler would put
// into the SystemErrorCode.VALIDATION_ERROR response.

// ApiVoteTypes are the vote types accepted by approval/create and
// message/detail (Java @OneOfType(types = {"TEE_DOWNLOAD", "NODE_ROUTE",
// "PROJECT_CREATE", "PROJECT_ARCHIVE"})). PROJECT_NODE_ADD is internal only.
var ApiVoteTypes = []string{VoteTypeTeeDownload, VoteTypeNodeRoute, VoteTypeProjectCreate, VoteTypeProjectArchive}

// PullStatusResourceTypes is Java PullStatusRequest @OneOfType(types = {"model", "rule", "table"}).
var PullStatusResourceTypes = []string{"model", "rule", "table"}

// MaxReplyReasonLen is Java VoteReplyRequest @Size(max = 50) on reason.
const MaxReplyReasonLen = 50

// oneOf mirrors Java ApprovalTypeValidator: null/empty is invalid, match is exact.
func oneOf(v string, types []string) bool {
	if v == "" {
		return false
	}
	for _, t := range types {
		if v == t {
			return true
		}
	}
	return false
}

func blank(v string) bool { return strings.TrimSpace(v) == "" }

// ValidateVoteType checks a vote type against ApiVoteTypes.
func ValidateVoteType(voteType string) string {
	if blank(voteType) {
		return "voteType must not be blank"
	}
	if !oneOf(voteType, ApiVoteTypes) {
		return "voteType must be one of " + strings.Join(ApiVoteTypes, ", ")
	}
	return ""
}

// ValidateReply mirrors Java VoteReplyRequest.
func ValidateReply(action, reason, voteID, participantID string) string {
	switch {
	case blank(voteID):
		return "voteId must not be blank"
	case blank(participantID):
		return "voteParticipantId must not be blank"
	case !oneOf(action, []string{ActionApproved, ActionRejected}):
		return "action must be APPROVED or REJECTED"
	case utf8.RuneCountInString(reason) > MaxReplyReasonLen:
		return "reason must less then 50 characters"
	}
	return ""
}

// ValidatePullStatus mirrors Java PullStatusRequest except taskID: the result
// list rows carry no task id, so an empty taskID is resolved from the result
// (Service.PullStatus) instead of being rejected.
func ValidatePullStatus(projectID, jobID, resourceID, resourceType string) string {
	switch {
	case blank(projectID):
		return "projectID must not be blank"
	case blank(jobID):
		return "jobID must not be blank"
	case blank(resourceID):
		return "resourceID must not be blank"
	case !oneOf(resourceType, PullStatusResourceTypes):
		return "resourceType must be one of " + strings.Join(PullStatusResourceTypes, ", ")
	}
	return ""
}

// ValidateVoteConfig mirrors the @Valid per-type config classes selected by
// voteType (NodeRouteVoteConfig, TeeDownLoadVoteConfig,
// ProjectCreateApprovalConfig, ProjectArchiveConfig).
func ValidateVoteConfig(voteType string, cfg *VoteConfig) string {
	if cfg == nil {
		return "voteConfig must not be null"
	}
	type field struct{ name, v string }
	var fields []field
	switch voteType {
	case VoteTypeNodeRoute:
		fields = []field{{"srcNodeId", cfg.SrcNodeID}, {"desNodeId", cfg.DesNodeID},
			{"srcNodeAddr", cfg.SrcNodeAddr}, {"desNodeAddr", cfg.DesNodeAddr}}
	case VoteTypeTeeDownload:
		fields = []field{{"taskID", cfg.TaskID}, {"resourceID", cfg.ResourceID}, {"jobID", cfg.JobID},
			{"resourceType", cfg.ResourceType}, {"projectID", cfg.ProjectID}, {"graphID", cfg.GraphID}}
	case VoteTypeProjectCreate, VoteTypeProjectArchive:
		fields = []field{{"projectId", cfg.ProjectID}}
	}
	for _, f := range fields {
		if blank(f.v) {
			return "voteConfig." + f.name + " must not be blank"
		}
	}
	switch voteType {
	case VoteTypeNodeRoute:
		if !ipPrefixPattern.MatchString(cfg.SrcNodeAddr) || !ipPrefixPattern.MatchString(cfg.DesNodeAddr) {
			return "address needs to start with http or https"
		}
	case VoteTypeProjectCreate:
		if len(cfg.Participants) == 0 {
			return "voteConfig.participants must not be empty"
		}
	}
	return ""
}
