package kuscia

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// NodeEndpoint describes how to reach one node's KusciaAPI. It is the Go
// counterpart of Java KusciaGrpcConfig (domainId/host/port/protocol/mode/
// certFile/keyFile/caFile/token).
type NodeEndpoint struct {
	DomainID string `json:"domain_id"`
	// Mode: master / lite / autonomy / p2p (informational).
	Mode string `json:"mode,omitempty"`
	ClientConfig
	// Persist marks runtime-registered endpoints (e.g. inst node/register)
	// that must survive restarts.
	Persist bool `json:"-"`
}

// Validate mirrors Java KusciaGrpcConfig.validateAndProcess: host may carry
// the port ("host:port"); tls/mtls require cert/key/token.
func (e *NodeEndpoint) Validate() error {
	if strings.TrimSpace(e.DomainID) == "" {
		return errors.New("kuscia: endpoint domain_id is required")
	}
	if strings.TrimSpace(e.Host) == "" {
		return errors.New("kuscia: endpoint host is required")
	}
	if i := strings.LastIndex(e.Host, ":"); i > 0 && !strings.Contains(e.Host, "]") && !strings.Contains(e.Host, "://") {
		var port int
		if _, err := fmt.Sscanf(e.Host[i+1:], "%d", &port); err == nil {
			e.Port = port
			e.Host = e.Host[:i]
		}
	}
	if e.Port < 0 || e.Port > 65535 {
		return fmt.Errorf("kuscia: invalid port %d", e.Port)
	}
	switch strings.ToLower(e.Protocol) {
	case "", "notls":
		e.Protocol = "notls"
	case "tls", "mtls":
		e.Protocol = strings.ToLower(e.Protocol)
		if e.CertFile == "" || e.KeyFile == "" || e.Token == "" {
			return errors.New("kuscia: cert_file, key_file and token are required when protocol is tls/mtls")
		}
	default:
		return fmt.Errorf("kuscia: invalid protocol %q (tls/mtls/notls)", e.Protocol)
	}
	return nil
}

// ClientPool is a registry of KusciaAPI clients keyed by domain (node) id.
// It corresponds to Java DynamicKusciaChannelProvider: the default client
// targets the local (master/autonomy) KusciaAPI; additional nodes can be
// registered at runtime (inst node/register) and are persisted to a JSON file.
type ClientPool struct {
	mu          sync.RWMutex
	def         *Client
	clients     map[string]*Client
	endpoints   map[string]NodeEndpoint
	persistPath string
	listeners   []func(ep NodeEndpoint, registered bool)
}

// NewClientPool creates a pool with the given default client. persistPath
// (optional) is the JSON file used to store runtime-registered endpoints.
func NewClientPool(def *Client, persistPath string) *ClientPool {
	return &ClientPool{
		def:         def,
		clients:     make(map[string]*Client),
		endpoints:   make(map[string]NodeEndpoint),
		persistPath: persistPath,
	}
}

// Default returns the default (local) client. May be nil.
func (p *ClientPool) Default() *Client {
	if p == nil {
		return nil
	}
	return p.def
}

// Get returns the client registered for domainID, falling back to the default.
func (p *ClientPool) Get(domainID string) *Client {
	if p == nil {
		return nil
	}
	if c, ok := p.Lookup(domainID); ok {
		return c
	}
	return p.def
}

// Lookup returns the client registered for domainID only.
func (p *ClientPool) Lookup(domainID string) (*Client, bool) {
	if p == nil || domainID == "" {
		return nil, false
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	c, ok := p.clients[domainID]
	return c, ok
}

// IsRegistered reports whether a dedicated channel exists for domainID
// (Java KusciaGrpcClientAdapter.isDomainRegistered).
func (p *ClientPool) IsRegistered(domainID string) bool {
	_, ok := p.Lookup(domainID)
	return ok
}

// OnChange registers a listener invoked after Register (true) / Unregister (false).
// Corresponds to Java RegisterKusciaEvent / UnRegisterKusciaEvent.
func (p *ClientPool) OnChange(fn func(ep NodeEndpoint, registered bool)) {
	p.mu.Lock()
	p.listeners = append(p.listeners, fn)
	p.mu.Unlock()
}

// Register validates the endpoint, builds a client and stores it, replacing
// any previous registration for the same domain.
func (p *ClientPool) Register(ep NodeEndpoint) error {
	if err := ep.Validate(); err != nil {
		return err
	}
	c, err := NewClientE(&ep.ClientConfig)
	if err != nil {
		return err
	}
	p.mu.Lock()
	p.clients[ep.DomainID] = c
	persisted := ep.Persist
	if old, ok := p.endpoints[ep.DomainID]; ok && old.Persist {
		persisted = true
	}
	ep.Persist = persisted
	p.endpoints[ep.DomainID] = ep
	listeners := append([]func(NodeEndpoint, bool){}, p.listeners...)
	var perr error
	if ep.Persist {
		perr = p.saveLocked()
	}
	p.mu.Unlock()
	for _, fn := range listeners {
		fn(ep, true)
	}
	return perr
}

// Unregister removes the client for domainID.
func (p *ClientPool) Unregister(domainID string) error {
	p.mu.Lock()
	ep, ok := p.endpoints[domainID]
	delete(p.clients, domainID)
	delete(p.endpoints, domainID)
	listeners := append([]func(NodeEndpoint, bool){}, p.listeners...)
	var perr error
	if ok && ep.Persist {
		perr = p.saveLocked()
	}
	p.mu.Unlock()
	if ok {
		for _, fn := range listeners {
			fn(ep, false)
		}
	}
	return perr
}

// Endpoints returns all registered endpoints sorted by domain id.
func (p *ClientPool) Endpoints() []NodeEndpoint {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]NodeEndpoint, 0, len(p.endpoints))
	for _, ep := range p.endpoints {
		out = append(out, ep)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DomainID < out[j].DomainID })
	return out
}

// Load reads persisted endpoints from persistPath and registers them.
// A missing file is not an error. Invalid entries are skipped and reported.
func (p *ClientPool) Load() error {
	if p.persistPath == "" {
		return nil
	}
	data, err := os.ReadFile(p.persistPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("kuscia: read pool file: %w", err)
	}
	var eps []NodeEndpoint
	if err := json.Unmarshal(data, &eps); err != nil {
		return fmt.Errorf("kuscia: parse pool file: %w", err)
	}
	var errs []error
	for _, ep := range eps {
		ep.Persist = true
		if err := p.Register(ep); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", ep.DomainID, err))
		}
	}
	return errors.Join(errs...)
}

func (p *ClientPool) saveLocked() error {
	if p.persistPath == "" {
		return nil
	}
	eps := make([]NodeEndpoint, 0)
	for _, ep := range p.endpoints {
		if ep.Persist {
			eps = append(eps, ep)
		}
	}
	sort.Slice(eps, func(i, j int) bool { return eps[i].DomainID < eps[j].DomainID })
	data, err := json.MarshalIndent(eps, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p.persistPath), 0o700); err != nil {
		return fmt.Errorf("kuscia: create pool dir: %w", err)
	}
	tmp := p.persistPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("kuscia: write pool file: %w", err)
	}
	return os.Rename(tmp, p.persistPath)
}
