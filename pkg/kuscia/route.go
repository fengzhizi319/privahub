package kuscia

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// --- DomainRoute Types (kusciaapi/domain_route.proto) ---

// EndpointPort is one port of a route endpoint.
type EndpointPort struct {
	Name       string `json:"name"`
	Port       int32  `json:"port"`
	Protocol   string `json:"protocol"`
	IsTLS      bool   `json:"isTLS"`
	PathPrefix string `json:"path_prefix,omitempty"`
}

// RouteEndpoint is the destination endpoint of a DomainRoute.
type RouteEndpoint struct {
	Host  string         `json:"host"`
	Ports []EndpointPort `json:"ports"`
}

// TokenConfig configures token-based route authentication.
type TokenConfig struct {
	DestinationPublicKey string    `json:"destination_public_key,omitempty"`
	RollingUpdatePeriod  FlexInt64 `json:"rolling_update_period,omitempty"`
	SourcePublicKey      string    `json:"source_public_key,omitempty"`
	TokenGenMethod       string    `json:"token_gen_method,omitempty"`
}

// MTLSConfig configures mTLS route authentication.
type MTLSConfig struct {
	TLSCA                  string `json:"tls_ca,omitempty"`
	SourceClientPrivateKey string `json:"source_client_private_key,omitempty"`
	SourceClientCert       string `json:"source_client_cert,omitempty"`
}

// Transit configures a transit route.
type Transit struct {
	TransitMethod string `json:"transit_method,omitempty"`
	Domain        *struct {
		DomainID string `json:"domain_id"`
	} `json:"domain,omitempty"`
}

// BodyEncryption configures payload encryption.
type BodyEncryption struct {
	Algorithm string `json:"algorithm,omitempty"`
}

// CreateDomainRouteRequest represents a Kuscia CreateDomainRoute API request.
type CreateDomainRouteRequest struct {
	AuthenticationType string          `json:"authentication_type,omitempty"` // Token / MTLS
	Destination        string          `json:"destination"`
	Endpoint           *RouteEndpoint  `json:"endpoint,omitempty"`
	Source             string          `json:"source"`
	TokenConfig        *TokenConfig    `json:"token_config,omitempty"`
	MTLSConfig         *MTLSConfig     `json:"mtls_config,omitempty"`
	Transit            *Transit        `json:"transit,omitempty"`
	BodyEncryption     *BodyEncryption `json:"body_encryption,omitempty"`
}

// CreateDomainRouteResponse represents a Kuscia CreateDomainRoute API response.
type CreateDomainRouteResponse struct {
	Status Status `json:"status"`
	Data   struct {
		Name string `json:"name"`
	} `json:"data"`
}

// QueryDomainRouteRequest represents a Kuscia QueryDomainRoute API request.
type QueryDomainRouteRequest struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
}

// RouteStatus is the status of a DomainRoute ("Succeeded" / "Failed").
type RouteStatus struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

// Route status values.
const (
	RouteStatusSucceeded = "Succeeded"
	RouteStatusFailed    = "Failed"
)

// DomainRouteInfo is QueryDomainRouteResponseData.
type DomainRouteInfo struct {
	Name               string          `json:"name"`
	AuthenticationType string          `json:"authentication_type"`
	Destination        string          `json:"destination"`
	Endpoint           *RouteEndpoint  `json:"endpoint,omitempty"`
	Source             string          `json:"source"`
	TokenConfig        *TokenConfig    `json:"token_config,omitempty"`
	MTLSConfig         *MTLSConfig     `json:"mtls_config,omitempty"`
	Status             RouteStatus     `json:"status"`
	Transit            *Transit        `json:"transit,omitempty"`
	BodyEncryption     *BodyEncryption `json:"body_encryption,omitempty"`
}

// QueryDomainRouteResponse represents a Kuscia QueryDomainRoute API response.
type QueryDomainRouteResponse struct {
	Status Status          `json:"status"`
	Data   DomainRouteInfo `json:"data"`
}

// DeleteDomainRouteRequest represents a Kuscia DeleteDomainRoute API request.
type DeleteDomainRouteRequest struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
}

// DeleteDomainRouteResponse represents a Kuscia DeleteDomainRoute API response.
type DeleteDomainRouteResponse struct {
	Status Status `json:"status"`
}

// BatchQueryDomainRouteStatusRequest represents a batch route status query.
type BatchQueryDomainRouteStatusRequest struct {
	RouteKeys []QueryDomainRouteRequest `json:"route_keys"`
}

// DomainRouteStatusEntry represents a single route status in batch response.
type DomainRouteStatusEntry struct {
	Name        string      `json:"name"`
	Source      string      `json:"source"`
	Destination string      `json:"destination"`
	Status      RouteStatus `json:"status"`
}

