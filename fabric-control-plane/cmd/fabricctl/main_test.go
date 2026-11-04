package main

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestCLI(t *testing.T) {
	for _, command := range []string{"validate", "plan"} {
		var out, errOut bytes.Buffer
		if code := run([]string{command, "-file", "../../examples/native-network.json"}, &out, &errOut); code != 0 {
			t.Fatalf("%s failed: %s", command, errOut.String())
		}
		if !json.Valid(out.Bytes()) {
			t.Fatal("CLI output is not JSON")
		}
	}
}

func TestCLIRejectsMissingFile(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"plan", "-file", "does-not-exist.json"}, &out, &errOut); code != 2 {
		t.Fatalf("unexpected exit code %d", code)
	}
}
