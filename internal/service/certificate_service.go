package service

import (
	"context"
	"errors"

	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
)

// CertificateService wraps KusciaAPI CertificateService.GenerateKeyCerts. It is
// the Go counterpart of Java CertificateServiceImpl and is meant to be used by
// the vote (approval) system to obtain a key/cert pair for request signing and
// verification.
type CertificateService struct {
	nodes *NodeService
}

// NewCertificateService creates a CertificateService using the node client pool.
func NewCertificateService(nodes *NodeService) *CertificateService {
	return &CertificateService{nodes: nodes}
}

// KeyCerts is a generated private key and certificate chain (base64 PEM as returned by Kuscia).
type KeyCerts struct {
	Key       string   `json:"key"`
	CertChain []string `json:"certChain"`
}

// GenerateKeyCerts asks the KusciaAPI of nodeID (default channel when the node
// has no dedicated channel) to issue a key/cert pair for commonName.
func (s *CertificateService) GenerateKeyCerts(ctx context.Context, nodeID, commonName string, durationSec int64) (*KeyCerts, error) {
	if s == nil || s.nodes == nil {
		return nil, errors.New("certificate service not configured")
	}
	c := s.nodes.kc(nodeID)
	if c == nil {
		return nil, bizErr(errcode.KusciaRpcError, "kuscia client not configured")
	}
	if commonName == "" {
		commonName = nodeID
	}
	resp, err := c.GenerateKeyCerts(ctx, &kuscia.GenerateKeyCertsRequest{
		CommonName:  commonName,
		DurationSec: durationSec,
		KeyType:     "PKCS#8",
	})
	if err != nil {
		return nil, bizWrap(errcode.KusciaRpcError, err, "")
	}
	return &KeyCerts{Key: resp.Key, CertChain: resp.CertChain}, nil
}
