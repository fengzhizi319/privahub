package service

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base32"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fengzhizi319/privahub/internal/graphjob"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
)

// TEE graph execution (Java KusciaTrustedFlowJobConverter): a TEE project
// runs every selected component on the project's TEE node with the
// "tee-image" AppImage; the task body is signed by the platform (RS256 with
// config/certs/client.pem) and the TEE capsule manager verifies it.

const (
	// TeeAppImage is Java JobConstants.TEE_APP_IMAGE.
	TeeAppImage = "tee-image"
	// teeDmInputPrefix / teeDmOutputPrefix are JobConverter.DM_INPUT / DM_OUTPUT.
	teeDmInputPrefix  = "dm://input/?"
	teeDmOutputPrefix = "dm://output/?"
	teeSignAlgorithm  = "RS256"
	// TeeProjectMode is Java Constants.TEE_PROJECT_MODE (compared case-insensitively).
	TeeProjectMode = "tee"
)

// TeeGraphConfig configures TEE graph execution.
type TeeGraphConfig struct {
	// DomainID is the default TEE node (tee.domain-id, default "tee").
	DomainID string
	// CapsuleManager is tee.capsule-manager ('#' is replaced by the TEE node).
	CapsuleManager string
	// CertDir holds client.pem / client.crt / ca.crt used to sign TEE tasks
	// (Java ./config/certs).
	CertDir string
}

func (c TeeGraphConfig) withDefaults() TeeGraphConfig {
	if c.DomainID == "" {
		c.DomainID = "tee"
	}
	if c.CapsuleManager == "" {
		c.CapsuleManager = "capsule-manager.#.svc"
	}
	if c.CertDir == "" {
		c.CertDir = "./config/certs"
	}
	return c
}

// SetTeeConfig configures TEE graph execution (capsule manager, signing certs).
func (s *GraphService) SetTeeConfig(cfg TeeGraphConfig) *GraphService {
	s.extMu.Lock()
	s.tee = cfg.withDefaults()
	s.extMu.Unlock()
	return s
}

// SetTeeSigner overrides the TEE task signer (tests / HSM integrations).
func (s *GraphService) SetTeeSigner(signer TeeTaskSigner) *GraphService {
	s.extMu.Lock()
	s.signer = signer
	s.extMu.Unlock()
	return s
}

// WithKusciaPool sets the per-node KusciaAPI pool used for channel-aware
// queries (AUTONOMY route checks and output datasource lookups).
func (s *GraphService) WithKusciaPool(p *kuscia.ClientPool) *GraphService {
	s.extMu.Lock()
	s.pool = p
	s.extMu.Unlock()
	return s
}

// teeConfig returns the TEE settings; an unconfigured service (zero value)
// gets the defaults (withDefaults is idempotent).
func (s *GraphService) teeConfig() TeeGraphConfig {
	s.extMu.RLock()
	defer s.extMu.RUnlock()
	return s.tee.withDefaults()
}

func (s *GraphService) teeSigner() TeeTaskSigner {
	s.extMu.RLock()
	defer s.extMu.RUnlock()
	if s.signer != nil {
		return s.signer
	}
	return &FileTeeTaskSigner{CertDir: s.tee.withDefaults().CertDir}
}

// kusciaFor returns the KusciaAPI of a dedicated channel when one is
// registered in the pool, else the default API (Java
// kusciaGrpcClientAdapter.xxx(request, channelNodeId)).
func (s *GraphService) kusciaFor(channel string) GraphKusciaAPI {
	s.extMu.RLock()
	pool := s.pool
	s.extMu.RUnlock()
	if pool != nil && channel != "" {
		if c, ok := pool.Lookup(channel); ok && c != nil {
			return c
		}
	}
	return s.kuscia
}

// --- signer ---

// TeeTaskSigner signs TEE task bodies (Java KusciaTrustedFlowJobConverter
// getTaskInitiatorId / signRSA256 / getCerts).
type TeeTaskSigner interface {
	InitiatorID() (string, error)
	Sign(input string) (string, error)
	Certs() ([]string, error)
}

// FileTeeTaskSigner reads client.pem / client.crt / ca.crt from CertDir.
type FileTeeTaskSigner struct {
	CertDir string
}

