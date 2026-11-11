package persistence

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Independent disposable PostgreSQL: never reconfigures the shared development
// database or puts passwords/private keys into argv, Docker env or diagnostics.
func TestProductionPostgresTLSAndRoleOnDocker(t *testing.T) {
	if os.Getenv("SMP_POSTGRES_TLS_E2E") != "1" {
		t.Skip("requires explicitly enabled disposable TLS PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	private := t.TempDir()
	write := func(name string, raw []byte) {
		t.Helper()
		if os.WriteFile(filepath.Join(private, name), raw, 0600) != nil {
			t.Fatal("protected fixture write")
		}
	}
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		t.Fatal("random credential generation")
	}
	password := hex.EncodeToString(random)
	write("password", []byte(password))
	caKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	serverKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	now := time.Now()
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "disposable-postgres-ca"}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal("fixture CA generation")
	}
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "localhost"}, DNSNames: []string{"localhost"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, NotBefore: ca.NotBefore, NotAfter: ca.NotAfter, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, leaf, ca, &serverKey.PublicKey, caKey)
	if err != nil {
		t.Fatal("fixture server certificate")
	}
	key, _ := x509.MarshalPKCS8PrivateKey(serverKey)
	write("ca.pem", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}))
	write("server.pem", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	write("server.key", pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key}))
	name := "smp-db-tls-" + hex.EncodeToString(random[:6])
	docker := func(args ...string) ([]byte, error) { return exec.CommandContext(ctx, "docker", args...).Output() }
	script := "install -o postgres -g postgres -m 600 /fixtures/server.key /tmp/server.key; install -o postgres -g postgres -m 644 /fixtures/server.pem /tmp/server.pem; exec docker-entrypoint.sh postgres -c ssl=on -c ssl_cert_file=/tmp/server.pem -c ssl_key_file=/tmp/server.key"
	if _, err := docker("run", "-d", "--pull", "never", "--name", name, "--mount", "type=bind,source="+private+",target=/fixtures,readonly", "-e", "POSTGRES_USER=fixture_admin", "-e", "POSTGRES_DB=fabric_tls_tests", "-e", "POSTGRES_PASSWORD_FILE=/fixtures/password", "-p", "127.0.0.1::5432", "postgres:17.6", "sh", "-ec", script); err != nil {
		t.Fatal("disposable TLS database start failed; diagnostics withheld")
	}
	defer func() {
		cleanup, c := context.WithTimeout(context.Background(), 30*time.Second)
		defer c()
		if exec.CommandContext(cleanup, "docker", "rm", "-f", "-v", name).Run() != nil {
			t.Error("disposable TLS database cleanup failed")
		}
	}()
	raw, err := docker("port", name, "5432/tcp")
	if err != nil {
		t.Fatal("disposable database port unavailable")
	}
	_, port, err := net.SplitHostPort(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatal("invalid fixture port")
	}
	connection := func(role, sslmode string) string {
		u := url.URL{Scheme: "postgres", Host: net.JoinHostPort("127.0.0.1", port), Path: "/fabric_tls_tests", User: url.UserPassword(role, password)}
		q := url.Values{"sslmode": {sslmode}, "sslrootcert": {filepath.Join(private, "ca.pem")}}
		u.RawQuery = q.Encode()
		return u.String()
	}
	var admin *Database
	for ctx.Err() == nil {
		admin, err = Open(ctx, connection("fixture_admin", "verify-full"), false)
		if err == nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if err != nil {
		t.Fatal("TLS fixture did not become ready; diagnostics withheld")
	}
	defer admin.Close()
	if err = admin.Migrate(ctx); err != nil {
		t.Fatal("fixture schema setup failed")
	}
	// SQL password is delivered as protocol data in memory, never process argv.
	quoted := "'" + password + "'"
	for _, sql := range []string{"CREATE ROLE fixture_api LOGIN PASSWORD " + quoted, "GRANT CONNECT ON DATABASE fabric_tls_tests TO fixture_api", "GRANT USAGE ON SCHEMA public TO fixture_api", "GRANT SELECT ON sathonate_oidc_memberships TO fixture_api", "GRANT SELECT,INSERT,UPDATE,DELETE ON sathonate_networks,sathonate_network_revisions,sathonate_operations,sathonate_deployments,sathonate_maintenance TO fixture_api"} {
		if _, err = admin.pool.Exec(ctx, sql); err != nil {
			t.Fatal("restricted role setup failed")
		}
	}
	service, err := Open(ctx, connection("fixture_api", "verify-full"), true)
	if err != nil {
		t.Fatal("restricted verified-TLS role rejected")
	}
	for _, tenant := range []string{"tenant-a", "tenant-b"} {
		if _, err := admin.pool.Exec(ctx, "INSERT INTO sathonate_networks(tenant_id,id,network_key,revision,payload) VALUES($1,'fixture','fixture',1,'{}')", tenant); err != nil {
			t.Fatal("RLS fixture")
		}
	}
	tx, err := service.pool.Begin(ctx)
	if err != nil {
		t.Fatal("RLS transaction")
	}
	if _, err = tx.Exec(ctx, "SELECT set_config('sathonate.tenant_id','tenant-a',true)"); err != nil {
		t.Fatal("RLS context")
	}
	var count int
	if err = tx.QueryRow(ctx, "SELECT count(*) FROM sathonate_networks").Scan(&count); err != nil || count != 1 {
		t.Fatal("restricted role crossed tenant boundary")
	}
	if _, err = tx.Exec(ctx, "UPDATE sathonate_networks SET revision=2 WHERE tenant_id='tenant-b'"); err != nil {
		t.Fatal("RLS update query")
	}
	_ = tx.Rollback(ctx)
	t.Run("customer", func(t *testing.T) { customerPostgresFixture(t, ctx, admin, service) })
	service.Close()
	if db, err := Open(ctx, connection("fixture_admin", "verify-full"), true); err == nil {
		db.Close()
		t.Fatal("superuser accepted for production API")
	}
	for _, mode := range []string{"disable", "prefer", "require", "verify-ca"} {
		if db, err := Open(ctx, connection("fixture_api", mode), true); err == nil {
			db.Close()
			t.Fatal("weak TLS mode accepted", mode)
		}
	}
	otherKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	other := *ca
	other.Subject = pkix.Name{CommonName: "untrusted-root"}
	otherDER, _ := x509.CreateCertificate(rand.Reader, &other, &other, &otherKey.PublicKey, otherKey)
	write("wrong-ca.pem", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: otherDER}))
	wrongTrust := strings.Replace(connection("fixture_api", "verify-full"), url.QueryEscape(filepath.Join(private, "ca.pem")), url.QueryEscape(filepath.Join(private, "wrong-ca.pem")), 1)
	if db, err := Open(ctx, wrongTrust, true); err == nil {
		db.Close()
		t.Fatal("untrusted database certificate accepted")
	}
	for _, attribute := range []string{"CREATEROLE", "CREATEDB", "REPLICATION", "BYPASSRLS"} {
		t.Run(attribute, func(t *testing.T) {
			if _, err := admin.pool.Exec(ctx, "ALTER ROLE fixture_api "+attribute); err != nil {
				t.Fatal("role fixture change")
			}
			if db, err := Open(ctx, connection("fixture_api", "verify-full"), true); err == nil {
				db.Close()
				t.Error("privileged role accepted")
			}
			if _, err := admin.pool.Exec(ctx, "ALTER ROLE fixture_api NO"+attribute); err != nil {
				t.Fatal("role fixture reset")
			}
		})
	}
	for _, role := range []string{"pg_read_server_files", "pg_write_server_files", "pg_execute_server_program", "pg_monitor", "pg_read_all_data", "pg_write_all_data"} {
		t.Run(role, func(t *testing.T) {
			if _, err := admin.pool.Exec(ctx, "GRANT "+role+" TO fixture_api"); err != nil {
				t.Fatal("predefined role fixture")
			}
			if db, err := Open(ctx, connection("fixture_api", "verify-full"), true); err == nil {
				db.Close()
				t.Error("predefined privileged role accepted")
			}
			if _, err := admin.pool.Exec(ctx, "REVOKE "+role+" FROM fixture_api"); err != nil {
				t.Fatal("predefined role reset")
			}
		})
	}
	if _, err = admin.pool.Exec(ctx, "GRANT INSERT ON sathonate_oidc_memberships TO fixture_api"); err != nil {
		t.Fatal("membership privilege fixture")
	}
	if db, err := Open(ctx, connection("fixture_api", "verify-full"), true); err == nil {
		db.Close()
		t.Fatal("membership-writable API role accepted")
	}
	if _, err = admin.pool.Exec(ctx, "REVOKE INSERT ON sathonate_oidc_memberships FROM fixture_api"); err != nil {
		t.Fatal("membership privilege reset")
	}
	if _, err = admin.pool.Exec(ctx, "CREATE ROLE fixture_dba CREATEROLE; GRANT fixture_dba TO fixture_api"); err != nil {
		t.Fatal("privileged membership setup")
	}
	if db, err := Open(ctx, connection("fixture_api", "verify-full"), true); err == nil {
		db.Close()
		t.Fatal("privileged membership accepted")
	}
	if _, err = admin.pool.Exec(ctx, "REVOKE fixture_dba FROM fixture_api; ALTER TABLE sathonate_networks OWNER TO fixture_api"); err != nil {
		t.Fatal("ownership fixture setup")
	}
	if db, err := Open(ctx, connection("fixture_api", "verify-full"), true); err == nil {
		db.Close()
		t.Fatal("application-table owner accepted")
	}
	fmt.Println("Disposable PostgreSQL verified TLS, restricted role, privileged membership/table ownership denial and cleanup exercised; credentials withheld")
}
