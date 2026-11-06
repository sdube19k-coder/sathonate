package domain

import (
	"os"
	"strings"
	"testing"
)

func TestPublicNetworkIntentRejectsPrivateMaterialInDisplayFields(t *testing.T) {
	raw, e := os.ReadFile("../../examples/dynamic/a.json")
	if e != nil {
		t.Fatal(e)
	}
	base, e := Decode(strings.NewReader(string(raw)))
	if e != nil {
		t.Fatal(e)
	}
	for _, value := range []string{"-----BEGIN PRIVATE KEY-----\nsynthetic-noncredential\n-----END PRIVATE KEY-----", "eyJ" + strings.Repeat("a", 16) + "." + strings.Repeat("b", 16) + "." + strings.Repeat("c", 32)} {
		s := base
		s.Metadata.Name = value
		issues := Validate(s)
		found := false
		for _, i := range issues {
			if strings.Contains(i.Message, "bearer tokens") {
				found = true
			}
			if strings.Contains(i.Message, value) {
				t.Fatal("private intent echoed in validation diagnostics")
			}
		}
		if !found {
			t.Fatal("private material disguised as a display name was accepted")
		}
	}
}