func (f *FileTeeTaskSigner) read(name string) (string, error) {
	b, err := os.ReadFile(filepath.Join(f.CertDir, name))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// InitiatorID is BASE32(SM3(DER(X.509 public key of ca.crt))) without padding.
func (f *FileTeeTaskSigner) InitiatorID() (string, error) {
	raw, err := f.read("ca.crt")
	if err != nil {
		return "", err
	}
	return TeeInitiatorIDFromCert([]byte(raw))
}

// TeeInitiatorIDFromCert computes the task initiator id of a PEM certificate.
func TeeInitiatorIDFromCert(pemBytes []byte) (string, error) {
	blk, _ := pem.Decode(pemBytes)
	if blk == nil {
		return "", errors.New("tee: invalid certificate pem")
	}
	cert, err := x509.ParseCertificate(blk.Bytes)
	if err != nil {
		return "", err
	}
	h := sm3Sum(cert.RawSubjectPublicKeyInfo)
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(h[:]), nil
}

// Sign is SHA256withRSA over input with client.pem, base64 encoded.
func (f *FileTeeTaskSigner) Sign(input string) (string, error) {
	raw, err := f.read("client.pem")
	if err != nil {
		return "", err
	}
	return TeeSignRS256([]byte(raw), input)
}

// TeeSignRS256 signs input with a PEM (PKCS#8 or PKCS#1) RSA private key.
func TeeSignRS256(keyPEM []byte, input string) (string, error) {
	var der []byte
	if blk, _ := pem.Decode(keyPEM); blk != nil {
		der = blk.Bytes
	} else {
		// Java accepts a bare base64 PKCS#8 body.
		s := strings.NewReplacer("-----BEGIN PRIVATE KEY-----", "", "-----END PRIVATE KEY-----", "", "\n", "", "\r", "").Replace(string(keyPEM))
		d, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return "", err
		}
		der = d
	}
	var key *rsa.PrivateKey
	if k, err := x509.ParsePKCS8PrivateKey(der); err == nil {
		rk, ok := k.(*rsa.PrivateKey)
		if !ok {
			return "", errors.New("tee: private key is not RSA")
		}
		key = rk
	} else if k1, err1 := x509.ParsePKCS1PrivateKey(der); err1 == nil {
		key = k1
	} else {
		return "", err
	}
	sum := sha256.Sum256([]byte(input))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(sig), nil
}

// Certs returns [client.crt, ca.crt].
func (f *FileTeeTaskSigner) Certs() ([]string, error) {
	c, err := f.read("client.crt")
	if err != nil {
		return nil, err
	}
	ca, err := f.read("ca.crt")
	if err != nil {
		return nil, err
	}
	return []string{c, ca}, nil
}

// --- trusted flow converter ---

// teeDistData is secretflow spec DistData (only data_refs are set).
type teeGraphDataRef struct {
	URI string `json:"uri"`
}

type teeGraphDistData struct {
	DataRefs []teeGraphDataRef `json:"data_refs"`
}

// teePipelineNodeDef is the Pipeline.NodeDef JSON signed as the task body.
type teePipelineNodeDef struct {
	Domain     string             `json:"domain,omitempty"`
	Name       string             `json:"name,omitempty"`
	Version    string             `json:"version,omitempty"`
	AttrPaths  []string           `json:"attr_paths,omitempty"`
	Attrs      []json.RawMessage  `json:"attrs,omitempty"`
	Inputs     []teeGraphDistData `json:"inputs,omitempty"`
	OutputURIs []string           `json:"output_uris,omitempty"`
}

// teeGraphEvalParam is NodeEvalParam (NodeDefUtils.toNodeEvalParam).
type teeGraphEvalParam struct {
	Version    string             `json:"version"`
	CompID     string             `json:"comp_id"`
	AttrPaths  []string           `json:"attr_paths,omitempty"`
	Attrs      []json.RawMessage  `json:"attrs,omitempty"`
	Inputs     []teeGraphDistData `json:"inputs,omitempty"`
	OutputURIs []string           `json:"output_uris,omitempty"`
}

// teeGraphTaskConfig is secretflow.pipeline.TaskConfig.
type teeGraphTaskConfig struct {
	TaskInitiatorID        string   `json:"task_initiator_id,omitempty"`
	Scope                  string   `json:"scope,omitempty"`
	TaskBody               string   `json:"task_body,omitempty"`
	Signature              string   `json:"signature,omitempty"`
	SignAlgorithm          string   `json:"sign_algorithm,omitempty"`
	TaskInitiatorCerts     []string `json:"task_initiator_certs,omitempty"`
	CapsuleManagerEndpoint string   `json:"capsule_manager_endpoint,omitempty"`
}

type teeGraphTaskInputConfig struct {
	SfDatasourceConfig map[string]graphjob.DatasourceRef `json:"sf_datasource_config"`
	SfNodeEvalParam    teeGraphEvalParam                 `json:"sf_node_eval_param"`
	TeeTaskConfig      teeGraphTaskConfig                `json:"tee_task_config"`
}

// TeeDmInputURL is JobConverter.buildDmInputUrl.
func TeeDmInputURL(id string) (string, error) {
	if id == "" {
		return "", padErr(errcode.GraphDependentNodeNotRun, "dm input id is null")
	}
	return teeDmInputPrefix + "id=" + id, nil
}

// TeeDmOutputURL is JobConverter.buildDmOutputUrl.
func TeeDmOutputURL(id string) (string, error) {
	if id == "" {
		return "", padErr(errcode.GraphDependentNodeNotRun, "dm input id | uri is null")
	}
	return teeDmOutputPrefix + "id=" + id + "&&datasource_id=" + graphjob.DefaultDataSourceID + "&&uri=" + id, nil
}

func marshalNoEscape(v any) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
	return strings.TrimSpace(buf.String())
}

