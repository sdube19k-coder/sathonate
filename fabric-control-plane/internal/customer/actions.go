package customer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
)

func hash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
func digest(value any) string { raw, _ := json.Marshal(value); return hash(raw) }

type Command struct {
	Action    string `json:"action"`
	Name      string `json:"name,omitempty"`
	Reference string `json:"reference,omitempty"`
}

func (s Service) Execute(ctx context.Context, actor Actor, key string, cmd Command) error {
	if !actor.valid() {
		return ErrDenied
	}
	if len(key) < 16 || !idPattern.MatchString(key) {
		return ErrInvalid
	}
	now := s.now()
	subject := actor.identity()
	actorHash := digest(subject)
	change := Change{ID: "actor:" + digest([]string{actorHash, key}), Action: cmd.Action, Digest: digest(cmd), ActorSHA256: actorHash, At: now}
	return s.Store.Update(ctx, actor.Tenant, change, func(account *Account) error {
		if cmd.Action == "accept-invitation" {
			if cmd.Name != "" || !idPattern.MatchString(cmd.Reference) {
				return ErrInvalid
			}
			for i, invitation := range account.Invitations {
				if invitation.ID != cmd.Reference || invitation.Recipient != subject {
					continue
				}
				if invitation.State != "pending" || !now.Before(invitation.ExpiresAt) {
					return ErrConflict
				}
				if !member(*account, subject) {
					account.Members = append(account.Members, subject)
				}
				account.Invitations[i].State = "accepted"
				return nil
			}
			return ErrDenied
		}
		if account.Owner != subject {
			return ErrDenied
		}
		switch cmd.Action {
		case "rename-organization":
			name := strings.TrimSpace(cmd.Name)
			if len(name) < 2 || len(name) > 100 || cmd.Reference != "" {
				return ErrInvalid
			}
			account.Name = name
		case "start-trial":
			if cmd.Name != "" || cmd.Reference != "" {
				return ErrInvalid
			}
			if account.Subscription.Status != "" {
				return ErrConflict
			}
			account.Subscription = Subscription{"pilot", "trialing", now.Add(14 * 24 * time.Hour), now}
		case "request-project":
			name := strings.TrimSpace(cmd.Name)
			if len(name) < 2 || len(name) > 100 || cmd.Reference != "" {
				return ErrInvalid
			}
			plan, _ := planFor(account.Subscription.Plan)
			if !entitled(account.Subscription, now) {
				return ErrDenied
			}
			if len(account.Projects) >= plan.ActiveProjects {
				return ErrConflict
			}
			id, err := newID()
			if err != nil {
				return err
			}
			account.Projects = append(account.Projects, Project{"project-" + id, name, "awaiting-identity-approval"})
		case "invite":
			if cmd.Name != "" || len(cmd.Reference) == 0 || len(cmd.Reference) > 255 || cmd.Reference == actor.Subject {
				return ErrInvalid
			}
			recipient := Identity{actor.Issuer, cmd.Reference}
			if member(*account, recipient) {
				return ErrConflict
			}
			invitations := account.Invitations[:0]
			for _, invite := range account.Invitations {
				if invite.State == "pending" && now.Before(invite.ExpiresAt) && invite.Recipient == recipient {
					return ErrConflict
				}
				if invite.State == "pending" && now.Before(invite.ExpiresAt) {
					invitations = append(invitations, invite)
				}
			}
			if len(invitations) >= 100 {
				return ErrConflict
			}
			id, err := newID()
			if err != nil {
				return err
			}
			account.Invitations = append(invitations, Invitation{id, recipient, now.Add(48 * time.Hour), "pending"})
		case "revoke-invitation":
			if cmd.Name != "" || !idPattern.MatchString(cmd.Reference) {
				return ErrInvalid
			}
			for i, invitation := range account.Invitations {
				if invitation.ID == cmd.Reference && invitation.State == "pending" {
					account.Invitations[i].State = "revoked"
					return nil
				}
			}
			return ErrConflict
		default:
			return ErrInvalid
		}
		return nil
	})
}

// AllowProject requires an independently authenticated membership plus the
// exact active customer project. It never grants Kubernetes execution.
func (s Service) AllowProject(ctx context.Context, actor Actor) error {
	if !actor.valid() {
		return ErrDenied
	}
	account, err := s.Store.Read(ctx, actor.Tenant)
	if err != nil {
		return err
	}
	if !member(account, actor.identity()) || !entitled(account.Subscription, s.now()) {
		return ErrDenied
	}
	for _, project := range account.Projects {
		if project.ID == actor.Project && project.State == "active" {
			return nil
		}
	}
	return ErrDenied
}
