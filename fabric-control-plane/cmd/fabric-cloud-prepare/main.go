// fabric-cloud-prepare materializes the server-generated Bevel handoff.
// Preparation does not authorize or execute cloud deployment.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"sathonate.local/fabric-control-plane/internal/cloud"
)

func main() {
	input := flag.String("package", "", "saved deployment package JSON")
	digest := flag.String("sha256", "", "expected SHA256 of the complete package file")
	registryPath := flag.String("registry", "", "trusted server target registry")
	tenant := flag.String("tenant", "", "tenant selected by the trusted dispatcher")
	output := flag.String("output", "", "new private output directory")
	flag.Parse()
	if err := run(*input, *digest, *registryPath, *tenant, *output); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(input, digest, registryPath, tenant, output string) error {
	if input == "" || registryPath == "" || tenant == "" || output == "" || len(digest) != 64 {
		return fmt.Errorf("package, sha256, registry, tenant and new output directory are required")
	}
	f, err := os.Open(input)
	if err != nil {
		return err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, 2<<20))
	if err != nil {
		return err
	}
	hash := sha256.Sum256(raw)
	if hex.EncodeToString(hash[:]) != digest {
		return fmt.Errorf("package digest mismatch")
	}
	var p cloud.Package
	if err = json.Unmarshal(raw, &p); err != nil {
		return fmt.Errorf("invalid deployment package")
	}
	registry, err := cloud.LoadRegistry(registryPath)
	if err != nil {
		return err
	}
	prepared, err := cloud.Prepare(p.Desired, p.Revision, tenant, registry)
	if err != nil {
		return err
	}
	if p.APIVersion != prepared.APIVersion || p.Digest != prepared.Digest {
		return fmt.Errorf("network digest or package version mismatch")
	}
	// Recompile from typed desired input and trusted target configuration; never
	// execute the supplied package's variables, manifests or authorization field.
	if prepared.Bundle == nil {
		return fmt.Errorf("target binding or Bevel compilation is blocked")
	}
	if err = os.Mkdir(output, 0700); err != nil {
		return fmt.Errorf("output must be a new private directory: %w", err)
	}
	write := func(name string, v any) error {
		b, e := json.MarshalIndent(v, "", "  ")
		if e != nil {
			return e
		}
		return os.WriteFile(filepath.Join(output, name), b, 0600)
	}
	if err = write("network.json", p.Desired); err != nil {
		return err
	}
	if err = write("bevel-input.json", prepared.Bundle.Variables); err != nil {
		return err
	}
	if err = write("kubernetes.json", prepared.Manifests); err != nil {
		return err
	}
	return write("readiness.json", prepared.Checks)
}
