package deployment

import "errors"

// Optional trusted inventory for the explicit four-node local qualification.
// Customers select approved placements; they cannot add hosts through a CR.
// Empty inventory preserves the qualified single-node profile and its digest.
func validateLocalWorkerInventory(c SecureLocalConfig) error {
	if len(c.WorkerNodes) == 0 {
		return nil
	}
	deny := errors.New("multi-node local target requires three distinct owned workers and exact worker-only placements")
	if len(c.WorkerNodes) != 3 {
		return deny
	}
	allowed := map[string]bool{c.ClusterName + "-worker": true, c.ClusterName + "-worker2": true, c.ClusterName + "-worker3": true}
	seen := map[string]bool{}
	for _, name := range c.WorkerNodes {
		if !allowed[name] || seen[name] {
			return deny
		}
		seen[name] = true
	}
	placed := map[string]bool{}
	for _, selector := range c.Runtime.NodeSelectors {
		name := selector["kubernetes.io/hostname"]
		if len(selector) != 1 || !allowed[name] {
			return deny
		}
		placed[name] = true
	}
	if len(placed) != 3 {
		return deny
	}
	return nil
}

// Readiness is deliberately separate from ownership: an unavailable owned
// worker must remain observable without authorizing a different host.
func localWorkerProviderMatches(cluster, node, provider string) bool {
	return provider == "kind://docker/"+cluster+"/"+node
}
