// Offline only. Never authenticates, applies manifests or provisions resources.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"sathonate.local/fabric-control-plane/internal/deployment"
)

func main() {
	input := flag.String("input", "", "trusted qualification JSON; no credentials")
	output := flag.String("output", "", "new offline export; refuses overwrite")
	flag.Parse()
	fail := func() {
		fmt.Fprintln(os.Stderr, "Offline qualification export rejected; inspect trusted bindings and choose a new output file")
		os.Exit(1)
	}
	if *input == "" || *output == "" {
		fail()
	}
	f, err := os.Open(*input)
	if err != nil {
		fail()
	}
	d := json.NewDecoder(io.LimitReader(f, 1<<20))
	d.DisallowUnknownFields()
	var spec deployment.QualificationExport
	var extra any
	if d.Decode(&spec) != nil || d.Decode(&extra) != io.EOF {
		f.Close()
		fail()
	}
	f.Close()
	result, err := deployment.ExportQualification(spec)
	if err != nil {
		fail()
	}
	raw, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fail()
	}
	out, err := os.OpenFile(*output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		fail()
	}
	_, err = out.Write(append(raw, '\n'))
	closeErr := out.Close()
	if err != nil || closeErr != nil {
		fail()
	}
	fmt.Println("Offline manifests exported; no AWS/Kubernetes action executed or qualified")
}
