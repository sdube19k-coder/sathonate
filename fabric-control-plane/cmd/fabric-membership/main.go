// Explicit development-only administrative membership registration. This is not
// an HTTP API, cannot create projects, and never resolves identity from email.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"sathonate.local/fabric-control-plane/internal/identity"
	"sathonate.local/fabric-control-plane/internal/persistence"
	"time"
)

func main() {
	mode := flag.String("mode", "", "must explicitly be local-development")
	path := flag.String("grant-file", "", "protected JSON record containing the previously verified issuer/subject and approved scope")
	network := flag.String("network-id", "", "existing approved development network ID")
	flag.Parse()
	fail := func() {
		fmt.Fprintln(os.Stderr, "Development membership was not granted; verify explicit mode, protected input, existing scope ownership and administrative database access")
		os.Exit(1)
	}
	if *mode != "local-development" || *path == "" || *network == "" {
		fail()
	}
	connection := os.Getenv("DATABASE_URL")
	u, err := url.Parse(connection)
	if err != nil || u.Scheme != "postgres" || (u.Hostname() != "127.0.0.1" && u.Hostname() != "::1") || u.Path != "/fabric_smp_tests" {
		fail()
	}
	file, err := os.Open(*path)
	if err != nil {
		fail()
	}
	defer file.Close()
	var g identity.Membership
	decoder := json.NewDecoder(io.LimitReader(file, 1<<20))
	decoder.DisallowUnknownFields()
	var extra any
	if decoder.Decode(&g) != nil || decoder.Decode(&extra) != io.EOF {
		fail()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	db, err := persistence.Open(ctx, connection, false)
	if err != nil {
		fail()
	}
	defer db.Close()
	if err = db.GrantDevelopmentReadOnly(ctx, g, *network); err != nil {
		fail()
	}
	sum := sha256.Sum256([]byte(g.Issuer + "\x00" + g.Subject))
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"developmentOnly": true, "permissions": []string{"read"}, "issuerSubjectSHA256": hex.EncodeToString(sum[:]), "tenant": g.Tenant, "project": g.Project, "expiresAt": g.ExpiresAt})
}
