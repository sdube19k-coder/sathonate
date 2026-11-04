package kubernetesfabric

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type ArgumentEvidence struct {
	Samples              int  `json:"samples"`
	CACommands           int  `json:"caCommands"`
	ProtectedCLICommands int  `json:"protectedCLICommands"`
	CredentialsAbsent    bool `json:"credentialsAbsent"`
}

// Inspect the actual child process; private logical arguments use stdin or RAM.
func (w *Worker) execute(cmd *exec.Cmd) error {
	if !w.Secure {
		return cmd.Run()
	}
	if e := cmd.Start(); e != nil {
		return e
	}
	var raw []byte
	var e error
	// Linux may expose an empty cmdline briefly while exec replaces the child.
	// Retry that observation, without treating constructed argv as live evidence.
	for attempt := 0; attempt < 10; attempt++ {
		raw, e = os.ReadFile("/proc/" + strconv.Itoa(cmd.Process.Pid) + "/cmdline")
		if e == nil && len(raw) > 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if e == nil && len(raw) > 0 {
		if w.argumentSamples == nil {
			w.argumentSamples = map[string]int{}
		}
		w.argumentSamples[filepath.Base(cmd.Path)]++
		bad := strings.Contains(string(raw), "PRIVATE KEY")
		for _, secret := range w.secrets {
			if len(secret) > 0 && strings.Contains(string(raw), secret) {
				bad = true
			}
		}
		if bad {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return secureDenied
		}
	}
	return cmd.Wait()
}
func (w *Worker) argumentEvidence() (*ArgumentEvidence, error) {
	evidence := &ArgumentEvidence{CACommands: w.argumentSamples["fabric-ca-client"], ProtectedCLICommands: w.argumentSamples["kubectl-hlf"], CredentialsAbsent: true}
	for _, count := range w.argumentSamples {
		evidence.Samples += count
	}
	if evidence.CACommands == 0 || evidence.ProtectedCLICommands == 0 {
		return nil, fmt.Errorf("protected process argument inspection incomplete (ca=%d, cli=%d, samples=%d)", evidence.CACommands, evidence.ProtectedCLICommands, evidence.Samples)
	}
	return evidence, nil
}
