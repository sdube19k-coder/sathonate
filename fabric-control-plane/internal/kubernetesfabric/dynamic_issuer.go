package kubernetesfabric

import (
	"bytes"
	"encoding/pem"
	"sort"
)

// An existing versioned service retains its original enrolled issuer even when
// membership changes reorder peers. Root provenance comes only from managed
// channel authorities, never from a caller-supplied certificate.
func dynamicServiceIssuer(existingRoot string, roots map[string]string) (string, error) {
	ids := []string{}
	for id := range roots {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	if len(ids) == 0 {
		return "", secureDenied
	}
	if existingRoot == "" {
		return ids[0], nil
	}
	owned, rest := pem.Decode([]byte(existingRoot))
	if owned == nil || owned.Type != "CERTIFICATE" || len(bytes.TrimSpace(rest)) != 0 {
		return "", secureDenied
	}
	match := ""
	for _, id := range ids {
		root, rest := pem.Decode([]byte(roots[id]))
		if root == nil || root.Type != "CERTIFICATE" || len(bytes.TrimSpace(rest)) != 0 {
			return "", secureDenied
		}
		if bytes.Equal(owned.Bytes, root.Bytes) {
			if match != "" {
				return "", secureDenied
			}
			match = id
		}
	}
	if match == "" {
		return "", secureDenied
	}
	return match, nil
}
