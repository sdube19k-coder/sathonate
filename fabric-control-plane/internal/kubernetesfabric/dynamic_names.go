package kubernetesfabric

import (
	"crypto/sha256"
	"encoding/hex"
	"sathonate.local/fabric-control-plane/internal/domain"
	"strconv"
)

func DynamicService(channel, name string, sequence int) string {
	sum := sha256.Sum256([]byte(channel + "/" + name))
	return "cc-" + hex.EncodeToString(sum[:8]) + "-s" + strconv.Itoa(sequence)
}

func (w *Worker) dynamic() bool {
	return w.Spec.Spec.Deployment.TargetRef == domain.DynamicLocalProfile
}

// CA resource names and issuer roles are resolved independently. A split TLS
// authority must never fall back to the organization's signing root.
func (w *Worker) authority(org, caname string) string {
	if !w.dynamic() || w.Spec.Spec.Configuration == nil {
		return org
	}
	role := "signing"
	if caname == "tlsca" {
		role = "tls"
	}
	for _, a := range w.Spec.Spec.Configuration.Authorities {
		if a.OrganizationRef == org {
			for _, r := range a.Roles {
				if r == role {
					return a.ID
				}
			}
		}
	}
	return ""
}
func (w *Worker) caName(id string) string {
	if w.dynamic() {
		return id
	}
	return id + "-ca"
}
func (w *Worker) peerMSP(org string) string {
	for _, o := range w.Spec.Spec.Organizations {
		if o.ID == org {
			return o.MSPID
		}
	}
	return ""
}
func (w *Worker) firstPeer(org string) string {
	for _, n := range w.Spec.Spec.Nodes {
		if n.Role == "peer" && n.OrganizationRef == org {
			return n.ID
		}
	}
	return ""
}

func (w *Worker) dynamicSettings(name string) (domain.NodeSettings, bool) {
	if !w.dynamic() || w.Spec.Spec.Configuration == nil {
		return domain.NodeSettings{}, false
	}
	if n, ok := w.Spec.Spec.Configuration.Nodes[name]; ok {
		return n, true
	}
	for _, a := range w.Spec.Spec.Configuration.Authorities {
		if a.ID == name {
			return domain.NodeSettings{Resources: a.Resources, Storage: a.Storage}, true
		}
	}
	return domain.NodeSettings{}, false
}
