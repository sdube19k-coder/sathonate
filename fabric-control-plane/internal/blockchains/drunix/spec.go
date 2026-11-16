// Package drunix describes a planning-only Drunix v1.0.0 network.
package drunix

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
)

const (
	ID          = "drunix"
	Version     = "1.0.0"
	SourceSHA   = "3fb2b135a5b2adf7e1071d9b1624052a1bbb1772"
	ReleaseURL  = "https://github.com/npci/drunix/releases/tag/v1.0.0"
	SpecVersion = "drunix.sathonate.io/v1alpha1"
)

type NetworkSpec struct {
	APIVersion    string         `json:"apiVersion"`
	Version       string         `json:"version"`
	Name          string         `json:"name"`
	Consensus     string         `json:"consensus"`
	Organizations []Organization `json:"organizations"`
	Orderer       Orderer        `json:"orderer"`
}

type Organization struct {
	ID                string `json:"id"`
	MSPID             string `json:"mspId"`
	LitePeer          string `json:"litePeer"`
	CommittingPeer    string `json:"committingPeer"`
	ValidationService string `json:"validationService"`
	StateDatabase     string `json:"stateDatabase"`
	TransientStore    string `json:"transientStore"`
}

type Orderer struct {
	ID    string `json:"id"`
	MSPID string `json:"mspId"`
}

var (
	dnsLabel = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
	mspID    = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,63}$`)
)

// Decode validates a single Sathonate planning spec. No upstream config,
// credentials, target selection or executable input is accepted.
func Decode(raw []byte) (NetworkSpec, error) {
	var spec NetworkSpec
	if len(raw) > 64<<10 {
		return spec, errors.New("Drunix spec exceeds 64 KiB")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&spec); err != nil {
		return NetworkSpec{}, errors.New("invalid Drunix spec JSON or unknown field")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return NetworkSpec{}, errors.New("expected exactly one Drunix spec object")
	}
	if err := spec.Validate(); err != nil {
		return NetworkSpec{}, err
	}
	return spec, nil
}

// Validate checks the modeled upstream sample shape, not deployability.
func (s NetworkSpec) Validate() error {
	if s.APIVersion != SpecVersion || s.Version != Version {
		return errors.New("unsupported Drunix spec or protocol version")
	}
	if !dnsLabel.MatchString(s.Name) || s.Consensus != "raft" || len(s.Organizations) != 2 {
		return errors.New("reference profile requires a DNS-label name, Raft and two peer organizations")
	}
	ids, msps := map[string]bool{}, map[string]bool{}
	register := func(id string, seen map[string]bool, pattern *regexp.Regexp) bool {
		if !pattern.MatchString(id) || seen[id] {
			return false
		}
		seen[id] = true
		return true
	}
	if !register(s.Orderer.ID, ids, dnsLabel) || !register(s.Orderer.MSPID, msps, mspID) {
		return errors.New("reference profile requires a single orderer with a valid node ID and MSP ID")
	}
	for i, org := range s.Organizations {
		if !register(org.ID, ids, dnsLabel) || !register(org.MSPID, msps, mspID) {
			return fmt.Errorf("organization %d requires a unique organization ID and MSP ID", i+1)
		}
		for _, node := range []string{org.LitePeer, org.CommittingPeer, org.ValidationService} {
			if !register(node, ids, dnsLabel) {
				return fmt.Errorf("organization %d requires distinct lite peer, committing peer and validation service IDs", i+1)
			}
		}
		if org.StateDatabase != "yugabytedb" || org.TransientStore != "keydb" {
			return fmt.Errorf("organization %d must use reference storage: yugabytedb and keydb", i+1)
		}
	}
	return nil
}
