// Package cloud integrates registered EKS targets with the pinned Bevel contract.
package cloud

import (
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"regexp"
)

type Target struct {
	Operator         *OperatorTarget                `json:"operator,omitempty"`
	ID               string                         `json:"id"`
	TenantID         string                         `json:"tenantId"`
	Name             string                         `json:"name"`
	AccountID        string                         `json:"accountId"`
	Region           string                         `json:"region"`
	ClusterName      string                         `json:"clusterName"`
	KubeContext      string                         `json:"kubeContext"`
	RunnerNamespace  string                         `json:"runnerNamespace"`
	StorageClass     string                         `json:"storageClass"`
	DNSDomain        string                         `json:"dnsDomain"`
	GitOpsURL        string                         `json:"gitOpsUrl"`
	GitOpsSecret     string                         `json:"gitOpsSecret"`
	RunnerImage      string                         `json:"runnerImage"`
	BevelCommit      string                         `json:"bevelCommit"`
	BevelCheckout    string                         `json:"bevelCheckout,omitempty"`
	FabricVersion    string                         `json:"fabricVersion"`
	CertificationRef string                         `json:"certificationRef,omitempty"`
	Organizations    map[string]OrganizationBinding `json:"organizations"`
	Placements       map[string]Placement           `json:"placements"`
}
type OrganizationBinding struct {
	Country          string `json:"country"`
	State            string `json:"state"`
	Location         string `json:"location"`
	VaultURL         string `json:"vaultUrl"`
	VaultMount       string `json:"vaultMount"`
	VaultTokenSecret string `json:"vaultTokenSecret"`
}
type Placement struct {
	Zone         string            `json:"zone"`
	NodeSelector map[string]string `json:"nodeSelector"`
}
type Registry struct {
	Targets []Target `json:"targets"`
}

var name = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
var shortName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,11}$`)
var digestImage = regexp.MustCompile(`^[a-zA-Z0-9./:_-]+@sha256:[a-f0-9]{64}$`)
var commitID = regexp.MustCompile(`^[a-f0-9]{40}$`)

func LoadRegistry(path string) (Registry, error) {
	if path == "" {
		return Registry{Targets: []Target{}}, nil
	}
	f, e := os.Open(path)
	if e != nil {
		return Registry{}, e
	}
	defer f.Close()
	var registry Registry
	decoder := json.NewDecoder(f)
	decoder.DisallowUnknownFields()
	if e = decoder.Decode(&registry); e != nil {
		return Registry{}, e
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return Registry{}, errors.New("registry requires one JSON document")
	}
	seen := map[string]bool{}
	namespaces := map[string]bool{}
	for _, t := range registry.Targets {
		if !name.MatchString(t.ID) || t.TenantID == "" || seen[t.TenantID+"/"+t.ID] {
			return Registry{}, errors.New("invalid or duplicate registered target")
		}
		seen[t.TenantID+"/"+t.ID] = true
		if t.Operator != nil {
			binding := t.AccountID + "/" + t.Region + "/" + t.ClusterName + "/" + t.RunnerNamespace
			if namespaces[binding] {
				return Registry{}, errors.New("operator namespace cannot be shared by target bindings")
			}
			namespaces[binding] = true
		}
	}
	return registry, nil
}
func (r Registry) Find(tenant, id string) (Target, bool) {
	for _, t := range r.Targets {
		if t.TenantID == tenant && t.ID == id {
			return t, true
		}
	}
	return Target{}, false
}

type Summary struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Region           string `json:"region"`
	ClusterName      string `json:"clusterName"`
	Provider         string `json:"provider"`
	ExecutionEnabled bool   `json:"executionEnabled"`
}

func (r Registry) Summaries(tenant string) []Summary {
	out := []Summary{}
	for _, t := range r.Targets {
		if t.TenantID == tenant {
			out = append(out, Summary{ID: t.ID, Name: t.Name, Region: t.Region, ClusterName: t.ClusterName, Provider: "aws", ExecutionEnabled: false})
		}
	}
	return out
}
func httpsURL(value string) bool {
	u, e := url.Parse(value)
	return e == nil && u.Scheme == "https" && u.Host != "" && u.User == nil && u.RawQuery == "" && u.Fragment == ""
}
