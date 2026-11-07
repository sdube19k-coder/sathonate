// Package deployment owns the shared deployment operation and status contract.
package deployment

import (
	"context"
	"fmt"
	"time"

	"sathonate.local/fabric-control-plane/internal/domain"
	"sathonate.local/fabric-control-plane/internal/kubernetesfabric"
	"sathonate.local/fabric-control-plane/internal/localfabric"
	"sathonate.local/fabric-control-plane/internal/operations"
)

var Stages = []operations.Stage{
	{Key: "validate", Label: "Validate saved network JSON"},
	{Key: "identity", Label: "Resolve deployment identity"},
	{Key: "kubernetes", Label: "Prepare Kubernetes cluster"},
	{Key: "ca", Label: "Enroll Fabric CA identities"},
	{Key: "orderers", Label: "Deploy ordering service"},
	{Key: "peers", Label: "Deploy organization peers"},
	{Key: "channels", Label: "Create channels and join members"},
	{Key: "verify", Label: "Verify network and transaction"},
}

type Result struct {
	LocalDeletion     *LocalDeletion                         `json:"localDeletion,omitempty"`
	Recovery          *NodeRecovery                          `json:"recovery,omitempty"`
	PreviousNetwork   *domain.NetworkSpec                    `json:"previousNetwork,omitempty"`
	Verifications     []kubernetesfabric.DynamicVerification `json:"verifications,omitempty"`
	LifecycleApproval string                                 `json:"lifecycleApproval,omitempty"`
	LifecycleParent   string                                 `json:"lifecycleParent,omitempty"`
	LifecycleAction   string                                 `json:"lifecycleAction,omitempty"`
	Lifecycle         *kubernetesfabric.LifecycleState       `json:"lifecycle,omitempty"`
	Security          *kubernetesfabric.ArgumentEvidence     `json:"security,omitempty"`
	TenantID          string                                 `json:"-"`
	Mode              string                                 `json:"mode"`
	Simulated         bool                                   `json:"simulated"`
	Network           domain.NetworkSpec                     `json:"network"`
	Revision          int                                    `json:"revision"`
	OperationID       string                                 `json:"operationId"`
	TargetSHA256      string                                 `json:"targetSHA256,omitempty"`
	Phase             string                                 `json:"phase"`
	Ready             bool                                   `json:"ready"`
	Verified          bool                                   `json:"verified"`
	CheckedAt         time.Time                              `json:"checkedAt"`
	Resources         []Resource                             `json:"resources"`
	TransactionID     string                                 `json:"transactionId,omitempty"`
	Notice            string                                 `json:"notice"`
	Health            *localfabric.Health                    `json:"health,omitempty"`
}
type Resource struct {
	Name         string `json:"name"`
	Kind         string `json:"kind"`
	Organization string `json:"organization,omitempty"`
	Status       string `json:"status"`
}

// Runner is shared by isolated simulation and real local Kubernetes execution. Emission errors must
// stop execution, so an unrecorded stage can never be reported as completed.
type Runner interface {
	Mode() string
	Run(context.Context, Request, func(string, string) error) (Outcome, error)
}
type Request struct {
	Recovery                                       *NodeRecovery
	PreviousNetwork                                *domain.NetworkSpec
	LifecycleAction, LifecycleApproval             string
	Lifecycle                                      *kubernetesfabric.LifecycleState
	TargetSHA256                                   string
	TenantID, NetworkID, OperationID, FailureStage string
	Network                                        domain.NetworkSpec
}
type Outcome struct {
	Verifications []kubernetesfabric.DynamicVerification
	Security      *kubernetesfabric.ArgumentEvidence
	Resources     []Resource
	Health        *localfabric.Health
	TransactionID string
}
type MockRunner struct{ Delay time.Duration }

func (MockRunner) Mode() string { return "mock" }
func (m MockRunner) Run(ctx context.Context, request Request, emit func(string, string) error) (Outcome, error) {
	s, fail := request.Network, request.FailureStage
	for _, stage := range Stages {
		if err := emit(stage.Key, "[MOCK] Simulating: "+stage.Label+". No infrastructure command is executed."); err != nil {
			return Outcome{}, err
		}
		timer := time.NewTimer(m.Delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return Outcome{}, ctx.Err()
		case <-timer.C:
		}
		if stage.Key == fail {
			return Outcome{}, fmt.Errorf("[MOCK] Requested failure at %s; no real infrastructure was changed", fail)
		}
	}
	resources := []Resource{}
	for _, o := range s.Spec.Organizations {
		resources = append(resources, Resource{Name: o.IdentityProviderRef, Kind: "ca", Organization: o.ID, Status: "simulated-ready"})
	}
	for _, n := range s.Spec.Nodes {
		resources = append(resources, Resource{Name: n.ID, Kind: n.Role, Organization: n.OrganizationRef, Status: "simulated-ready"})
	}
	for _, c := range s.Spec.Channels {
		resources = append(resources, Resource{Name: c.ID, Kind: "channel", Status: "simulated-ready"})
	}
	return Outcome{Resources: resources}, nil
}
