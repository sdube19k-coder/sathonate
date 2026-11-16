// Local administrator tool for the disposable Sprint 3 project.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"time"

	"sathonate.local/fabric-control-plane/internal/domain"
	"sathonate.local/fabric-control-plane/internal/identity"
	"sathonate.local/fabric-control-plane/internal/persistence"
)

func main() {
	issuer := flag.String("verified-issuer", "", "issuer verified by local console")
	subject := flag.String("verified-subject", "", "subject verified by local console")
	flag.Parse()
	const tenant, project = "sathonate-sprint3-qualification", "disposable-eks"
	connection := os.Getenv("DATABASE_URL")
	u, err := url.Parse(connection)
	if err != nil || u.Scheme != "postgres" || u.Hostname() != "127.0.0.1" ||
		u.Port() != "15432" || u.Path != "/fabric_smp_tests" ||
		*issuer != "https://integrator-5135965.okta.com/oauth2/default" || *subject == "" {
		log.Fatal("local test database and verified development identity required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	db, err := persistence.Open(ctx, connection, false)
	if err != nil {
		log.Fatal("local test database unavailable")
	}
	defer db.Close()
	var verified bool
	err = db.Pool().QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM sathonate_oidc_memberships WHERE environment='local-development'
		AND issuer=$1 AND subject=$2 AND tenant_id='smp-sprint1-1543ce01'
		AND project_id='project-a' AND role='viewer')`, *issuer, *subject).Scan(&verified)
	if err != nil || !verified {
		log.Fatal("previously approved development identity required")
	}
	raw, err := os.ReadFile("examples/sprint3-protected-eks-network.json")
	var spec domain.NetworkSpec
	if err != nil || json.Unmarshal(raw, &spec) != nil || spec.Spec.Deployment.TargetRef != domain.SecureEKSProfile {
		log.Fatal("prepared Sprint 3 design unavailable")
	}
	scope := (identity.Principal{TenantID: tenant, ProjectID: project}).StorageTenant()
	repo := db.ForTenant(ctx, scope)
	existing, err := repo.List()
	if err != nil || len(existing) > 1 {
		log.Fatal("unexpected disposable project state")
	}
	id := ""
	if len(existing) == 1 {
		expected, _ := json.Marshal(domain.WithDefaults(spec))
		actual, _ := json.Marshal(existing[0].Desired)
		if string(expected) != string(actual) || existing[0].Archived {
			log.Fatal("existing design differs from the prepared design")
		}
		id = existing[0].ID
	} else {
		saved, err := repo.Save(spec, "", 0)
		if err != nil {
			log.Fatal("unable to save disposable design")
		}
		id = saved.ID
		spec = saved.Desired
	}
	expires := time.Now().UTC().Add(4 * time.Hour)
	_, err = db.Pool().Exec(ctx, `INSERT INTO sathonate_oidc_memberships
	 (environment,issuer,subject,tenant_id,project_id,storage_scope,anchor_network_id,role,permissions,not_before,expires_at)
	 VALUES ('local-development',$1,$2,$3,$4,$5,$6,'operator','["read","deploy"]',now(),$7)
	 ON CONFLICT(environment,issuer,subject,tenant_id,project_id) DO UPDATE SET
	 not_before=EXCLUDED.not_before, expires_at=EXCLUDED.expires_at
	 WHERE sathonate_oidc_memberships.anchor_network_id=EXCLUDED.anchor_network_id
	 AND sathonate_oidc_memberships.role=EXCLUDED.role
	 AND sathonate_oidc_memberships.permissions=EXCLUDED.permissions
	 RETURNING anchor_network_id`, *issuer, *subject, tenant, project, scope, id, expires)
	if err != nil {
		log.Fatal("local disposable grant failed")
	}
	canonical, _ := json.Marshal(domain.WithDefaults(spec))
	fmt.Printf("Project %s, network %s, storage scope %s, SHA256 %x, expires %s\n",
		project, id, scope, sha256.Sum256(canonical), expires.Format(time.RFC3339))
}
