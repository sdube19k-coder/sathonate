package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"
	"time"

	"sathonate.local/fabric-control-plane/internal/customer"
	"sathonate.local/fabric-control-plane/internal/persistence"
)

func main() {
	tenant := flag.String("tenant", "", "administrator-approved tenant ID")
	project := flag.String("project", "", "administrator-approved project ID")
	window := flag.Duration("window", 7*24*time.Hour, "receipt lookback")
	flag.Parse()
	if *tenant == "" || *project == "" || os.Getenv("CUSTOMER_METER_DATABASE_URL") == "" {
		log.Fatal("explicit project scope and restricted metering connection required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	db, err := persistence.Open(ctx, os.Getenv("CUSTOMER_METER_DATABASE_URL"), true)
	if err != nil {
		log.Fatal("verified metering database connection unavailable")
	}
	defer db.Close()
	store := customer.Postgres{Pool: db.Pool()}
	if store.MeterReady(ctx) != nil {
		log.Fatal("restricted metering grants unavailable")
	}
	result, err := (customer.Meter{Service: customer.Service{Store: store}, Source: store}).Sweep(ctx, *tenant, *project, *window)
	if json.NewEncoder(os.Stdout).Encode(result) != nil || err != nil {
		log.Fatal("metering sweep incomplete; retry or review the source window")
	}
}
