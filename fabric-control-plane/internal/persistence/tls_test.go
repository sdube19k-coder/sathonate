package persistence

import (
	"crypto/tls"
	"github.com/jackc/pgx/v5/pgxpool"
	"testing"
)

func TestProductionPostgresTLSModes(t *testing.T) {
	for _, tc := range []struct {
		mode string
		want bool
	}{{"disable", false}, {"allow", false}, {"prefer", false}, {"require", false}, {"verify-ca", false}, {"verify-full", true}} {
		t.Run(tc.mode, func(t *testing.T) {
			cfg, err := pgxpool.ParseConfig("postgres://service@db.example/fabric?sslmode=" + tc.mode)
			if err != nil {
				t.Fatal(err)
			}
			if got := verifiedPostgresTLS(cfg); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}

}

func TestProductionPostgresDoesNotWeakenConfiguredTLS(t *testing.T) {
	cfg, err := pgxpool.ParseConfig("postgres://service@db.example/fabric?sslmode=verify-full")
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.TLSConfig.MinVersion = tls.VersionTLS13
	if !verifiedPostgresTLS(cfg) || cfg.ConnConfig.TLSConfig.MinVersion != tls.VersionTLS13 {
		t.Fatal("stronger configured TLS minimum was weakened")
	}
}
