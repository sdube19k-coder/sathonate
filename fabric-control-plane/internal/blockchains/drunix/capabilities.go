package drunix

type Capability struct {
	ID       string `json:"id"`
	Evidence string `json:"evidence"`
}

// Capabilities returns upstream source observations, never runtime grants.
func Capabilities() []Capability {
	const source = "https://github.com/npci/drunix/blob/v1.0.0/"
	const architecture = source + "docs/drunix-arch.md"
	const sample = source + "drunix-network/test-network/README.md"
	return []Capability{
		{"channel-and-chaincode-sample", sample},
		{"keydb-transient-store", architecture},
		{"msp-tls", source + "drunix-network/config/peer/core.yaml"},
		{"raft-sample", sample},
		{"split-peer-roles", architecture},
		{"stateless-validation", architecture},
		{"yugabytedb-state", architecture},
	}
}
