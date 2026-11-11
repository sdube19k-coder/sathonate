package main

import (
	"context"
	"flag"
	"log"
	"os"
	"time"

	"sathonate.local/fabric-control-plane/internal/customer"
	"sathonate.local/fabric-control-plane/internal/deployment"
	"sathonate.local/fabric-control-plane/internal/identity"
	"sathonate.local/fabric-control-plane/internal/persistence"
)

func main() {
	action := flag.String("action", "", "bootstrap, approve-project or record-usage")
	tenant := flag.String("tenant", "", "existing verified tenant ID")
	name := flag.String("name", "", "customer organization name")
	issuer := flag.String("issuer", "", "verified owner issuer")
	subject := flag.String("subject", "", "verified owner subject")
	project := flag.String("project", "", "customer project ID")
	approval := flag.String("approval", "", "external administrative approval reference")
	operation := flag.String("operation", "", "verified customer operation ID")
	flag.Parse()
	if (*action != "bootstrap" && *action != "approve-project" && *action != "record-usage") || *project == "" || *issuer == "" || *subject == "" {
		log.Fatal("customer administrative action required")
	}
	connection := os.Getenv("CUSTOMER_ADMIN_DATABASE_URL")
	if connection == "" {
		log.Fatal("administrator PostgreSQL connection required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	// This separate admin account must be restricted by explicit database grants.
	// Enforce TLS using a direct driver configuration check.
	db, err := persistence.Open(ctx, connection, true)
	if err != nil {
		log.Fatal("cannot open verified PostgreSQL connection with a restricted administrator role")
	}
	defer db.Close()
	store := customer.Postgres{Pool: db.Pool()}
	if store.RequireAdministrator(ctx) != nil {
		log.Fatal("separate customer administrator role required")
	}
	source := persistence.MembershipSource{Pool: db.Pool()}
	grants, err := source.Lookup(ctx, *issuer, *subject, "production")
	if err != nil || !approvedGrant(grants, *issuer, *subject, *tenant, *project, time.Now()) {
		log.Fatal("current server-owned administrator project grant is required")
	}
	switch *action {
	case "bootstrap":
		err = store.Bootstrap(ctx, *tenant, *name, customer.Identity{Issuer: *issuer, Subject: *subject}, "")
	case "approve-project":
		account, readErr := store.Read(ctx, *tenant)
		if readErr != nil || account.Owner != (customer.Identity{Issuer: *issuer, Subject: *subject}) {
			log.Fatal("approval grant does not match the account owner")
		}
		err = (customer.Service{Store: store}).ApproveProject(ctx, *tenant, *project, *approval)
	case "record-usage":
		scope := (identity.Principal{TenantID: *tenant, ProjectID: *project}).StorageTenant()
		history, openErr := deployment.OpenHistoryDatabase(ctx, db.Pool(), []string{scope})
		if openErr != nil {
			log.Fatal("qualified customer operation history unavailable")
		}
		defer history.Close()
		job, jobErr := history.Operations().Get(scope, *operation)
		result, resultErr := history.Status(scope, *operation)
		if jobErr != nil || resultErr != nil || job.FinishedAt == nil || job.TenantID != scope ||
			result.OperationID != job.ID || result.Phase != "succeeded" {
			log.Fatal("qualified customer operation receipt unavailable")
		}
		receipt := customer.VerifiedOperation{ID: job.ID, Scope: job.TenantID, Mode: result.Mode, Status: job.Status, TransactionID: result.TransactionID,
			Simulated: result.Simulated, Verified: result.Verified, Ready: result.Ready, FinishedAt: *job.FinishedAt}
		err = (customer.Service{Store: store}).RecordVerifiedOperation(ctx, *tenant, *project, receipt)
	}
	if err != nil {
		log.Fatal("customer administrative action refused or unavailable")
	}
	log.Print("customer administrative action applied")
}

func approvedGrant(grants []identity.Membership, issuer, subject, tenant, project string, now time.Time) bool {
	for _, grant := range grants {
		if grant.Environment == "production" && grant.Issuer == issuer && grant.Subject == subject && grant.Tenant == tenant &&
			grant.Project == project && grant.Role == "admin" && !now.Before(grant.NotBefore) && now.Before(grant.ExpiresAt) {
			for _, permission := range grant.Permissions {
				if permission == "write" {
					return true
				}
			}
		}
	}
	return false
}
