# Repository access

Owner: `sdube19k-coder`. Repository: `sdube19k-coder/sathonate` (private).

The local Windows checkout uses a dedicated Ed25519 deployment key with write
access to this repository only. The private file remains outside the project,
with inheritance disabled and access limited to the current Windows user.
Neither key material nor CLI tokens are stored in this Git repository.

The local remote uses SSH host alias `github-sathonate-sdube19k-coder`. A
repository-local `core.sshCommand` selects the separate configuration file
`C:/Users/dubes/.ssh/sathonate_sdube19k_coder.conf`; it selects only this key,
disables agent fallback and requires GitHub's verified Ed25519 host key.
Existing SSH keys, global SSH configuration and the old account are preserved.

GitHub CLI authorization for administration is isolated under
`E:/Sathonate/.cache/github-sdube19k-coder/gh`. For future CLI use:

```powershell
$env:GH_CONFIG_DIR='E:/Sathonate/.cache/github-sdube19k-coder/gh'
gh api user --jq .login
# Must print sdube19k-coder before any administration.
```

Other computers need their own authorized key; never transfer this private key
through Git. If this machine/key is retired, revoke the corresponding deploy
key in the new repository's settings. Do not revoke unrelated account keys.
