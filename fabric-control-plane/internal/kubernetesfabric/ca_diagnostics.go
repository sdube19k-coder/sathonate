package kubernetesfabric

import "strings"

// Return only constant categories, never a substring of CA output.
func caFailureCategory(raw []byte) string {
	s := strings.ToLower(string(raw))
	for _, v := range [][2]string{{"certificate signed by unknown authority", "tls-trust"}, {"authentication failure", "authentication"}, {"authorization failure", "authorization"}, {"not authorized", "authorization"}, {"failed to load identity", "registrar-identity"}, {"no such file", "missing-file"}, {"connection refused", "endpoint-unavailable"}, {"already registered", "already-registered"}, {"max enrollments", "enrollment-limit"}} {
		if strings.Contains(s, v[0]) {
			return v[1]
		}
	}
	return "unclassified"
}
