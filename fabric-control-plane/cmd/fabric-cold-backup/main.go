// Local administrator-only cold volume recovery. API tokens and private material
// never appear in command arguments or receipt output. The DB URL is inherited.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"sathonate.local/fabric-control-plane/internal/deployment"
	"sathonate.local/fabric-control-plane/internal/maintenance"
	"sathonate.local/fabric-control-plane/internal/persistence"
	"time"
)

func main() {
	config := flag.String("secure-local-config", "", "protected disposable target config")
	action := flag.String("action", "", "capture, prepare-restore, restore, resume or scheduled-capture")
	tenant := flag.String("tenant", "", "verified server storage tenant")
	operation := flag.String("operation", "", "latest verified operation")
	artifact := flag.String("artifact", "", "new encrypted artifact for capture, existing artifact for restore/resume")
	keyFile := flag.String("key-file", "", "protected 32-byte key file, never inline")
	bindingFile := flag.String("binding", "", "optional public capture binding for exact scoped control-row restore")
	scheduleDirectory := flag.String("schedule-directory", "", "operator-owned private absolute directory for daily artifacts")
	scheduleHour := flag.Int("schedule-hour-utc", -1, "UTC hour at which scheduled-capture may run")
	retain := flag.Int("schedule-keep", 7, "number of completed daily encrypted artifacts to keep (1-365)")
	flag.Parse()
	fail := func() {
		fmt.Fprintln(os.Stderr, "Cold maintenance failed; private diagnostics withheld. Keep the API stopped and inspect its protected checkpoint.")
		os.Exit(1)
	}
	if os.Getenv("SATHONATE_APP_ENV") != "local-development" {
		fail()
	}
	key, e := os.ReadFile(*keyFile)
	if e != nil || len(key) != 32 {
		fail()
	}
	defer clear(key)
	runner, e := deployment.LoadSecureRunner(*config)
	if e != nil {
		fail()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()
	dbConfig, parseErr := pgxpool.ParseConfig(os.Getenv("DATABASE_URL"))
	if parseErr != nil || !deployment.ColdDatabaseAllowed(dbConfig.ConnConfig.Host, dbConfig.ConnConfig.Database) {
		fail()
	}
	db, e := persistence.Open(ctx, os.Getenv("DATABASE_URL"), false)
	if e != nil {
		fail()
	}
	defer db.Close()
	var binding *maintenance.Binding
	if *bindingFile != "" {
		data, err := os.ReadFile(*bindingFile)
		if err != nil || len(data) > 4096 {
			fail()
		}
		if json.Unmarshal(data, &binding) != nil {
			fail()
		}
	}
	if *action == "scheduled-capture" {
		if *artifact != "" || binding != nil || *tenant == "" || *operation == "" {
			fail()
		}
		var network string
		tx, err := db.Pool().BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
		if err != nil {
			fail()
		}
		if _, err = tx.Exec(ctx, "SELECT set_config('sathonate.tenant_id',$1,true)", *tenant); err == nil {
			err = tx.QueryRow(ctx, "SELECT network_id FROM sathonate_operations WHERE tenant_id=$1 AND id=$2 AND status='succeeded'", *tenant, *operation).Scan(&network)
		}
		_ = tx.Rollback(ctx)
		if err != nil || network == "" {
			fail()
		}
		target := *tenant + "\x00" + network + "\x00" + runner.BindingDigest(deployment.Request{})
		var receipt deployment.ColdReceipt
		ran, err := maintenance.RunDailyBackup(*scheduleDirectory, target, *scheduleHour, *retain, time.Now(), func(path string) (string, error) {
			var captureErr error
			receipt, captureErr = runner.ColdBackup(ctx, db.Pool(), deployment.ColdOptions{Tenant: *tenant, Operation: *operation, Action: "capture-resume", Artifact: path, Key: key})
			return receipt.ArtifactSHA256, captureErr
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, "Scheduled backup refused or interrupted:", err)
			os.Exit(1)
		}
		if !ran {
			fmt.Println(`{"status":"skipped"}`)
		} else {
			_ = json.NewEncoder(os.Stdout).Encode(receipt)
		}
		return
	}
	receipt, e := runner.ColdBackup(ctx, db.Pool(), deployment.ColdOptions{Tenant: *tenant, Operation: *operation, Action: *action, Artifact: *artifact, Key: key, Binding: binding})
	if e != nil {
		// ColdBackup exposes fixed operational categories, never driver output,
		// SQL, certificates, key bytes or subprocess diagnostics.
		fmt.Fprintln(os.Stderr, "Cold maintenance refused:", e.Error())
		os.Exit(1)
	}
	json.NewEncoder(os.Stdout).Encode(receipt)
}
