package drafts

import (
	"time"

	"sathonate.local/fabric-control-plane/internal/domain"
	"sathonate.local/fabric-control-plane/internal/planner"
)

// BuildDraft is shared by local and PostgreSQL repositories so admission/revision behavior cannot diverge.
func BuildDraft(spec domain.NetworkSpec, previous *Draft, revision int) (Draft, error) {
	spec = domain.WithDefaults(spec)
	issues := domain.Validate(spec)
	if domain.HasErrors(issues) {
		return Draft{}, ErrInvalid
	}
	now := time.Now().UTC()
	kind, message := "draft_created", "Network draft saved. No infrastructure was deployed."
	d := Draft{ID: id(), Revision: 1, CreatedAt: now, Events: []Event{}}
	if previous != nil {
		if previous.Archived || previous.Revision != revision {
			return Draft{}, ErrConflict
		}
		if previous.Desired.Metadata.ID != spec.Metadata.ID {
			return Draft{}, ErrIdentity
		}
		d = clone(*previous)
		d.Revision++
		kind = "draft_updated"
		message = "Network configuration updated. Previous review cleared."
	}
	d.Desired = clone(spec)
	d.Issues = issues
	d.Status = "draft"
	d.Plan = nil
	d.UpdatedAt = now
	d.Events = append(d.Events, Event{ID: id(), Kind: kind, Message: message, At: now, Revision: d.Revision})
	return d, nil
}
func BuildReview(previous Draft, revision int) (Draft, error) {
	if previous.Archived || previous.Revision != revision {
		return Draft{}, ErrConflict
	}
	d := clone(previous)
	p, e := planner.Create(d.Desired)
	if e != nil {
		return Draft{}, ErrInvalid
	}
	if d.Plan != nil && d.Plan.ID == p.ID {
		return d, nil
	}
	d.Plan = &p
	d.Status = "reviewed"
	d.UpdatedAt = time.Now().UTC()
	d.Events = append(d.Events, Event{ID: id(), Kind: "plan_reviewed", Message: "Deployment plan generated for review. Execution is unavailable.", At: d.UpdatedAt, Revision: d.Revision})
	return d, nil
}
func BuildArchive(previous Draft, revision int) (Draft, error) {
	if previous.Revision != revision {
		return Draft{}, ErrConflict
	}
	d := clone(previous)
	if d.Archived {
		return d, nil
	}
	d.Archived = true
	d.Revision++
	d.UpdatedAt = time.Now().UTC()
	d.Events = append(d.Events, Event{ID: id(), Kind: "draft_archived", Message: "Draft archived. No infrastructure was changed.", At: d.UpdatedAt, Revision: d.Revision})
	return d, nil
}
