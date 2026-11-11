package customer

import (
	"context"
	"regexp"
	"time"

	"sathonate.local/fabric-control-plane/internal/identity"
)

var operationID = regexp.MustCompile(`^op_[a-f0-9]{32}$`)
var transactionID = regexp.MustCompile(`^[a-f0-9]{64}$`)

type VerifiedOperation struct {
	ID, Scope, Mode, Status, TransactionID string
	Simulated, Verified, Ready             bool
	FinishedAt                             time.Time
}

// RecordVerifiedOperation only projects durable, verified customer-platform
// operations. Historical local, EKS qualification and mock modes are excluded.
func (s Service) RecordVerifiedOperation(ctx context.Context, tenant, project string, receipt VerifiedOperation) error {
	scope := (identity.Principal{TenantID: tenant, ProjectID: project}).StorageTenant()
	if !operationID.MatchString(receipt.ID) || receipt.Scope != scope || receipt.Mode != "customer-kubernetes" ||
		receipt.Status != "succeeded" || !transactionID.MatchString(receipt.TransactionID) ||
		receipt.Simulated || !receipt.Ready || !receipt.Verified {
		return ErrDenied
	}
	return s.recordUsage(ctx, tenant, UsageReceipt{receipt.ID, project, "operation-verified", receipt.FinishedAt})
}
