package cloud

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"sathonate.local/fabric-control-plane/internal/domain"
	"sathonate.local/fabric-control-plane/internal/planner"
)

type Bundle struct {
	ExecutionAuthorized bool           `json:"executionAuthorized"`
	APIVersion          string         `json:"apiVersion"`
	TenantID            string         `json:"tenantId"`
	TargetID            string         `json:"targetId"`
	NetworkID           string         `json:"networkId"`
	PlanDigest          string         `json:"planDigest"`
	BevelCommit         string         `json:"bevelCommit"`
	GitOpsURL           string         `json:"gitOpsUrl"`
	GitBranch           string         `json:"gitBranch"`
	Variables           map[string]any `json:"variables"`
}

func secretFile(secret, key string) string {
	return "{{ lookup('ansible.builtin.file', '/run/sathonate-secrets/" + secret + "/" + key + "') }}"
}
func resources(storage string) map[string]any {
	return map[string]any{"requests": map[string]string{"cpu": "500m", "memory": "1Gi"}, "limits": map[string]string{"cpu": "2", "memory": "4Gi"}, "storagesize": storage}
}

// Compile never resolves credentials. It rejects unsupported fork assumptions instead of renaming MSP identities.
func Compile(s domain.NetworkSpec, t Target) (Bundle, error) {
	s = domain.WithDefaults(s)
	if domain.HasErrors(domain.Validate(s)) {
		return Bundle{}, fmt.Errorf("invalid desired topology")
	}
	for _, check := range StaticChecks(s, t) {
		if check.Status != "passed" {
			return Bundle{}, fmt.Errorf("%s: %s", check.ID, check.Message)
		}
	}
	p, e := planner.Create(s)
	if e != nil {
		return Bundle{}, e
	}
	branch := "networks/" + s.Metadata.ID
	repo, _ := url.Parse(t.GitOpsURL)
	network := map[string]any{"_id": s.Metadata.ID, "status": "new", "type": "fabric", "version": s.Spec.FabricVersion, "frontend": "disabled", "docker": map[string]string{"url": "ghcr.io/hyperledger", "username": "", "password": "", "enableCredentials": "no"}, "env": map[string]any{"type": s.Metadata.ID, "proxy": "haproxy", "retry_count": 60, "external_dns": "disabled", "annotations": map[string]any{"service": []any{}, "deployment": []any{}, "pvc": []any{}}}, "job_pod_resources": resources("1Gi")}
	organizations := []any{}
	globalOrderers := []map[string]any{}
	byOrgOrderers := map[string][]map[string]any{}
	byOrgPeers := map[string][]map[string]any{}
	for _, n := range s.Spec.Nodes {
		if !shortName.MatchString(n.ID) {
			return Bundle{}, fmt.Errorf("node %s exceeds this fork's 12-character name contract; explicit engine alias support is required", n.ID)
		}
		uri := n.ID + "." + n.OrganizationRef + "-net." + t.DNSDomain + ":8443"
		if n.Role == "orderer" {
			o := map[string]any{"orderer": nil, "_id": n.ID, "name": n.ID, "type": "orderer", "consensus": "raft", "org_name": n.OrganizationRef, "uri": uri, "certificate": "/home/bevel/certs/" + n.OrganizationRef + "/" + n.ID + "/ca.crt", "status": "new", "grpc": map[string]int{"port": 7050}, "pod_resources": resources("20Gi")}
			globalOrderers = append(globalOrderers, o)
			byOrgOrderers[n.OrganizationRef] = append(byOrgOrderers[n.OrganizationRef], o)
		} else {
			peer := map[string]any{"peer": nil, "_id": n.ID, "name": n.ID, "type": "anchor", "peerstatus": "new", "loglevel": "INFO", "gossippeeraddress": n.ID + "." + n.OrganizationRef + "-net:7051", "peerAddress": uri, "certificate": "/home/bevel/certs/" + n.OrganizationRef + "/" + n.ID + "/ca.crt", "cli": "disabled", "grpc": map[string]int{"port": 7051}, "events": map[string]int{"port": 7053}, "couchdb": map[string]any{"port": 5984, "storagesize": "50Gi"}, "pod_resources": resources("50Gi"), "chaincodes": []any{}}
			byOrgPeers[n.OrganizationRef] = append(byOrgPeers[n.OrganizationRef], peer)
		}
	}
	for _, o := range s.Spec.Organizations {
		binding := t.Organizations[o.ID]
		kind := "mixed"
		if len(byOrgPeers[o.ID]) == 0 {
			kind = "orderer"
		} else if len(byOrgOrderers[o.ID]) == 0 {
			kind = "peer"
		}
		if strings.ContainsAny(binding.State+binding.Location, "{}\n\r/") {
			return Bundle{}, fmt.Errorf("organization subject fields contain unsupported characters")
		}
		organizations = append(organizations, map[string]any{"organization": nil, "_id": o.ID, "name": o.ID, "type": kind, "country": binding.Country, "state": binding.State, "location": binding.Location, "subject": "O=" + o.ID + ",C=" + binding.Country, "external_url_suffix": t.DNSDomain, "org_status": "new", "fabric_console": "disabled", "cloud_provider": "aws", "aws": map[string]string{"access_key": "", "secret_key": ""}, "k8s": map[string]string{"region": t.Region, "context": t.KubeContext, "config_file": "/run/sathonate-secrets/eks-access/kubeconfig"}, "vault": map[string]string{"type": "hashicorp", "url": binding.VaultURL, "secret_path": binding.VaultMount, "root_token": secretFile(binding.VaultTokenSecret, "token")}, "gitops": map[string]string{"git_protocol": "https", "git_url": t.GitOpsURL, "git_repo": repo.Host + repo.Path, "branch": branch, "release_dir": "platforms/hyperledger-fabric/releases/" + s.Metadata.ID, "chart_source": "platforms/hyperledger-fabric/charts", "username": secretFile(t.GitOpsSecret, "username"), "password": secretFile(t.GitOpsSecret, "password"), "email": "automation@sathonate.invalid", "private_key": ""}, "ca_data": map[string]string{"url": "ca." + o.ID + "-net." + t.DNSDomain + ":8443", "certificate": "/home/bevel/certs/" + o.ID + "/ca.crt"}, "services": map[string]any{"ca": map[string]any{"_id": o.ID + "-ca", "name": "ca", "type": "ca", "status": "new", "subject": "/C=" + binding.Country + "/O=" + o.ID, "grpc": map[string]int{"port": 7054}, "pod_resources": resources("10Gi")}, "consensus": map[string]string{"name": "raft"}, "orderers": nonNil(byOrgOrderers[o.ID]), "peers": nonNil(byOrgPeers[o.ID])}})
	}
	channels := []any{}
	orderersByID := map[string]map[string]any{}
	for _, o := range globalOrderers {
		orderersByID[o["name"].(string)] = o
	}
	for _, c := range s.Spec.Channels {
		participants := []any{}
		endorsers := []any{}
		ordererOrganizations := []any{}
		creator := false
		orderingAddress := ""
		if len(c.OrdererRefs) > 0 {
			orderingAddress = orderersByID[c.OrdererRefs[0]]["uri"].(string)
		}
		for _, org := range c.OrganizationRefs {
			peers := []any{}
			endorserPeers := []any{}
			orderers := []any{}
			for _, peer := range byOrgPeers[org] {
				id := peer["name"].(string)
				if !contains(c.PeerRefs, id) {
					continue
				}
				address := peer["peerAddress"]
				peers = append(peers, map[string]any{"peer": nil, "name": id, "peerstatus": "new", "anchorstatus": "new", "gossipAddress": address, "peerAddress": address})
				endorserPeers = append(endorserPeers, map[string]any{"peer": nil, "name": id, "corepeerAddress": address, "certificate": peer["certificate"]})
			}
			for _, orderer := range byOrgOrderers[org] {
				if contains(c.OrdererRefs, orderer["name"].(string)) {
					orderers = append(orderers, orderer)
				}
			}
			role := "joiner"
			if !creator && len(peers) > 0 {
				role = "creator"
				creator = true
			}
			participants = append(participants, map[string]any{"organization": nil, "name": org, "type": role, "org_status": "new", "ordererAddress": orderingAddress, "peers": peers, "orderers": orderers, "chaincodes": []any{}})
			if len(endorserPeers) > 0 {
				endorsers = append(endorsers, map[string]any{"organization": nil, "name": org, "peers": endorserPeers})
			}
			if len(orderers) > 0 {
				ordererOrganizations = append(ordererOrganizations, map[string]string{"name": org})
			}
		}
		channels = append(channels, map[string]any{"channel": nil, "_id": c.ID, "channel_name": c.ID, "channel_status": "new", "consortium": "consortium", "genesis": map[string]string{"name": "genesis"}, "participants": participants, "endorsers": endorsers, "orderer": ordererOrganizations, "chaincodes": []any{}})
	}
	network["organizations"] = organizations
	network["orderers"] = globalOrderers
	network["channels"] = channels
	return Bundle{APIVersion: "cloud.sathonate.io/v1alpha1", TenantID: t.TenantID, TargetID: t.ID, NetworkID: s.Metadata.ID, PlanDigest: p.ID, BevelCommit: t.BevelCommit, GitOpsURL: t.GitOpsURL, GitBranch: branch, Variables: map[string]any{"network": network, "disableCallBack": "true", "performance_mode": "false", "no_ansible_log": true, "keep_build_dir": "true", "final_flux_poll_change": "false"}}, nil
}
func nonNil(v []map[string]any) []map[string]any {
	if v == nil {
		return []map[string]any{}
	}
	return v
}
func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}

