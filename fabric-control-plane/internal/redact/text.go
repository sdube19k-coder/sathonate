// Package redact removes common credential forms before logs enter product responses or journals.
package redact

import "regexp"

var privateKey = regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`)
var urlCredentials = regexp.MustCompile(`(https?://)[^\s/@:]+:[^\s/@]+@`)
var jsonSecret = regexp.MustCompile(`(?i)("(?:password|passwd|pass|secret|enrollsecret|enroll_secret|enroll-pw|enroll-secret|token|access_token|refresh_token|client_secret|private_key|aws_secret_access_key|aws_session_token|authorization)"\s*:\s*)"(?:\\.|[^"\\])*"`)
var assignment = regexp.MustCompile(`(?i)(\b(?:password|passwd|pass|secret|enrollsecret|enroll_secret|enroll-pw|enroll-secret|client_secret|aws_secret_access_key|aws_session_token|vault_token|git_token|token)\s*[=:]\s*)(?:"[^"\r\n]*"|'[^'\r\n]*'|[^\s,;]+)`)
var bearer = regexp.MustCompile(`(?i)\bBearer\s+[a-z0-9._~+/=-]+`)

func Text(s string) string {
	s = privateKey.ReplaceAllString(s, "[REDACTED PRIVATE KEY]")
	s = urlCredentials.ReplaceAllString(s, "${1}[REDACTED]@")
	s = jsonSecret.ReplaceAllString(s, `${1}"[REDACTED]"`)
	s = assignment.ReplaceAllString(s, "${1}[REDACTED]")
	return bearer.ReplaceAllString(s, "Bearer [REDACTED]")
}
