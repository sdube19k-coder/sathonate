// Package maintenance provides bound, secret-free lifecycle records. There is no
// production credential mutation adapter; plans and simulated tests are not rotation.
package maintenance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"time"

	"sathonate.local/fabric-control-plane/internal/identity"
)

var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
var referencePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}/[a-z][a-z0-9-]{0,62}$`)
var ErrDenied = errors.New("maintenance authorization or binding denied")
var ErrConflict = errors.New("maintenance revision or idempotency conflict")

func hash(v any) string {
	raw, _ := json.Marshal(v)
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}
func HashBytes(v []byte) string { h := sha256.Sum256(v); return hex.EncodeToString(h[:]) }

type Binding struct {
	TenantID      string `json:"tenantId"`
	NetworkID     string `json:"networkId"`
	TargetSHA256  string `json:"targetSHA256"`
	NetworkSHA256 string `json:"networkSHA256"`
}

func (b Binding) valid() bool {
	return regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,127}$`).MatchString(b.TenantID) && regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,127}$`).MatchString(b.NetworkID) && digestPattern.MatchString(b.TargetSHA256) && digestPattern.MatchString(b.NetworkSHA256)
}
func (b Binding) Digest() string { return hash(b) }

var steps = []string{"inventory", "backup", "replacement-enrollment", "replacement-verification", "activation", "active-verification", "retirement"}

type Checkpoint struct {
	Step           string    `json:"step"`
	ArtifactSHA256 string    `json:"artifactSHA256"`
	At             time.Time `json:"at"`
	Simulated      bool      `json:"simulated"`
}
type Intent struct {
	ID                string       `json:"id"`
	Binding           Binding      `json:"binding"`
	IdentityRef       string       `json:"identityRef"`
	BindingSHA256     string       `json:"bindingSHA256"`
	Revision          int          `json:"revision"`
	Phase             string       `json:"phase"`
	CreatedAt         time.Time    `json:"createdAt"`
	ActorSHA256       string       `json:"actorSHA256"`
	Simulated         bool         `json:"simulated"`
	Checkpoints       []Checkpoint `json:"checkpoints"`
	KeySHA256         string       `json:"-"`
	LastRequestSHA256 string       `json:"lastRequestSHA256,omitempty"`
}

func NewIntent(p identity.Principal, b Binding, ref, key string, now time.Time) (Intent, error) {
	if !p.CanAdminister() || p.TenantID != b.TenantID || !b.valid() || !referencePattern.MatchString(ref) || len(key) < 8 || len(key) > 128 {
		return Intent{}, ErrDenied
	}
	binding := hash([]string{b.Digest(), ref})
	keyHash := hash(key)
	return Intent{ID: "maint_" + hash([]string{b.TenantID, keyHash})[:32], Binding: b, IdentityRef: ref, BindingSHA256: binding, Revision: 1, Phase: "planned", CreatedAt: now.UTC(), ActorSHA256: hash(p.Subject), Checkpoints: []Checkpoint{}, KeySHA256: keyHash}, nil
}

type StepApproval struct {
	IntentID, BindingSHA256, Step, AuthorizationRef string
	ExpiresAt                                       time.Time
}
type Receipt struct {
	BindingSHA256, Step, ArtifactSHA256 string
	Verified                            bool
	Simulated                           bool
}

// Reconcile MUST use operationKey to adopt interrupted work; never repeat an
// external rotation blindly. No implementation is registered in the API.
type Driver interface {
	Reconcile(context.Context, Intent, string, string) (Receipt, error)
}

func Advance(ctx context.Context, p identity.Principal, current Intent, revision int, key string, approval StepApproval, driver Driver, now time.Time) (Intent, error) {
	if !p.CanAdminister() || p.TenantID != current.Binding.TenantID || !current.Binding.valid() || current.BindingSHA256 != hash([]string{current.Binding.Digest(), current.IdentityRef}) {
		return current, ErrDenied
	}
	if revision != current.Revision {
		if current.LastRequestSHA256 == hash(key) && revision == current.Revision-1 {
			return current, nil
		}
		return current, ErrConflict
	}
	if len(key) < 8 || len(key) > 128 || len(current.Checkpoints) >= len(steps) {
		return current, ErrConflict
	}
	step := steps[len(current.Checkpoints)]
	if approval.IntentID != current.ID || approval.BindingSHA256 != current.BindingSHA256 || approval.Step != step || !regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`).MatchString(approval.AuthorizationRef) || !approval.ExpiresAt.After(now) || approval.ExpiresAt.After(now.Add(time.Hour)) {
		return current, ErrDenied
	}
	if driver == nil {
		return current, errors.New("live maintenance adapter is not configured; no identity was changed")
	}
	receipt, err := driver.Reconcile(ctx, current, step, current.ID+"/"+step)
	if err != nil {
		return current, errors.New("maintenance step failed; resume through the same bound operation")
	}
	if !receipt.Verified || receipt.BindingSHA256 != current.BindingSHA256 || receipt.Step != step || !digestPattern.MatchString(receipt.ArtifactSHA256) {
		return current, errors.New("maintenance verification receipt invalid; retirement is forbidden")
	}
	if len(current.Checkpoints) > 0 && current.Simulated != receipt.Simulated {
		return current, errors.New("simulated and real receipts cannot be mixed")
	}
	next := current
	next.Revision++
	next.Phase = step
	next.Simulated = receipt.Simulated
	next.LastRequestSHA256 = hash(key)
	next.Checkpoints = append(append([]Checkpoint{}, current.Checkpoints...), Checkpoint{step, receipt.ArtifactSHA256, now.UTC(), receipt.Simulated})
	return next, nil
}