// Manifests emits credentials-free staging resources. It does not create them in Kubernetes.
func Manifests(bundle Bundle, target Target) (map[string]any, error) {
	b, e := json.Marshal(bundle)
	if e != nil {
		return nil, e
	}
	hash := sha256.Sum256(b)
	digest := hex.EncodeToString(hash[:])
	resourceName := "sathonate-" + digest[:20]
	labels := map[string]string{"app.kubernetes.io/managed-by": "sathonate", "sathonate.io/network": bundle.NetworkID, "sathonate.io/bundle": digest[:32]}
	config := map[string]any{"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]any{"name": resourceName, "namespace": target.RunnerNamespace, "labels": labels}, "immutable": true, "data": map[string]string{"bundle.json": string(b)}}
	mounts := []any{map[string]any{"name": "bundle", "mountPath": "/inputs", "readOnly": true}, map[string]any{"name": "workspace", "mountPath": "/home"}}
	volumes := []any{map[string]any{"name": "bundle", "configMap": map[string]string{"name": resourceName}}, map[string]any{"name": "workspace", "emptyDir": map[string]string{"sizeLimit": "20Gi"}}}
	for i, secret := range secretNames(target) {
		name := fmt.Sprintf("secret-%d", i)
		volumes = append(volumes, map[string]any{"name": name, "secret": map[string]any{"secretName": secret, "defaultMode": 288}})
		mounts = append(mounts, map[string]any{"name": name, "mountPath": "/run/sathonate-secrets/" + secret, "readOnly": true})
	}
	job := map[string]any{"apiVersion": "batch/v1", "kind": "Job", "metadata": map[string]any{"name": resourceName, "namespace": target.RunnerNamespace, "labels": labels}, "spec": map[string]any{"backoffLimit": 0, "activeDeadlineSeconds": 7200, "ttlSecondsAfterFinished": 86400, "template": map[string]any{"metadata": map[string]any{"labels": labels}, "spec": map[string]any{"restartPolicy": "Never", "serviceAccountName": "sathonate-runner", "automountServiceAccountToken": false, "securityContext": map[string]any{"runAsNonRoot": true, "runAsUser": 10001, "runAsGroup": 10001, "fsGroup": 10001, "seccompProfile": map[string]string{"type": "RuntimeDefault"}}, "containers": []any{map[string]any{"name": "bevel", "image": target.RunnerImage, "imagePullPolicy": "IfNotPresent", "command": []string{"/opt/sathonate/bin/cloud-runner"}, "args": []string{"-bundle", "/inputs/bundle.json", "-sha256", digest}, "env": []any{map[string]string{"name": "HOME", "value": "/home"}, map[string]string{"name": "ANSIBLE_NO_LOG", "value": "true"}}, "securityContext": map[string]any{"allowPrivilegeEscalation": false, "capabilities": map[string]any{"drop": []string{"ALL"}}}, "resources": map[string]any{"requests": map[string]string{"cpu": "1", "memory": "2Gi"}, "limits": map[string]string{"cpu": "4", "memory": "8Gi"}}, "volumeMounts": mounts}}, "volumes": volumes}}}}
	job["spec"].(map[string]any)["suspend"] = true
	return map[string]any{"apiVersion": "v1", "kind": "List", "items": []any{config, job}}, nil
}
