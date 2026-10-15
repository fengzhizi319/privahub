package service

import (
	"fmt"
	"strings"
)

// ValidateServingPartyConfigs mirrors the bean validation of Java
// CreateModelServingRequest.partyConfigs (@Valid List<PartyConfig>):
// nodeId / featureTableId @NotBlank, isMock @NotNull and every resource's
// minCPU / maxCPU / minMemory / maxMemory @NotBlank. It returns "" when valid,
// otherwise the VALIDATION_ERROR message.
func ValidateServingPartyConfigs(parties []ServingPartyConfig) string {
	if len(parties) == 0 {
		return "partyConfigs must not be empty"
	}
	seen := make(map[string]bool, len(parties))
	for i, p := range parties {
		prefix := fmt.Sprintf("partyConfigs[%d].", i)
		switch {
		case strings.TrimSpace(p.NodeID) == "":
			return prefix + "nodeId must not be blank"
		case strings.TrimSpace(p.FeatureTableID) == "":
			return prefix + "featureTableId must not be blank"
		case p.IsMock == nil:
			return prefix + "isMock must not be null"
		}
		// Kuscia parties are a list, while party_configs is a map keyed by node.
		// Duplicate ids would silently overwrite configuration on only one side.
		if seen[p.NodeID] {
			return prefix + "nodeId must be unique"
		}
		seen[p.NodeID] = true
		for j, r := range p.Resources {
			rp := fmt.Sprintf("%sresources[%d].", prefix, j)
			for _, f := range []struct{ name, v string }{
				{"minCPU", r.MinCPU}, {"maxCPU", r.MaxCPU}, {"minMemory", r.MinMemory}, {"maxMemory", r.MaxMemory},
			} {
				if strings.TrimSpace(f.v) == "" {
					return rp + f.name + " must not be blank"
				}
			}
		}
	}
	return ""
}
