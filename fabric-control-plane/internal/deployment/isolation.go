package deployment

import (
	"context"
	"encoding/json"
	"errors"
	"net"
)

// Install containment before creating any customer workload. API destinations
// come from this guarded disposable cluster, never a customer request.
func (r SecureRunner) installIsolation(ctx context.Context, ns string) error {
	b := r.base()
	raw, err := b.run(ctx, "-n", "default", "get", "endpoints", "kubernetes", "-o", "json")
	if err != nil {
		return errors.New("cannot determine guarded cluster API endpoints")
	}
	var endpoints struct {
		Subsets []struct{ Addresses []struct{ IP string } }
	}
	if json.Unmarshal(raw, &endpoints) != nil {
		return errors.New("invalid API endpoints")
	}
	service, err := b.run(ctx, "-n", "default", "get", "service", "kubernetes", "-o", "jsonpath={.spec.clusterIP}")
	if err != nil || net.ParseIP(string(service)) == nil {
		return errors.New("invalid API service address")
	}
	ips := []string{string(service)}
	for _, set := range endpoints.Subsets {
		for _, address := range set.Addresses {
			if net.ParseIP(address.IP) == nil {
				return errors.New("invalid control-plane IP")
			}
			ips = append(ips, address.IP)
		}
	}
	if len(ips) < 2 {
		return errors.New("control-plane endpoints unavailable")
	}
	manifest := isolationManifest(ns, ips)
	if r.binding.remote != nil {
		for _, item := range manifest["items"].([]any) {
			obj := item.(map[string]any)
			if obj["kind"] == "ResourceQuota" {
				obj["spec"].(map[string]any)["hard"].(map[string]any)["requests.storage"] = "80Gi"
			}
		}
	}
	return b.apply(ctx, manifest)
}

func isolationManifest(ns string, apiIPs []string) map[string]any {
	type M = map[string]any
	meta := func(name string) M {
		return M{"name": name, "namespace": ns, "labels": M{"fabric.sathonate.io/profile": "smp-secure-local-v1"}}
	}
	policy := func(name string, spec M) M {
		return M{"apiVersion": "networking.k8s.io/v1", "kind": "NetworkPolicy", "metadata": meta(name), "spec": spec}
	}
	ports := []any{}
	for _, port := range []int{7050, 7051, 7052, 7053, 7054, 9443, 9999} {
		ports = append(ports, M{"protocol": "TCP", "port": port})
	}
	same := M{"podSelector": M{}}
	apiPeers := []any{}
	for _, ip := range apiIPs {
		suffix := "/32"
		if net.ParseIP(ip).To4() == nil {
			suffix = "/128"
		}
		apiPeers = append(apiPeers, M{"ipBlock": M{"cidr": ip + suffix}})
	}
	dns := M{"namespaceSelector": M{"matchLabels": M{"kubernetes.io/metadata.name": "kube-system"}}, "podSelector": M{"matchLabels": M{"k8s-app": "kube-dns"}}}
	items := []any{
		M{"apiVersion": "v1", "kind": "ServiceAccount", "metadata": meta("default"), "automountServiceAccountToken": false},
		M{"apiVersion": "v1", "kind": "ResourceQuota", "metadata": meta("network-budget"), "spec": M{"hard": M{"pods": "40", "requests.cpu": "16", "requests.memory": "16Gi", "limits.cpu": "40", "limits.memory": "40Gi", "persistentvolumeclaims": "20", "requests.storage": "32Gi", "secrets": "200", "configmaps": "100", "services": "32"}}},
		M{"apiVersion": "v1", "kind": "LimitRange", "metadata": meta("container-budget"), "spec": M{"limits": []any{M{"type": "Container", "default": M{"cpu": "1", "memory": "512Mi"}, "defaultRequest": M{"cpu": "100m", "memory": "128Mi"}, "max": M{"cpu": "2", "memory": "2Gi"}}}}},
		policy("default-deny", M{"podSelector": M{}, "policyTypes": []string{"Ingress", "Egress"}}),
		policy("fabric-traffic", M{"podSelector": M{}, "policyTypes": []string{"Ingress", "Egress"}, "ingress": []any{M{"from": []any{same}, "ports": ports}}, "egress": []any{M{"to": []any{same}, "ports": ports}, M{"to": []any{dns}, "ports": []any{M{"protocol": "UDP", "port": 53}, M{"protocol": "TCP", "port": 53}}}}}),
		policy("controller-issuer-api", M{"podSelector": M{"matchExpressions": []any{M{"key": "sathonate.io/api-client", "operator": "In", "values": []string{"true"}}}}, "policyTypes": []string{"Egress"}, "egress": []any{M{"to": apiPeers, "ports": []any{M{"protocol": "TCP", "port": 443}, M{"protocol": "TCP", "port": 6443}}}}}),
	}
	return M{"apiVersion": "v1", "kind": "List", "items": items}
}
