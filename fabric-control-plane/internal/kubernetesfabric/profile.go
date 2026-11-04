package kubernetesfabric

import (
	"fmt"
	"os"

	"sigs.k8s.io/yaml"
)

// Write the same SDK connection schema used by kubectl-hlf inspect without
// inspect's cluster-wide discovery (v1.14.0 filters only after listing all CRs).
func (w *Worker) scopedProfile(path string) error {
	organizations, peers, orderers, channelPeers := Object{}, Object{}, Object{}, Object{}
	ordererNames := []string{}
	for _, o := range w.Spec.Spec.Organizations {
		organizations[o.MSPID] = Object{"mspid": o.MSPID, "cryptoPath": "/workspace/cryptopath", "users": Object{}, "peers": []string{}, "orderers": []string{}}
	}
	for _, n := range w.Spec.Spec.Nodes {
		resource, port := "fabricpeers", "7051"
		if n.Role == "orderer" {
			resource, port = "fabricorderernodes", "7050"
		}
		v, e := w.get(resource, n.ID)
		if e != nil {
			return e
		}
		status := object(v["status"])
		cert := text(status["tlsCaCert"])
		if cert == "" {
			cert = text(status["tlsCert"])
		}
		if cert == "" {
			return fmt.Errorf("node TLS trust is unavailable")
		}
		name := n.ID + "." + w.Namespace
		connection := Object{"url": "grpcs://" + name + ":" + port, "grpcOptions": Object{"allow-insecure": false}, "tlsCACerts": Object{"pem": cert}}
		msp := ""
		for _, o := range w.Spec.Spec.Organizations {
			if o.ID == n.OrganizationRef {
				msp = o.MSPID
			}
		}
		org := object(organizations[msp])
		if n.Role == "peer" {
			peers[name] = connection
			org["peers"] = append(org["peers"].([]string), name)
			channelPeers[name] = Object{"discover": true, "endorsingPeer": true, "chaincodeQuery": true, "ledgerQuery": true, "eventSource": true}
		} else {
			orderers[name] = connection
			org["orderers"] = append(org["orderers"].([]string), name)
			ordererNames = append(ordererNames, name)
		}
	}
	profile := Object{"name": "fabric-smp", "version": "1.0.0", "client": Object{"organization": w.Spec.Spec.Organizations[0].MSPID}, "organizations": organizations, "peers": peers, "orderers": orderers, "certificateAuthorities": Object{}, "channels": Object{"_default": Object{"orderers": ordererNames, "peers": channelPeers}, w.Spec.Spec.Channels[0].ID: Object{"orderers": ordererNames, "peers": channelPeers}}}
	raw, e := yaml.Marshal(profile)
	if e != nil {
		return e
	}
	return os.WriteFile(path, raw, 0600)
}