// convertTrustedFlowJob mirrors KusciaTrustedFlowJobConverter.converter.
func (s *GraphService) convertTrustedFlowJob(_ context.Context, cc *chainContext, job *projectJob) (*kuscia.CreateJobRequest, error) {
	initiator := ""
	tasks := make([]kuscia.TaskConfig, 0, len(job.Tasks))
	for _, t := range job.Tasks {
		parties := make([]kuscia.Party, 0, len(t.Parties))
		if len(t.Parties) > 0 {
			initiator = t.Parties[0]
		}
		for _, p := range t.Parties {
			parties = append(parties, kuscia.Party{DomainID: p})
		}
		cfg, err := s.renderTeeTaskInputConfig(cc, t)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, kuscia.TaskConfig{
			TaskID:       t.TaskID,
			Alias:        t.TaskID,
			AppImage:     TeeAppImage,
			Parties:      parties,
			Dependencies: t.Dependencies,
			InputConfig:  cfg,
		})
	}
	return &kuscia.CreateJobRequest{
		JobID:          job.JobID,
		Initiator:      initiator,
		MaxParallelism: clampInt32(job.MaxParallelism),
		Tasks:          tasks,
	}, nil
}

// renderTeeTaskInputConfig mirrors KusciaTrustedFlowJobConverter.renderTaskInputConfig.
func (s *GraphService) renderTeeTaskInputConfig(cc *chainContext, t *jobTask) (string, error) {
	projectID := cc.project.ProjectID
	nd := t.Node.Def()
	inputs := make([]teeGraphDistData, 0, len(t.Node.Inputs))
	for _, in := range t.Node.Inputs {
		u, err := TeeDmInputURL(in)
		if err != nil {
			return "", err
		}
		inputs = append(inputs, teeGraphDistData{DataRefs: []teeGraphDataRef{{URI: u}}})
	}
	outputs := make([]string, 0, len(t.Node.Outputs))
	for _, o := range t.Node.Outputs {
		u, err := TeeDmOutputURL(o)
		if err != nil {
			return "", err
		}
		outputs = append(outputs, u)
	}
	pipe := teePipelineNodeDef{
		Domain: nd.Domain, Name: nd.Name, Version: nd.Version,
		AttrPaths: nd.AttrPaths, Attrs: nd.Attrs, Inputs: inputs, OutputURIs: outputs,
	}
	body := marshalNoEscape(pipe)
	body = base64.StdEncoding.EncodeToString([]byte(body))

	signer := s.teeSigner()
	initiatorID, err := signer.InitiatorID()
	if err != nil {
		return "", padErr(errcode.GraphDependentNodeNotRun, fmt.Sprintf("tee sign error: %v", err))
	}
	sig, err := signer.Sign(initiatorID + "." + projectID + "." + body)
	if err != nil {
		return "", padErr(errcode.GraphDependentNodeNotRun, fmt.Sprintf("tee sign error: %v", err))
	}
	certs, err := signer.Certs()
	if err != nil {
		return "", padErr(errcode.GraphDependentNodeNotRun, fmt.Sprintf("tee sign error: %v", err))
	}

	version := nd.Version
	if version == "" {
		version = "0.0.1"
	}
	compID := ""
	if nd.Domain != "" && nd.Name != "" {
		compID = nd.Domain + "/" + nd.Name + ":" + version
	}
	ds := make(map[string]graphjob.DatasourceRef, len(t.Parties))
	for _, p := range t.Parties {
		ds[p] = graphjob.DatasourceRef{ID: graphjob.DefaultDataSourceID}
	}
	cfg := teeGraphTaskInputConfig{
		SfDatasourceConfig: ds,
		SfNodeEvalParam: teeGraphEvalParam{
			Version: version, CompID: compID, AttrPaths: nd.AttrPaths, Attrs: nd.Attrs,
			Inputs: inputs, OutputURIs: outputs,
		},
		TeeTaskConfig: teeGraphTaskConfig{
			TaskInitiatorID:        initiatorID,
			Scope:                  projectID,
			TaskInitiatorCerts:     certs,
			TaskBody:               body,
			Signature:              sig,
			SignAlgorithm:          teeSignAlgorithm,
			CapsuleManagerEndpoint: strings.ReplaceAll(s.teeConfig().CapsuleManager, "#", cc.teeNodeID) + ":80",
		},
	}
	return marshalNoEscape(cfg), nil
}

// isTeeProject reports a TEE compute-mode project (Java endsWithIgnoreCase(mode, "tee")).
func isTeeComputeMode(mode string) bool {
	return strings.HasSuffix(strings.ToLower(mode), TeeProjectMode)
}

// RebuildTeeResultID mirrors GraphServiceImpl.getResultOutputVO: a result
// pulled from the TEE node to nodeID is stored as "{nodeId}-{teeResultId}";
// the center view reads the TEE side result "{teeResultId}" on teeNodeID.
func RebuildTeeResultID(nodeID, resultID, teeNodeID string) (centerNodeID, centerResultID string) {
	return teeNodeID, strings.ReplaceAll(resultID, nodeID+"-", "")
}
