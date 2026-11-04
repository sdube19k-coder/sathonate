// fabric-maintenance performs local encrypted bundle operations only. It does
// not export Kubernetes Secrets, contact a CA or execute live rotation/destruction.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sathonate.local/fabric-control-plane/internal/maintenance"
	"time"
)

func read(path string, limit int64) ([]byte, error) {
	file, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer file.Close()
	raw, e := io.ReadAll(io.LimitReader(file, limit+1))
	if int64(len(raw)) > limit {
		return nil, fmt.Errorf("input limit")
	}
	return raw, e
}
func main() {
	action := flag.String("action", "", "backup, restore or inspect-certificate (local files only)")
	input := flag.String("input", "", "private input file")
	output := flag.String("output", "", "new private output file")
	bindingFile := flag.String("binding", "", "non-secret exact binding JSON")
	keyFile := flag.String("key-file", "", "protected 32-byte key file; never inline")
	flag.Parse()
	fail := func() {
		fmt.Fprintln(os.Stderr, "Local maintenance operation failed; check private paths, key, binding and input type. No credentials are printed.")
		os.Exit(1)
	}
	raw, e := read(*input, maintenance.MaxBackupBytes*2)
	if e != nil {
		fail()
	}
	defer clear(raw)
	if *action == "inspect-certificate" {
		observation, e := maintenance.InspectCertificate(raw, time.Now())
		if e != nil {
			fail()
		}
		json.NewEncoder(os.Stdout).Encode(observation)
		return
	}
	if *action != "backup" && *action != "restore" {
		fail()
	}
	metadata, e := read(*bindingFile, 4096)
	if e != nil {
		fail()
	}
	var binding maintenance.Binding
	decoder := json.NewDecoder(bytes.NewReader(metadata))
	decoder.DisallowUnknownFields()
	var extra any
	if decoder.Decode(&binding) != nil || decoder.Decode(&extra) != io.EOF {
		fail()
	}
	key, e := read(*keyFile, 32)
	if e != nil {
		fail()
	}
	defer clear(key)
	var result []byte
	var receipt maintenance.BackupReceipt
	if *action == "backup" {
		result, receipt, e = maintenance.SealBackup(binding, key, raw, time.Now())
	} else {
		result, receipt, e = maintenance.OpenBackup(binding, key, raw)
	}
	if e != nil {
		fail()
	}
	defer clear(result)
	if e = maintenance.CreatePrivate(*output, result); e != nil {
		fail()
	}
	json.NewEncoder(os.Stdout).Encode(receipt)
}
