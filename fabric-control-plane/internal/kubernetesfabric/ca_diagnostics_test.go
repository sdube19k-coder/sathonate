package kubernetesfabric

import "testing"

func TestCADiagnosticsNeverEchoPrivateOutput(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{{"secret=canary authentication failure", "authentication"}, {"PRIVATE KEY canary failed to load identity", "registrar-identity"}, {"canary unrelated", "unclassified"}} {
		if got := caFailureCategory([]byte(tc.raw)); got != tc.want {
			t.Fatal("unsafe diagnostic category", got)
		}
	}
}
