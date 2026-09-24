package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
)

// Shared helpers of the graph / project / job services (graph migration work).

// PadError is a business error carrying a Java SecretPad error code and its
// message arguments (Java SecretpadException.of(code, args...)).
type PadError struct {
	Code *errcode.ErrorCode
	Args []string
	// Base is an optional sentinel for errors.Is compatibility.
	Base error
}

func (e *PadError) Error() string {
	if len(e.Args) > 0 {
		return fmt.Sprintf("[%d] %s: %s", e.Code.Code, e.Code.Message, strings.Join(e.Args, ","))
	}
	return fmt.Sprintf("[%d] %s", e.Code.Code, e.Code.Message)
}

// Unwrap exposes the sentinel.
func (e *PadError) Unwrap() error { return e.Base }

func padErr(code *errcode.ErrorCode, args ...string) error {
	return &PadError{Code: code, Args: args}
}

func padErrBase(base error, code *errcode.ErrorCode, args ...string) error {
	return &PadError{Code: code, Args: args, Base: base}
}

// AsPadError extracts a PadError.
func AsPadError(err error) (*PadError, bool) {
	var pe *PadError
	if errors.As(err, &pe) {
		return pe, true
	}
	return nil, false
}

// Operator is the current user identity (UserContext) passed to services.
type Operator struct {
	UserID    string
	OwnerID   string
	OwnerType string // CENTER | EDGE | P2P ...
}

type operatorKey struct{}

// WithOperator stores the operator in ctx.
func WithOperator(ctx context.Context, op Operator) context.Context {
	return context.WithValue(ctx, operatorKey{}, op)
}

// OperatorFrom returns the operator stored in ctx (zero value when absent).
func OperatorFrom(ctx context.Context) Operator {
	if op, ok := ctx.Value(operatorKey{}).(Operator); ok {
		return op
	}
	return Operator{}
}

// isEdgeOperator reports an EDGE user.
func (o Operator) isEdge() bool { return strings.EqualFold(o.OwnerType, "EDGE") }

// GraphKusciaAPI is the subset of KusciaAPI used by the graph / project / job
// services. *kuscia.Client implements it; tests use a fake or httptest.
type GraphKusciaAPI interface {
	CreateJob(ctx context.Context, req *kuscia.CreateJobRequest) (*kuscia.CreateJobResponse, error)
	StopJob(ctx context.Context, jobID string) error
	QueryJobDetail(ctx context.Context, jobID string) (*kuscia.QueryJobDetailResponse, error)
	QueryDomainData(ctx context.Context, domainID, domainDataID string) (*kuscia.QueryDomainDataResponse, error)
	CreateDomainDataGrantWithID(ctx context.Context, req *kuscia.DomainDataGrantWithIDRequest) error
	QueryDomainDataGrantByID(ctx context.Context, domainID, grantID string) error
	ListDomainDataSource(ctx context.Context, domainID string) ([]kuscia.DomainDataSource, error)
	QueryDomain(ctx context.Context, domainID string) (*kuscia.QueryDomainResponse, error)
	QueryDomainRoute(ctx context.Context, source, destination string) (*kuscia.QueryDomainRouteResponse, error)
}

var _ GraphKusciaAPI = (*kuscia.Client)(nil)

// toGraphKuscia converts a possibly-nil client without the typed-nil trap.
func toGraphKuscia(c *kuscia.Client) GraphKusciaAPI {
	if c == nil {
		return nil
	}
	return c
}

// kusciaDatasourceTypeToPlatform mirrors DataSourceTypeEnum.kuscia2platform.
func kusciaDatasourceTypeToPlatform(t string) string {
	switch strings.ToLower(t) {
	case "localfs", "local":
		return "LOCAL"
	case "oss":
		return "OSS"
	case "mysql":
		return "MYSQL"
	case "odps":
		return "ODPS"
	case "http":
		return "HTTP"
	case "":
		return ""
	}
	return strings.ToUpper(t)
}

// grantDomainData mirrors checkOrCreateDomainDataGrant: query first, create
// with grantId "{domainDataId}-{grantNode}" when absent.
func grantDomainData(ctx context.Context, k GraphKusciaAPI, ownerNode, grantNode, domainDataID string) error {
	if k == nil || ownerNode == grantNode {
		return nil
	}
	grantID := domainDataID + "-" + grantNode
	if err := k.QueryDomainDataGrantByID(ctx, ownerNode, grantID); err == nil {
		return nil
	}
	err := k.CreateDomainDataGrantWithID(ctx, &kuscia.DomainDataGrantWithIDRequest{
		DomainID:          ownerNode,
		DomainDataGrantID: grantID,
		DomainDataID:      domainDataID,
		GrantDomain:       grantNode,
	})
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "exist") {
		return nil
	}
	return err
}

var cstZone = time.FixedZone("CST", 8*3600)

// rfc3339 formats like Java DateTimes.toRfc3339 (GMT+8); zero time -> "".
func rfc3339(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.In(cstZone).Format("2006-01-02T15:04:05+08:00")
}