// BatchQueryDomainRouteStatusResponse represents a batch route status response.
type BatchQueryDomainRouteStatusResponse struct {
	Status Status `json:"status"`
	Data   struct {
		Routes []DomainRouteStatusEntry `json:"routes"`
	} `json:"data"`
}

// --- Helpers ---

// BuildRouteEndpoint parses a node netAddress into a RouteEndpoint, following
// Java NodeRouteManager.buildRouteEndpoint: https → {name:https, protocol:HTTPS,
// isTLS:true}, otherwise {name:http, protocol:HTTP}. A bare "host:port" is
// treated as http. Missing port defaults to 80 (http) / 443 (https).
func BuildRouteEndpoint(netAddress string) (*RouteEndpoint, error) {
	addr := strings.TrimSpace(netAddress)
	if addr == "" {
		return nil, fmt.Errorf("kuscia: empty net address")
	}
	if !strings.Contains(addr, "://") {
		addr = "http://" + addr
	}
	u, err := url.Parse(addr)
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("kuscia: address format error: %q", netAddress)
	}
	host := u.Hostname()
	if host == "" {
		return nil, fmt.Errorf("kuscia: address format error: %q", netAddress)
	}
	isTLS := strings.EqualFold(u.Scheme, "https")
	port := 80
	if isTLS {
		port = 443
	}
	if p := u.Port(); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n <= 0 || n > 65535 {
			return nil, fmt.Errorf("kuscia: invalid port in %q", netAddress)
		}
		port = n
	}
	ep := EndpointPort{Name: "http", Port: int32(port), Protocol: "HTTP"}
	if isTLS {
		ep = EndpointPort{Name: "https", Port: int32(port), Protocol: "HTTPS", IsTLS: true}
	}
	return &RouteEndpoint{Host: host, Ports: []EndpointPort{ep}}, nil
}

// NewTokenRouteRequest builds a Java-compatible route creation request:
// authentication_type=Token, token_config.token_gen_method=RSA-GEN, endpoint from dstNetAddress.
func NewTokenRouteRequest(src, dst, dstNetAddress string) (*CreateDomainRouteRequest, error) {
	ep, err := BuildRouteEndpoint(dstNetAddress)
	if err != nil {
		return nil, err
	}
	return &CreateDomainRouteRequest{
		AuthenticationType: AuthTypeToken,
		Source:             src,
		Destination:        dst,
		Endpoint:           ep,
		TokenConfig:        &TokenConfig{TokenGenMethod: TokenGenRSAGen},
	}, nil
}

// --- DomainRoute Service Methods ---

// CreateDomainRoute creates a route between two domains.
func (c *Client) CreateDomainRoute(ctx context.Context, req *CreateDomainRouteRequest) error {
	var resp CreateDomainRouteResponse
	return c.doRequest(ctx, "/api/v1/route/create", req, &resp)
}

// QueryDomainRoute queries a route (including its status object).
func (c *Client) QueryDomainRoute(ctx context.Context, source, destination string) (*QueryDomainRouteResponse, error) {
	var resp QueryDomainRouteResponse
	req := &QueryDomainRouteRequest{Source: source, Destination: destination}
	if err := c.doRequest(ctx, "/api/v1/route/query", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// DomainRouteExists reports whether src->dst route exists in Kuscia.
func (c *Client) DomainRouteExists(ctx context.Context, source, destination string) (bool, error) {
	_, err := c.QueryDomainRoute(ctx, source, destination)
	if err == nil {
		return true, nil
	}
	if ErrorCode(err) != 0 {
		return false, nil
	}
	return false, err
}

// DeleteDomainRoute removes a route between two domains. A Kuscia 11404
// (route not exists) is treated as success, as in Java NodeRouteManager.
func (c *Client) DeleteDomainRoute(ctx context.Context, source, destination string) error {
	var resp DeleteDomainRouteResponse
	req := &DeleteDomainRouteRequest{Source: source, Destination: destination}
	if err := c.doRequest(ctx, "/api/v1/route/delete", req, &resp); err != nil {
		if ErrorCode(err) == ErrCodeDomainRouteNotExists {
			return nil
		}
		return err
	}
	return nil
}

// BatchQueryDomainRouteStatus queries status for multiple routes.
func (c *Client) BatchQueryDomainRouteStatus(ctx context.Context, routes []QueryDomainRouteRequest) ([]DomainRouteStatusEntry, error) {
	var resp BatchQueryDomainRouteStatusResponse
	req := &BatchQueryDomainRouteStatusRequest{RouteKeys: routes}
	if err := c.doRequest(ctx, "/api/v1/route/status/batchQuery", req, &resp); err != nil {
		return nil, err
	}
	return resp.Data.Routes, nil
}
