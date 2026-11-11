package customer

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func validIssuer(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && parsed.Scheme == "https" && parsed.Hostname() != "" && parsed.User == nil && parsed.RawQuery == "" && parsed.Fragment == "" && len(value) <= 512
}

type PaymentEvent struct {
	ID          string    `json:"id"`
	Tenant      string    `json:"tenant"`
	Customer    string    `json:"customer"`
	At          time.Time `json:"at"`
	Type        string    `json:"type"`
	Plan        string    `json:"plan,omitempty"`
	Until       time.Time `json:"until,omitempty"`
	InvoiceID   string    `json:"invoiceId,omitempty"`
	Currency    string    `json:"currency,omitempty"`
	AmountMinor int64     `json:"amountMinor,omitempty"`
}

type PaymentVerifier interface {
	Verify(signature string, body []byte, now time.Time) (PaymentEvent, error)
}

// FixtureVerifier implements a signed provider contract for controlled tests.
// A real payment provider requires a separate implementation of its protocol.
type FixtureVerifier struct{ Key []byte }

func (f FixtureVerifier) Verify(signature string, body []byte, now time.Time) (PaymentEvent, error) {
	var event PaymentEvent
	if len(f.Key) < 32 || len(body) == 0 || len(body) > 16384 || len(signature) > 128 {
		return event, ErrDenied
	}
	timestamp, encoded, ok := strings.Cut(signature, ".")
	delivered, err := strconv.ParseInt(timestamp, 10, 64)
	if !ok || err != nil || len(encoded) != 64 || time.Unix(delivered, 0).Before(now.Add(-5*time.Minute)) || time.Unix(delivered, 0).After(now.Add(time.Minute)) {
		return event, ErrDenied
	}
	actual, err := hex.DecodeString(encoded)
	if err != nil {
		return event, ErrDenied
	}
	mac := hmac.New(sha256.New, f.Key)
	mac.Write([]byte(timestamp + "."))
	mac.Write(body)
	if !hmac.Equal(actual, mac.Sum(nil)) {
		return event, ErrDenied
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var extra any
	if decoder.Decode(&event) != nil || decoder.Decode(&extra) != io.EOF ||
		event.At.IsZero() || event.At.After(now.Add(time.Minute)) {
		return PaymentEvent{}, ErrDenied
	}
	return event, nil
}

func (s Service) ApplyPayment(ctx context.Context, verifier PaymentVerifier, signature string, body []byte) error {
	if verifier == nil {
		return ErrUnavailable
	}
	now := s.now()
	event, err := verifier.Verify(signature, body, now)
	if err != nil {
		return ErrDenied
	}
	if !idPattern.MatchString(event.ID) || !idPattern.MatchString(event.Tenant) || !idPattern.MatchString(event.Customer) ||
		event.At.IsZero() || event.At.After(now.Add(time.Minute)) {
		return ErrInvalid
	}
	change := Change{ID: "payment:" + event.ID, Action: event.Type, Digest: digest(event), ActorSHA256: hash([]byte("provider:" + event.Customer)), At: now}
	return s.Store.Update(ctx, event.Tenant, change, func(account *Account) error {
		if account.ProviderRef == "" || account.ProviderRef != event.Customer {
			return ErrDenied
		}
		switch event.Type {
		case "subscription.active", "subscription.past_due", "subscription.canceled":
			if event.InvoiceID != "" || event.Currency != "" || event.AmountMinor != 0 {
				return ErrInvalid
			}
			if !event.At.After(account.Subscription.ChangedAt) {
				return ErrConflict
			}
			status := event.Type[len("subscription."):]
			if status == "active" {
				plan, ok := planFor(event.Plan)
				if !ok || plan.MonthlyMinor <= 0 || !event.Until.After(event.At) || event.Until.After(event.At.Add(370*24*time.Hour)) {
					return ErrInvalid
				}
				account.Subscription = Subscription{event.Plan, status, event.Until, event.At}
			} else {
				if event.Plan != "" || !event.Until.IsZero() || account.Subscription.Plan == "" {
					return ErrInvalid
				}
				account.Subscription.Status = status
				account.Subscription.ChangedAt = event.At
			}
		case "invoice.open", "invoice.paid", "invoice.void":
			if !idPattern.MatchString(event.InvoiceID) || event.Currency != "USD" || event.AmountMinor < 0 || event.AmountMinor > 100000000 ||
				event.Plan != "" || !event.Until.IsZero() {
				return ErrInvalid
			}
			status := event.Type[len("invoice."):]
			for i, invoice := range account.Invoices {
				if invoice.ID != event.InvoiceID {
					continue
				}
				if invoice.Status != "open" || status == "open" || invoice.AmountMinor != event.AmountMinor ||
					invoice.Currency != event.Currency || !event.At.After(invoice.ChangedAt) {
					return ErrConflict
				}
				account.Invoices[i].Status = status
				account.Invoices[i].ChangedAt = event.At
				return nil
			}
			if status != "open" {
				return ErrConflict
			}
			account.Invoices = append(account.Invoices, Invoice{event.InvoiceID, event.Currency, event.AmountMinor, status, event.At, event.At})
		default:
			return ErrInvalid
		}
		return nil
	})
}

type UsageReceipt struct {
	ID, Project, Kind string
	At                time.Time
}
type UsagePeriod struct {
	Month              string `json:"month"`
	VerifiedOperations int64  `json:"verifiedOperations"`
}

// recordUsage is a server-only projection of acknowledged platform operations.
// No end-user input, cloud compute prices, instance counts or storage fees enter it.
func (s Service) recordUsage(ctx context.Context, tenant string, receipt UsageReceipt) error {
	now := s.now()
	if !idPattern.MatchString(tenant) || !idPattern.MatchString(receipt.ID) || !idPattern.MatchString(receipt.Project) ||
		receipt.Kind != "operation-verified" || receipt.At.IsZero() || receipt.At.After(now) || receipt.At.Before(now.Add(-31*24*time.Hour)) {
		return ErrInvalid
	}
	change := Change{ID: "usage:" + receipt.ID, Action: "usage.recorded", Digest: digest(receipt), ActorSHA256: hash([]byte("platform-meter")), At: now}
	return s.Store.Update(ctx, tenant, change, func(account *Account) error {
		for _, project := range account.Projects {
			if project.ID != receipt.Project || project.State != "active" {
				continue
			}
			month := receipt.At.UTC().Format("2006-01")
			for i, period := range account.Usage {
				if period.Month == month {
					if period.VerifiedOperations >= 1<<62 {
						return ErrConflict
					}
					account.Usage[i].VerifiedOperations++
					return nil
				}
			}
			account.Usage = append(account.Usage, UsagePeriod{month, 1})
			return nil
		}
		return ErrDenied
	})
}

func (s Service) ApproveProject(ctx context.Context, tenant, project, approval string) error {
	if !idPattern.MatchString(tenant) || !idPattern.MatchString(project) || !idPattern.MatchString(approval) {
		return ErrInvalid
	}
	now := s.now()
	change := Change{ID: "project-approval:" + approval, Action: "project.approved", Digest: hash([]byte(project)), ActorSHA256: hash([]byte("administrator")), At: now}
	return s.Store.Update(ctx, tenant, change, func(account *Account) error {
		if !entitled(account.Subscription, now) {
			return ErrDenied
		}
		for i, p := range account.Projects {
			if p.ID == project && p.State == "awaiting-identity-approval" {
				account.Projects[i].State = "active"
				return nil
			}
		}
		return ErrConflict
	})
}
