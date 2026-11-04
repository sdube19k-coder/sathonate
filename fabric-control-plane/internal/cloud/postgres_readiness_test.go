package cloud

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestDatabaseReadinessRejectsUnsafeConfigWithoutConnecting(t *testing.T) {
	for _, mode := range []string{"disable", "prefer", "allow", "require", "verify-ca"} {
		connection := "postgres://person:secret-canary@127.0.0.1/db?sslmode=" + mode
		check := PostgreSQLReadiness(context.Background(), connection)
		raw, _ := json.Marshal(check)
		if check.Status != "BLOCKED" || strings.Contains(string(raw), "secret-canary") || strings.Contains(string(raw), "postgres://") {
			t.Fatal("unsafe TLS accepted or credentials exposed", mode)
		}
	}
	if c := PostgreSQLReadiness(context.Background(), ""); c.Status != "NOT VERIFIED" {
		t.Fatal("missing DB claimed verified")
	}
	if _, err := securePostgresConfig("postgres://person:secret-canary@database.example/db?sslmode=verify-full"); err != nil {
		t.Fatal("verified TLS configuration rejected")
	}
}
