package kubernetesfabric

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// secretCLICommand never places the logical CLI arguments in OS argv or env.
// The source-built CLI consumes this bounded JSON over a private anonymous pipe.
func secretCLICommand(args []string) ([]string, []byte, error) {
	raw, err := json.Marshal(args)
	if err != nil || len(raw) > 1<<20 {
		return nil, nil, fmt.Errorf("protected CLI input is invalid")
	}
	return []string{"--smp-stdin-arguments"}, raw, nil
}
func (w *Worker) protectedHLF(args []string) ([]byte, error) {
	argv, input, err := secretCLICommand(args)
	if err != nil {
		return nil, err
	}
	defer clear(input)
	cmd := exec.CommandContext(w.ctx, "kubectl-hlf", argv...)
	cmd.Stdin = bytes.NewReader(input)
	var out, diagnostics bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &diagnostics
	if err = w.execute(cmd); err != nil {
		// Only these fixed semantic markers may cross the private CLI boundary.
		// Full upstream diagnostics may contain CA enrollment requests or private keys.
		msg := strings.ToLower(diagnostics.String())
		for _, marker := range []string{"already registered", "not found", "not defined", "does not exist", "could not find", "sequence: '0'"} {
			if strings.Contains(msg, marker) {
				if marker == "sequence: '0'" {
					marker = "could not fetch approved chaincode definition sequence: '0'"
				}
				return nil, fmt.Errorf("protected Bevel command failed: %s", marker)
			}
		}
		return nil, fmt.Errorf("protected Bevel command failed; inspect authorized cluster health without exporting credentials")
	}
	return out.Bytes(), nil
}
func identityCredentialKey(org, identity string) string {
	sum := sha256.Sum256([]byte(org + "/" + identity))
	return "v2-" + hex.EncodeToString(sum[:16])
}

// Only known ephemeral files under the worker's memory-backed workspace are removed.
// Persistent enrollment Secrets are retained for reconciliation; this is not rotation.
func clearWorkspaceSecrets() {
	paths, _ := filepath.Glob("/workspace/*")
	for _, path := range paths {
		name := filepath.Base(path)
		if strings.HasSuffix(name, "-ca.yaml") || strings.HasSuffix(name, "-tlsca.yaml") || name == "network.yaml" || strings.HasSuffix(name, "-msp") {
			_ = os.RemoveAll(path)
		}
	}
}

// validateRenderedCredentials permits only the pinned operator's required inline
// enrollment fields. Do not persist rendered YAML or echo it in events/errors.
func (w *Worker) validateRenderedCredentials(v Object) error {
	kind := text(v["kind"])
	allowed := func(path string) bool {
		switch kind {
		case "FabricCA":
			return regexp.MustCompile(`^spec\.(ca|tlsCA)\.registry\.identities\.[0-9]+\.pass$`).MatchString(path)
		case "FabricPeer", "FabricOrdererNode":
			return path == "spec.secret.enrollment.component.enrollsecret" || path == "spec.secret.enrollment.tls.enrollsecret"
		}
		return false
	}
	var walk func(any, string) error
	walk = func(value any, path string) error {
		switch x := value.(type) {
		case map[string]any:
			for key, child := range x {
				p := key
				if path != "" {
					p = path + "." + key
				}
				if err := walk(child, p); err != nil {
					return err
				}
			}
		case []any:
			for i, child := range x {
				if err := walk(child, fmt.Sprintf("%s.%d", path, i)); err != nil {
					return err
				}
			}
		case string:
			for _, secret := range w.secrets {
				if len(secret) > 0 && strings.Contains(x, secret) && !allowed(path) {
					return fmt.Errorf("rendered resource contains credentials outside required operator enrollment fields")
				}
			}
		}
		return nil
	}
	return walk(v, "")
}

// The bootstrap identity registers only the supported node/admin/client types.
// It cannot delegate registration or perform revocation/affiliation management.
func restrictRegistrar(v Object) {
	for _, kind := range []string{"ca", "tlsCA"} {
		registry := object(object(object(v["spec"])[kind])["registry"])
		identities, _ := registry["identities"].([]any)
		for _, identity := range identities {
			object(identity)["attrs"] = Object{"hf.Registrar.Roles": "peer,orderer,admin,client", "hf.Registrar.DelegateRoles": "", "hf.Registrar.Attributes": "", "hf.Revoker": false, "hf.IntermediateCA": false, "hf.GenCRL": false, "hf.AffiliationMgr": false}
		}
	}
}
