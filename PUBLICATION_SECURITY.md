# Source publication security review

This private import contains committed source snapshots and redacted evidence.
It excludes old local Git history, runtime data, credentials, keys, kubeconfigs,
binary downloads, generated artifacts and unverified AWS drafts. No old account
repository or SSH key was modified.

Gitleaks v8.30.1 was downloaded from its official GitHub release. The archive
SHA256 matched the checksum published with the official release:
`d29144deff3a68aa93ced33dddf84b7fdc26070add4aa0f4513094c8332afc4e`.

The initial default scan reported 42 generic-key matches: 41 public SHA256
provenance values and one `kubectl get secret` resource name. Each was reviewed.
`.gitleaks.toml` extends every default detection rule and permits only these
exact lines at their exact paths, for the generic-key rule alone. No file,
directory, private-key rule or token rule is broadly excluded. Changed or newly
added credentials are not covered by those exact-content exceptions.

A temporary random canary was inserted into an isolated copy at an
exception-listed evidence path. The scanner still detected it. The canary and
all scanner working reports stay outside this repository.

This scan does not close historical exposed-key remediation or qualify production.
See the existing security-remediation matrix and Sprint 4 acceptance limitations.
