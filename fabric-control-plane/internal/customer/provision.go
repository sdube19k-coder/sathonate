package customer

import (
	"context"
	"encoding/json"
	"io"
	"regexp"
	"strings"

	"sathonate.local/fabric-control-plane/internal/domain"
	"sathonate.local/fabric-control-plane/internal/topology"
)

var namespaceName = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
var publicDigest = regexp.MustCompile(`^[a-f0-9]{64}$`)

type Target struct {
	Ref           string                                `json:"ref"`
	Tenant        string                                `json:"tenant"`
	Project       string                                `json:"project"`
	ClusterSHA256 string                                `json:"clusterSHA256"`
	Namespace     string                                `json:"namespace"`
	NamespaceUID  string                                `json:"namespaceUid"`
	Storage       map[string]topology.StorageCapability `json:"storage"`
}
type Targets []Target

func LoadTargets(reader io.Reader) (Targets, error) {
	var targets Targets
	limit := &io.LimitedReader{R: reader, N: 1 << 20}
	decoder := json.NewDecoder(limit)
	decoder.DisallowUnknownFields()
	var extra any
	if decoder.Decode(&targets) != nil || decoder.Decode(&extra) != io.EOF || limit.N == 0 || len(targets) == 0 {
		return nil, ErrInvalid
	}
	refs := map[[3]string]bool{}
	namespaces := map[[2]string]bool{}
	for _, target := range targets {
		ref := [3]string{target.Tenant, target.Project, target.Ref}
		namespace := [2]string{target.ClusterSHA256, target.Namespace}
		if !target.valid() || refs[ref] || namespaces[namespace] {
			return nil, ErrInvalid
		}
		refs[ref], namespaces[namespace] = true, true
	}
	return targets, nil
}
func (t Target) valid() bool {
	return idPattern.MatchString(t.Ref) && idPattern.MatchString(t.Tenant) && idPattern.MatchString(t.Project) &&
		publicDigest.MatchString(t.ClusterSHA256) && namespaceName.MatchString(t.Namespace) && t.Namespace != "default" &&
		t.Namespace != "smp-qualification" && !strings.HasPrefix(t.Namespace, "kube-") &&
		idPattern.MatchString(t.NamespaceUID) && len(t.Storage) > 0
}

type ProvisionPreview struct {
	TargetRef        string                        `json:"targetRef"`
	Revision         int                           `json:"revision"`
	BindingSHA256    string                        `json:"bindingSHA256"`
	Storage          []topology.StorageRequirement `json:"storage"`
	ExecutionEnabled bool                          `json:"executionEnabled"`
}

func (s Service) PreviewProvision(ctx context.Context, actor Actor, targets Targets, network string, revision int, spec domain.NetworkSpec) (ProvisionPreview, error) {
	spec = domain.WithDefaults(spec)
	if spec.Spec.Deployment.Provider != domain.CustomerKubernetesProvider || !idPattern.MatchString(network) || revision < 1 || spec.Spec.Configuration == nil || spec.Spec.Ordering.Consensus != "raft" {
		return ProvisionPreview{}, ErrInvalid
	}
	if err := s.AllowProject(ctx, actor); err != nil {
		return ProvisionPreview{}, err
	}
	var approved Target
	for _, target := range targets {
		if target.Tenant == actor.Tenant && target.Project == actor.Project && target.Ref == spec.Spec.Deployment.TargetRef {
			if approved.Ref != "" || !target.valid() {
				return ProvisionPreview{}, ErrDenied
			}
			approved = target
		}
	}
	if approved.Ref == "" {
		return ProvisionPreview{}, ErrDenied
	}
	if report := topology.Validate(topology.Contract{Network: spec, Settings: *spec.Spec.Configuration}); !report.Valid {
		return ProvisionPreview{}, ErrInvalid
	}
	storage, err := topology.CheckCustomerStorage(spec, approved.Storage)
	if err != nil {
		return ProvisionPreview{}, ErrInvalid
	}
	binding := digest(struct {
		Target   Target
		Network  string
		Revision int
		Spec     domain.NetworkSpec
	}{approved, network, revision, spec})
	return ProvisionPreview{TargetRef: approved.Ref, Revision: revision, BindingSHA256: binding, Storage: storage}, nil
}
