package customer

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"regexp"
	"time"
)

var (
	ErrDenied      = errors.New("customer operation denied")
	ErrConflict    = errors.New("customer operation conflicts with current state")
	ErrInvalid     = errors.New("invalid customer operation")
	ErrUnavailable = errors.New("customer operation unavailable")
	idPattern      = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,127}$`)
)

type Actor struct{ Issuer, Subject, Tenant, Project string }
type Identity struct {
	Issuer  string `json:"issuer"`
	Subject string `json:"subject"`
}
type Plan struct {
	ID             string `json:"id"`
	MonthlyMinor   int64  `json:"monthlyMinor"`
	Currency       string `json:"currency"`
	ActiveProjects int    `json:"activeProjects"`
}

func Plans() []Plan { return []Plan{{"pilot", 0, "USD", 3}, {"team", 4900, "USD", 25}} }
func planFor(id string) (Plan, bool) {
	for _, plan := range Plans() {
		if plan.ID == id {
			return plan, true
		}
	}
	return Plan{}, false
}

type Subscription struct {
	Plan      string    `json:"plan"`
	Status    string    `json:"status"`
	Until     time.Time `json:"until"`
	ChangedAt time.Time `json:"changedAt"`
}
type Project struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	State string `json:"state"`
}
type Invitation struct {
	ID        string    `json:"id"`
	Recipient Identity  `json:"recipient"`
	ExpiresAt time.Time `json:"expiresAt"`
	State     string    `json:"state"`
}
type Invoice struct {
	ID          string    `json:"id"`
	Currency    string    `json:"currency"`
	AmountMinor int64     `json:"amountMinor"`
	Status      string    `json:"status"`
	IssuedAt    time.Time `json:"issuedAt"`
	ChangedAt   time.Time `json:"changedAt"`
}
type Account struct {
	Usage        []UsagePeriod `json:"usage"`
	Name         string        `json:"name"`
	Owner        Identity      `json:"owner"`
	Members      []Identity    `json:"members"`
	Invitations  []Invitation  `json:"invitations"`
	Projects     []Project     `json:"projects"`
	Subscription Subscription  `json:"subscription"`
	Invoices     []Invoice     `json:"invoices"`
	// Reference is opaque and provisioned by a separate trusted operator.
	ProviderRef string `json:"providerRef"`
}
type View struct {
	Usage              []UsagePeriod `json:"usage,omitempty"`
	Name               string        `json:"name"`
	IsOwner            bool          `json:"isOwner"`
	Projects           []Project     `json:"projects"`
	Subscription       Subscription  `json:"subscription"`
	Invoices           []Invoice     `json:"invoices"`
	ActiveEntitlement  bool          `json:"activeEntitlement"`
	Invitations        []Invitation  `json:"invitations"`
	PendingInvitations []Invitation  `json:"pendingInvitations"`
}

type Change struct {
	ID, Action, Digest string
	ActorSHA256        string
	At                 time.Time
}

type Store interface {
	Read(context.Context, string) (Account, error)
	// Update must atomically reserve a change ID, apply the mutation and append
	// its audit record. Same ID/different digest must fail.
	Update(context.Context, string, Change, func(*Account) error) error
}

type Service struct {
	Store Store
	Now   func() time.Time
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
func (a Actor) identity() Identity { return Identity{a.Issuer, a.Subject} }
func (a Actor) valid() bool {
	return validIssuer(a.Issuer) && a.Subject != "" && len(a.Issuer) <= 512 && len(a.Subject) <= 255 && idPattern.MatchString(a.Tenant) && idPattern.MatchString(a.Project)
}
func newID() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", ErrUnavailable
	}
	return hex.EncodeToString(bytes[:]), nil
}
func entitled(subscription Subscription, now time.Time) bool {
	_, known := planFor(subscription.Plan)
	return known && (subscription.Status == "trialing" || subscription.Status == "active") && now.Before(subscription.Until)
}
func member(account Account, actor Identity) bool {
	if account.Owner == actor {
		return true
	}
	for _, person := range account.Members {
		if person == actor {
			return true
		}
	}
	return false
}
func (s Service) View(ctx context.Context, actor Actor) (View, error) {
	if !actor.valid() {
		return View{}, ErrDenied
	}
	account, err := s.Store.Read(ctx, actor.Tenant)
	if err != nil {
		return View{}, err
	}
	result := View{Projects: []Project{}, Invoices: []Invoice{}, Invitations: []Invitation{}, PendingInvitations: []Invitation{}}
	for _, invitation := range account.Invitations {
		if invitation.Recipient == actor.identity() && invitation.State == "pending" && s.now().Before(invitation.ExpiresAt) {
			result.PendingInvitations = append(result.PendingInvitations, invitation)
		}
	}
	if !member(account, actor.identity()) {
		if len(result.PendingInvitations) > 0 {
			return result, nil
		}
		return View{}, ErrDenied
	}
	result.Name = account.Name
	result.IsOwner = account.Owner == actor.identity()
	if result.IsOwner {
		result.Projects = append(result.Projects, account.Projects...)
		result.Invoices = append(result.Invoices, account.Invoices...)
		result.Usage = append(result.Usage, account.Usage...)
		result.Invitations = append(result.Invitations, account.Invitations...)
	} else {
		for _, p := range account.Projects {
			if p.ID == actor.Project {
				result.Projects = append(result.Projects, p)
			}
		}
	}
	result.Subscription = account.Subscription
	result.ActiveEntitlement = entitled(account.Subscription, s.now())
	return result, nil
}
