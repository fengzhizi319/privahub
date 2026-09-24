package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/fengzhizi319/privahub/internal/graphjob"
	"github.com/fengzhizi319/privahub/pkg/errcode"
)

// ComponentService mirrors Java ComponentServiceImpl: component definitions
// are SecretFlow CompListDef JSON files (config/components/{app}.json, see
// scripts/gen_components.py) plus config/i18n/{app}.json translations.
type ComponentService struct {
	settings func() *graphjob.Settings

	once  sync.Once
	lists []compListDef // loaded + merged (JsonProtobufSourceFactory semantics)
	err   error
}

// NewComponentService creates a ComponentService using the global graph settings.
func NewComponentService() *ComponentService {
	return &ComponentService{settings: graphjob.GetSettings}
}

// NewComponentServiceWithSettings creates a ComponentService with explicit settings (tests).
func NewComponentServiceWithSettings(s *graphjob.Settings) *ComponentService {
	return &ComponentService{settings: func() *graphjob.Settings { return s }}
}

type compListDef struct {
	Name    string            `json:"name"`
	Desc    string            `json:"desc"`
	Version string            `json:"version"`
	Comps   []json.RawMessage `json:"comps"`
}

type compHeader struct {
	Domain  string `json:"domain"`
	Name    string `json:"name"`
	Desc    string `json:"desc"`
	Version string `json:"version"`
}

// ComponentSummaryDef mirrors Java ComponentSummaryDef.
type ComponentSummaryDef struct {
	Domain  string `json:"domain"`
	Name    string `json:"name"`
	Desc    string `json:"desc"`
	Version string `json:"version"`
}

// CompListVO mirrors Java CompListVO.
type CompListVO struct {
	Name    string                `json:"name"`
	Desc    string                `json:"desc"`
	Version string                `json:"version"`
	Comps   []ComponentSummaryDef `json:"comps"`
}

// ComponentKey mirrors Java GetComponentRequest / ComponentKey.
type ComponentKey struct {
	App     string `json:"app"`
	Domain  string `json:"domain"`
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

const (
	appSecretflow   = "secretflow"
	appSecretpad    = "secretpad"
	appSecretpadTee = "secretpad_tee"
	appSCQL         = "scql"
	appTrustedflow  = "trustedflow"
)

func (s *ComponentService) load() ([]compListDef, error) {
	s.once.Do(func() {
		st := s.settings()
		items := []compListDef{}
		entries, err := os.ReadDir(st.ComponentDir)
		if err == nil {
			for _, e := range entries {
				if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
					continue
				}
				b, err := os.ReadFile(filepath.Join(st.ComponentDir, e.Name()))
				if err != nil {
					continue
				}
				var l compListDef
				if json.Unmarshal(b, &l) != nil || l.Name == "" {
					continue
				}
				items = append(items, l)
			}
		}
		if len(items) == 0 {
			items = legacyComponents(st.LegacyComponentFile)
		}
		sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
		var secretpad, secretpadTee, scql []json.RawMessage
		for _, l := range items {
			switch l.Name {
			case appSecretpad:
				secretpad = l.Comps
			case appSecretpadTee:
				secretpadTee = l.Comps
			case appSCQL:
				scql = l.Comps
			}
		}
		for _, l := range items {
			switch l.Name {
			case appTrustedflow:
				l.Comps = append(append([]json.RawMessage{}, l.Comps...), secretpadTee...)
			case appSecretflow:
				l.Comps = append(append(append([]json.RawMessage{}, l.Comps...), secretpad...), scql...)
			}
			s.lists = append(s.lists, l)
		}
		if len(s.lists) == 0 {
			s.err = padErr(errcode.GraphComponentNotExists, "no component definitions under "+st.ComponentDir)
		}
	})
	return s.lists, s.err
}

// legacyComponents converts the old flat config/components.json into a
// "secretflow" CompListDef (fallback when config/components is absent).
func legacyComponents(path string) []compListDef {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var flat []map[string]json.RawMessage
	if json.Unmarshal(b, &flat) != nil {
		return nil
	}
	l := compListDef{Name: appSecretflow, Desc: "First-party SecretFlow components.", Version: "1.0"}
	for _, c := range flat {
		var domain, name, codeName string
		_ = json.Unmarshal(c["domain"], &domain)
		_ = json.Unmarshal(c["code_name"], &codeName)
		if domain == "" {
			if i := strings.Index(codeName, "/"); i > 0 {
				domain = codeName[:i]
			}
		}
		if i := strings.Index(codeName, "/"); i > 0 {
			name = codeName[i+1:]
		} else {
			_ = json.Unmarshal(c["name"], &name)
		}
		c["domain"], _ = json.Marshal(domain)
		c["name"], _ = json.Marshal(name)
		delete(c, "code_name")
		delete(c, "category")
		raw, _ := json.Marshal(c)
		l.Comps = append(l.Comps, raw)
	}
	return []compListDef{l}
}

