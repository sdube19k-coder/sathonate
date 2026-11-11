// Disposable PostgreSQL logical backup. Run with matching PostgreSQL client tools.
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"sathonate.local/fabric-control-plane/internal/maintenance"
)

var fixtureName = regexp.MustCompile(`^sathonate_dr_[a-f0-9]{24}$`)

type boundedDump struct{ buffer bytes.Buffer }

func (w *boundedDump) Write(p []byte) (int, error) {
	if len(p) > maintenance.MaxBackupBytes-w.buffer.Len() {
		return 0, errors.New("logical archive exceeds limit")
	}
	return w.buffer.Write(p)
}

func privateInput(path string, max int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > max {
		return nil, errors.New("invalid private input")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return nil, errors.New("private input changed")
	}
	data, err := io.ReadAll(io.LimitReader(file, max+1))
	if err != nil || int64(len(data)) > max {
		clear(data)
		return nil, errors.New("private input exceeds limit")
	}
	return data, nil
}

func client(ctx context.Context, c *pgx.ConnConfig, database, tool string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, tool, append([]string{"--no-password"}, args...)...)
	// Do not inherit libpq routing, startup options or password-file settings.
	cmd.Env = []string{"PGHOST=" + c.Host, "PGPORT=" + strconv.Itoa(int(c.Port)), "PGUSER=" + c.User, "PGPASSWORD=" + c.Password, "PGDATABASE=" + database, "PGSSLMODE=disable"}
	return cmd
}

func run(ctx context.Context, action, artifact, keyPath, source string) error {
	if os.Getenv("SATHONATE_APP_ENV") != "local-development" || !fixtureName.MatchString(source) || artifact == "" || keyPath == "" {
		return errors.New("explicit local development and disposable database binding required")
	}
	key, err := privateInput(keyPath, 32)
	defer clear(key)
	if err != nil || len(key) != 32 {
		return errors.New("protected 32-byte key required")
	}
	binding := maintenance.Binding{TenantID: "local-postgres-dr", NetworkID: source, TargetSHA256: maintenance.HashBytes([]byte(source)), NetworkSHA256: maintenance.HashBytes([]byte("sathonate.postgres-logical/v1"))}
	config, err := pgx.ParseConfig(os.Getenv("SATHONATE_POSTGRES_DR_URL"))
	if err != nil || config.Database != source || config.Host != "127.0.0.1" || config.TLSConfig != nil || len(config.Fallbacks) != 0 || len(config.RuntimeParams) != 0 {
		return errors.New("exact IPv4 loopback fixture DSN with sslmode=disable required")
	}
	switch action {
	case "capture":
		var dump boundedDump
		cmd := client(ctx, config, source, "pg_dump", "--format=custom", "--no-owner", "--no-privileges")
		cmd.Stdout = &dump
		defer func() { clear(dump.buffer.Bytes()) }()
		if cmd.Run() != nil {
			return errors.New("bounded logical export failed")
		}
		encrypted, receipt, err := maintenance.SealBackup(binding, key, dump.buffer.Bytes(), time.Now())
		if err != nil || maintenance.CreatePrivate(artifact, encrypted) != nil {
			return errors.New("encrypted artifact creation failed")
		}
		return json.NewEncoder(os.Stdout).Encode(receipt)
	case "restore-new":
		encoded, err := privateInput(artifact, maintenance.MaxBackupBytes*2)
		if err != nil {
			return errors.New("encrypted artifact unavailable")
		}
		dump, receipt, err := maintenance.OpenBackup(binding, key, encoded)
		defer clear(dump)
		if err != nil {
			return errors.New("encrypted artifact authentication failed")
		}
		var random [12]byte
		if _, err := rand.Read(random[:]); err != nil {
			return errors.New("replacement database identity unavailable")
		}
		target := "sathonate_dr_" + hex.EncodeToString(random[:])
		admin := config.Copy()
		admin.Database = "postgres"
		conn, err := pgx.ConnectConfig(ctx, admin)
		if err != nil {
			return errors.New("fixture database administrator unavailable")
		}
		defer conn.Close(context.Background())
		name := pgx.Identifier{target}.Sanitize()
		if _, err := conn.Exec(ctx, "CREATE DATABASE "+name+" TEMPLATE template0"); err != nil {
			return errors.New("could not create new replacement database")
		}
		if _, err := conn.Exec(ctx, "REVOKE CONNECT ON DATABASE "+name+" FROM PUBLIC"); err == nil {
			cmd := client(ctx, config, target, "pg_restore", "--dbname="+target, "--single-transaction", "--exit-on-error", "--no-owner", "--no-privileges")
			cmd.Stdin = bytes.NewReader(dump)
			if cmd.Run() == nil {
				return json.NewEncoder(os.Stdout).Encode(struct {
					Database string                    `json:"database"`
					Backup   maintenance.BackupReceipt `json:"backup"`
				}{target, receipt})
			}
		}
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := conn.Exec(cleanup, "DROP DATABASE "+name); err != nil {
			return fmt.Errorf("replacement restore failed and cleanup is pending for %s", target)
		}
		return errors.New("replacement restore failed and its database was removed")
	default:
		return errors.New("select capture or restore-new")
	}
}

func main() {
	action := flag.String("action", "", "capture or restore-new")
	source := flag.String("source", "", "exact disposable source database name")
	artifact := flag.String("artifact", "", "protected encrypted archive path")
	key := flag.String("key-file", "", "protected 32-byte key path")
	flag.Parse()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := run(ctx, *action, *artifact, *key, *source); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
