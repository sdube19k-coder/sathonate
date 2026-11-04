package kubernetesfabric

import "testing"

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