// ListComponents mirrors listComponents: Map<app, CompListVO>, hidden
// components removed, secretpad / scql apps dropped.
func (s *ComponentService) ListComponents() (map[string]CompListVO, error) {
	lists, err := s.load()
	if err != nil {
		return nil, err
	}
	st := s.settings()
	resp := map[string]CompListVO{}
	for _, l := range lists {
		if len(l.Comps) == 0 {
			continue
		}
		vo := CompListVO{Name: l.Name, Desc: l.Desc, Version: l.Version, Comps: []ComponentSummaryDef{}}
		for _, raw := range l.Comps {
			var h compHeader
			if json.Unmarshal(raw, &h) != nil {
				continue
			}
			if st.IsHidden(l.Name + "/" + h.Domain + "/" + h.Name + ":" + h.Version) {
				continue
			}
			vo.Comps = append(vo.Comps, ComponentSummaryDef(h))
		}
		resp[l.Name] = vo
	}
	delete(resp, appSecretpad)
	delete(resp, appSCQL)
	return resp, nil
}

// BatchGetComponent mirrors batchGetComponent: ordered definitions, error on
// a missing key. An empty app searches secretflow first, then every app.
func (s *ComponentService) BatchGetComponent(keys []ComponentKey) ([]json.RawMessage, error) {
	lists, err := s.load()
	if err != nil {
		return nil, err
	}
	index := map[string]json.RawMessage{}
	byName := map[string]json.RawMessage{}
	for _, l := range lists {
		for _, raw := range l.Comps {
			var h compHeader
			if json.Unmarshal(raw, &h) != nil {
				continue
			}
			index[l.Name+"/"+h.Domain+"/"+h.Name] = raw
			k := h.Domain + "/" + h.Name
			if _, ok := byName[k]; !ok || l.Name == appSecretflow {
				byName[k] = raw
			}
		}
	}
	out := make([]json.RawMessage, 0, len(keys))
	for _, k := range keys {
		var def json.RawMessage
		var ok bool
		if k.App != "" {
			def, ok = index[k.App+"/"+k.Domain+"/"+k.Name]
		}
		if !ok {
			def, ok = byName[k.Domain+"/"+k.Name]
		}
		if !ok {
			return nil, padErr(errcode.GraphComponentNotExists, k.App+"/"+k.Domain+"/"+k.Name)
		}
		out = append(out, def)
	}
	return out, nil
}

// ListComponentI18n mirrors listComponentI18n: Map<app, Map<key, text>>,
// secretpad texts merged into every app, scql into secretflow, hidden keys removed.
func (s *ComponentService) ListComponentI18n() (map[string]map[string]any, error) {
	st := s.settings()
	entries, err := os.ReadDir(st.I18nDir)
	if err != nil {
		return nil, padErr(errcode.GraphComponent18nError, err.Error())
	}
	config := map[string]map[string]any{}
	secretpad, scql := map[string]any{}, map[string]any{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(st.I18nDir, e.Name()))
		if err != nil {
			return nil, padErr(errcode.GraphComponent18nError, err.Error())
		}
		var content map[string]any
		if json.Unmarshal(b, &content) != nil || len(content) == 0 {
			continue
		}
		app := strings.TrimSuffix(e.Name(), ".json")
		switch app {
		case appSecretpad:
			secretpad = content
		case appSCQL:
			scql = content
		default:
			config[app] = content
		}
	}
	for app, m := range config {
		for k, v := range secretpad {
			m[k] = v
		}
		if app == appSecretflow {
			for k, v := range scql {
				m[k] = v
			}
		}
	}
	for app, m := range config {
		for k := range m {
			if st.IsHidden(app + "/" + k) {
				delete(m, k)
			}
		}
	}
	return config, nil
}

// VersionList mirrors listComponentVersion(deployMode).
func (s *ComponentService) VersionList() map[string]string {
	return s.settings().VersionsForDeployMode()
}
