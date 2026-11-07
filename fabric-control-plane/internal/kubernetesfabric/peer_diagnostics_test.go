package kubernetesfabric

import (
	"strings"
	"testing"
)

func TestPeerOperationDiagnosticsExcludeArguments(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{{[]string{"lifecycle", "chaincode", "install", "private-canary"}, "lifecycle-install"}, {[]string{"chaincode", "query", "private-canary"}, "chaincode-query"}, {[]string{"private-canary"}, "unclassified"}, {[]string{"lifecycle", "chaincode", "private-canary"}, "unclassified"}} {
		if got := peerOperation(tc.args); got != tc.want {
			t.Fatal("untrusted operation diagnostic", got)
		}
	}
}

func TestRecoveryPeerDiagnosticsClassifiesOrderingWithoutExportingPayload(t *testing.T) {
	for _, tc := range []struct{ raw, category string }{{"private-canary Error: can't read block: SERVICE_UNAVAILABLE", "ordering-service-unavailable"}, {"private-canary no such file or directory", "local-material-file-unavailable"}, {"private-canary BAD_REQUEST", "ordering-bad-request"}} {
		e := securePeerFailure([]byte(tc.raw))
		if !strings.Contains(e.Error(), "("+tc.category+")") || strings.Contains(e.Error(), "private-canary") {
			t.Fatal("diagnostic classification leaked or lost category")
		}
	}
}
