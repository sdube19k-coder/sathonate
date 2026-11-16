// Package compatibility serves read-only discovery data. It is not consulted
// by deployment admission, dispatch or recovery.
package compatibility

import "sathonate.local/fabric-control-plane/internal/blockchains/drunix"

const (
	Qualified     = "qualified"
	Pending       = "pending"
	ReferenceOnly = "reference-only"
	Unsupported   = "unsupported"
)

type Combination struct {
	Protocol        string `json:"protocol"`
	ProtocolVersion string `json:"protocolVersion"`
	Engine          string `json:"engine"`
	EngineVersion   string `json:"engineVersion"`
	Target          string `json:"target"`
}

type Capability struct {
	ID       string `json:"id"`
	Evidence string `json:"evidence"`
}

type Protocol struct {
	ID           string       `json:"id"`
	Version      string       `json:"version"`
	SourceCommit string       `json:"sourceCommit,omitempty"`
	ReleaseURL   string       `json:"releaseUrl,omitempty"`
	Capabilities []Capability `json:"upstreamCapabilities,omitempty"`
}

type Entry struct {
	Combination
	Status   string `json:"status"`
	Evidence string `json:"evidence"`
}

type Catalog struct {
	Protocols    []Protocol `json:"protocols"`
	Combinations []Entry    `json:"combinations"`
}

// Snapshot returns fresh values in a stable order. An upstream capability is
// descriptive, independently of Sathonate qualification.
func Snapshot() Catalog {
	const bevel = "cd236ccc61d1d159b3dadb4625d08bb5682a8451"
	const controller = "sha256:427b352b20576258efdb59c9fc266ec01fe7b7f1419ce1e5d7ead488b10cba6c"
	const docs = "https://github.com/sdube19k-coder/sathonate/blob/1fcff204a8db081ea82271c2a272ac1832903079/fabric-control-plane/docs/"
	upstream := drunix.Capabilities()
	capabilities := make([]Capability, len(upstream))
	for i, capability := range upstream {
		capabilities[i] = Capability(capability)
	}
	return Catalog{
		Protocols: []Protocol{
			{ID: drunix.ID, Version: drunix.Version, SourceCommit: drunix.SourceSHA,
				ReleaseURL: drunix.ReleaseURL, Capabilities: capabilities},
			{ID: "fabric", Version: "2.5.15"},
		},
		Combinations: []Entry{
			{Combination{drunix.ID, drunix.Version, "upstream-compose", drunix.SourceSHA, "local-docker-sample"},
				ReferenceOnly, drunix.ReleaseURL},
			{Combination{"fabric", "2.5.15", "bevel", bevel + "@" + controller, "smp-secure-eks-v1"},
				Pending, docs + "sprint3-local-secure-eks.md"},
			{Combination{"fabric", "2.5.15", "bevel", bevel + "@" + controller, "smp-secure-local-v1"},
				Qualified, docs + "sprint3-local-secure-eks.md"},
		},
	}
}

// Decide describes an exact tuple. Unknown versions, engines or targets are
// unsupported. Even Qualified is evidence, not permission to deploy.
func Decide(key Combination) Entry {
	for _, entry := range Snapshot().Combinations {
		if entry.Combination == key {
			return entry
		}
	}
	return Entry{Combination: key, Status: Unsupported}
}
