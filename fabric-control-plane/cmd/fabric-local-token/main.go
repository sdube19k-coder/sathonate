// One-time interactive local credential provisioning. Never run with command
// output logging; only the SHA256 grant file belongs on the server.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sathonate.local/fabric-control-plane/internal/identity"
	"strings"
	"time"
)

func main() {
	environment := flag.String("mode", "", "must explicitly be local-development")
	developer := flag.String("developer", "", "developer identity")
	tenant := flag.String("tenant", "", "tenant identity")
	project := flag.String("project", "", "fixed project identity")
	permissions := flag.String("permissions", "read", "comma-separated read,write,deploy,retry,cancel,cleanup")
	output := flag.String("grants", "", "new server grant file (never overwritten)")
	lifetime := flag.Duration("lifetime", time.Hour, "maximum 24h")
	flag.Parse()
	token, digest, e := identity.GenerateLocalToken()
	if e != nil || *lifetime <= 0 || *lifetime > 24*time.Hour {
		fmt.Fprintln(os.Stderr, "token generation rejected")
		os.Exit(1)
	}
	grants := []identity.LocalGrant{{TokenSHA256: digest, Developer: *developer, Tenant: *tenant, Project: *project, Permissions: strings.Split(*permissions, ","), ExpiresAt: time.Now().Add(*lifetime)}}
	if _, e = identity.NewLocalTokens(*environment, grants); e != nil {
		fmt.Fprintln(os.Stderr, "local grant configuration rejected")
		os.Exit(1)
	}
	f, e := os.OpenFile(*output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		fmt.Fprintln(os.Stderr, "new grant file required")
		os.Exit(1)
	}
	e = json.NewEncoder(f).Encode(grants)
	if e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e != nil || closeErr != nil {
		fmt.Fprintln(os.Stderr, "grant persistence failed")
		os.Exit(1)
	}
	fmt.Println(token) // Client credential shown once; never stored by this command.
}
