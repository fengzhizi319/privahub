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
