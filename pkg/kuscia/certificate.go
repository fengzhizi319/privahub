package kuscia

import "context"

// --- Certificate Types ---

// GenerateKeyCertsRequest represents a certificate generation request.
type GenerateKeyCertsRequest struct {
	CommonName       string `json:"common_name"`
	Country          string `json:"country,omitempty"`
	Organization     string `json:"organization,omitempty"`
	OrganizationUnit string `json:"organization_unit,omitempty"`
	Locality         string `json:"locality,omitempty"`
	Province         string `json:"province,omitempty"`
	StreetAddress    string `json:"street_address,omitempty"`
	DurationSec      int64  `json:"duration_sec,omitempty"`
	KeyType          string `json:"key_type,omitempty"` // PKCS#1 or PKCS#8
}

// GenerateKeyCertsResponse represents a certificate generation response.
type GenerateKeyCertsResponse struct {
	Status    Status   `json:"status"`
	Key       string   `json:"key"`        // Base64 encoded private key
	CertChain []string `json:"cert_chain"` // Base64 encoded cert chain
}

// --- Certificate Client Methods ---

// GenerateKeyCerts generates a key/certificate pair via Kuscia.
func (c *Client) GenerateKeyCerts(ctx context.Context, req *GenerateKeyCertsRequest) (*GenerateKeyCertsResponse, error) {
	var resp GenerateKeyCertsResponse
	if err := c.doRequest(ctx, "/api/v1/certificate/generate", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
