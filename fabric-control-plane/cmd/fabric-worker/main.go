package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sathonate.local/fabric-control-plane/internal/domain"
	"sathonate.local/fabric-control-plane/internal/kubernetesfabric"
	"time"
)

func main() {
	input := flag.String("network", "/inputs/network.json", "validated network JSON")
	runtimeFile := flag.String("runtime", "", "trusted adapter runtime JSON")
	namespace := flag.String("namespace", "", "dedicated local namespace")
	secure := flag.Bool("secure-local", false, "explicit protected application issuer profile")
	lifecycleFile := flag.String("lifecycle", "", "approved bounded local lifecycle state")
	flag.Parse()
	if len(*namespace) < 6 || ((*namespace)[:6] != "smp-n-" && (!*secure || (*namespace)[:6] != "smp-s-")) {
		fmt.Fprintln(os.Stderr, "worker requires a dedicated smp-n- namespace")
		os.Exit(1)
	}
	file, err := os.Open(*input)
	if err != nil {
		fmt.Fprintln(os.Stderr, "network input unavailable")
		os.Exit(1)
	}
	spec, err := domain.Decode(file)
	file.Close()
	if err != nil {
		fmt.Fprintln(os.Stderr, "invalid network input")
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()
	encoder := json.NewEncoder(os.Stdout)
	var runtime kubernetesfabric.Runtime
	if *runtimeFile != "" {
		raw, e := os.ReadFile(*runtimeFile)
		if e != nil || json.Unmarshal(raw, &runtime) != nil || runtime.Validate(spec) != nil {
			fmt.Fprintln(os.Stderr, "invalid approved runtime")
			os.Exit(1)
		}
		// The pinned scoped CLI performs namespace-only discovery. Kubernetes RBAC
		// remains authoritative even if the tool is invoked with another namespace.
		if err = os.Setenv("SMP_WORKER_NAMESPACE", *namespace); err != nil {
			fmt.Fprintln(os.Stderr, "cannot configure scoped worker")
			os.Exit(1)
		}
	}
	worker := kubernetesfabric.Worker{Secure: *secure, Namespace: *namespace, Runtime: runtime, Spec: spec, Output: func(event kubernetesfabric.Event) error { return encoder.Encode(event) }}
	if *lifecycleFile != "" {
		raw, e := os.ReadFile(*lifecycleFile)
		var state kubernetesfabric.LifecycleState
		if !*secure || e != nil || json.Unmarshal(raw, &state) != nil || state.Validate() != nil {
			fmt.Fprintln(os.Stderr, "invalid local lifecycle state")
			os.Exit(1)
		}
		worker.Lifecycle = &state
	}
	if err = worker.Run(ctx); err != nil {
		_ = encoder.Encode(kubernetesfabric.Event{Message: err.Error()})
		os.Exit(1)
	}
}
