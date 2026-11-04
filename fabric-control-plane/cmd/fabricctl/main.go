package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"sathonate.local/fabric-control-plane/internal/domain"
	"sathonate.local/fabric-control-plane/internal/planner"
)

func run(args []string, out, errOut io.Writer) int {
	if len(args) == 0 || (args[0] != "validate" && args[0] != "plan") {
		fmt.Fprintln(errOut, "usage: fabricctl <validate|plan> -file <network.json>")
		return 2
	}
	f := flag.NewFlagSet(args[0], flag.ContinueOnError)
	f.SetOutput(errOut)
	path := f.String("file", "", "network specification JSON")
	if err := f.Parse(args[1:]); err != nil {
		return 2
	}
	if *path == "" || f.NArg() != 0 {
		fmt.Fprintln(errOut, "-file is required; extra arguments are not accepted")
		return 2
	}
	file, err := os.Open(*path)
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 2
	}
	defer file.Close()
	const maxSpecBytes = 4 << 20
	data, err := io.ReadAll(io.LimitReader(file, maxSpecBytes+1))
	if err != nil || len(data) > maxSpecBytes {
		fmt.Fprintln(errOut, "cannot read specification or file exceeds 4 MiB")
		return 2
	}
	s, err := domain.Decode(bytes.NewReader(data))
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 2
	}
	issues := domain.Validate(s)
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	if args[0] == "validate" || domain.HasErrors(issues) {
		result := struct {
			Valid  bool           `json:"valid"`
			Issues []domain.Issue `json:"issues"`
		}{!domain.HasErrors(issues), issues}
		if err := enc.Encode(result); err != nil {
			fmt.Fprintln(errOut, err)
			return 2
		}
		if !result.Valid {
			return 1
		}
		return 0
	}
	p, err := planner.Create(s)
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 1
	}
	if err := enc.Encode(p); err != nil {
		fmt.Fprintln(errOut, err)
		return 2
	}
	return 0
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }
