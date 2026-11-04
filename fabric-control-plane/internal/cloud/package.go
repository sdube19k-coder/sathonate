package cloud

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"sathonate.local/fabric-control-plane/internal/domain"
)

// Package is a revision-bound handoff, not a claim of deployed infrastructure.
type Package struct {
	APIVersion   string             `json:"apiVersion"`
	Revision     int                `json:"revision"`
	Desired      domain.NetworkSpec `json:"network"`
	Digest       string             `json:"networkSHA256"`
	Executable   bool               `json:"executable"`
	Checks       []Check            `json:"checks"`
	OperatorPlan *OperatorPlan      `json:"operatorPlan,omitempty"`
	Bundle       *Bundle            `json:"bevelBundle,omitempty"`
	Manifests    map[string]any     `json:"kubernetesManifests,omitempty"`
}

func Prepare(s domain.NetworkSpec, revision int, tenant string, registry Registry) (Package, error) {
	s = domain.WithDefaults(s)
	if domain.HasErrors(domain.Validate(s)) || revision < 1 {
		return Package{}, fmt.Errorf("invalid network or revision")
	}
	if s.Spec.Deployment.Engine != "bevel" {
		return Package{}, fmt.Errorf("deployment packages require the Bevel cloud engine")
	}
	raw, err := json.Marshal(s)
	if err != nil {
		return Package{}, err
	}
	hash := sha256.Sum256(raw)
	p := Package{APIVersion: "deployment.sathonate.io/v1alpha1", Revision: revision, Desired: s, Digest: hex.EncodeToString(hash[:]), Checks: []Check{}}
	target, ok := registry.Find(tenant, s.Spec.Deployment.TargetRef)
	if !ok {
		p.Checks = append(p.Checks, Check{"registered-target", "blocked", "Register the selected cluster and scoped identity for this tenant"})
		return p, nil
	}
	if target.Operator != nil {
		plan, err := CompileOperator(s, target, target.RunnerNamespace)
		if err != nil {
			p.Checks = append(p.Checks, Check{"operator-contract", "blocked", err.Error()})
			return p, nil
		}
		p.OperatorPlan = &plan
		p.Checks = append(p.Checks, Check{"eks-qualification", "blocked", "Operator plan compiled; exact network namespace binding, administrator approval and live preflight are required by the worker. Production remains disabled."})
		return p, nil
	}
	p.Checks = StaticChecks(s, target)
	bundle, err := Compile(s, target)
	if err != nil {
		p.Checks = append(p.Checks, Check{"bevel-compile", "blocked", err.Error()})
		return p, nil
	}
	p.Bundle = &bundle
	p.Manifests, err = Manifests(bundle, target)
	if err != nil {
		return Package{}, err
	}
	p.Checks = append(p.Checks, Check{"execution-release", "blocked", "Candidate compiled. Cloud runner qualification and live target verification are required before execution"})
	return p, nil
}
