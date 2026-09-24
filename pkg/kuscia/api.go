package kuscia

import "context"

// DomainAPI is the Domain/DomainRoute subset of KusciaAPI used by the node,
// route and inst services. *Client (HTTP) implements it; a gRPC-backed
// implementation can be swapped in without touching callers that depend on
// this interface.
type DomainAPI interface {
	CreateDomain(ctx context.Context, req *CreateDomainRequest) error
	UpdateDomain(ctx context.Context, req *UpdateDomainRequest) error
	QueryDomain(ctx context.Context, domainID string) (*QueryDomainResponse, error)
	DomainExists(ctx context.Context, domainID string) (bool, error)
	BatchQueryDomain(ctx context.Context, domainIDs []string) ([]BatchDomain, error)
	DeleteDomain(ctx context.Context, domainID string) error

	CreateDomainRoute(ctx context.Context, req *CreateDomainRouteRequest) error
	QueryDomainRoute(ctx context.Context, source, destination string) (*QueryDomainRouteResponse, error)
	DomainRouteExists(ctx context.Context, source, destination string) (bool, error)
	DeleteDomainRoute(ctx context.Context, source, destination string) error
	BatchQueryDomainRouteStatus(ctx context.Context, routes []QueryDomainRouteRequest) ([]DomainRouteStatusEntry, error)
}

// JobAPI is the Job subset of KusciaAPI.
type JobAPI interface {
	CreateJob(ctx context.Context, req *CreateJobRequest) (*CreateJobResponse, error)
	QueryJob(ctx context.Context, jobID string) (*QueryJobResponse, error)
	StopJob(ctx context.Context, jobID string) error
	RestartJob(ctx context.Context, jobID string) error
	DeleteJob(ctx context.Context, jobID string) error
	BatchQueryJobStatus(ctx context.Context, jobIDs []string) ([]JobStatusEntry, error)
}

var (
	_ DomainAPI = (*Client)(nil)
	_ JobAPI    = (*Client)(nil)
)
