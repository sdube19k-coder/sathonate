package main

import (
	"context"
	"encoding/json"
	"flag"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"sathonate.local/fabric-control-plane/internal/cloud"
	"sathonate.local/fabric-control-plane/internal/deployment"
	"sathonate.local/fabric-control-plane/internal/drafts"
	"sathonate.local/fabric-control-plane/internal/httpapi"
	"sathonate.local/fabric-control-plane/internal/identity"
	"sathonate.local/fabric-control-plane/internal/localfabric"
	"sathonate.local/fabric-control-plane/internal/persistence"
	"sathonate.local/fabric-control-plane/internal/readiness"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "loopback address for local development")
	data := flag.String("data", "data/drafts.json", "single-process development draft store")
	mode := flag.String("mode", "local-development", "local-development or production")
	localOIDC := flag.Bool("local-oidc", false, "explicit loopback-only real OIDC development qualification; no production execution")
	localGrants := flag.String("local-token-grants", "", "explicit local-development-only file of hashed developer/tenant/project bearer grants")
	secureConfig := flag.String("secure-local-config", "", "trusted disposable kind / pinned worker configuration for secure-local")
	secureEKSConfig := flag.String("secure-eks-config", "", "administrator-owned exact network/SSO/qualified image binding for secure-eks qualification")
	approvalFile := flag.String("eks-approval", "", "administrator-owned target-digest qualification approvals JSON")
	cloudConfig := flag.String("cloud-config", "", "trusted server-side EKS target registry JSON")
	migrate := flag.Bool("migrate", false, "apply initial PostgreSQL schema and exit")
	localRoot := flag.String("local-runtime", "data/local-fabric", "isolated local Docker runtime directory")
	deploymentMode := flag.String("deployment-mode", "disabled", "disabled, mock, local-kubernetes, eks-qualification; real production mode remains disabled")
	localKubeconfig := flag.String("local-kubeconfig", "", "explicit kubeconfig for kind-smp-qualification")
	workerImage := flag.String("local-worker-image", "sathonate/fabric-worker:local", "preloaded local worker image")
	cloudExecution := flag.Bool("enable-cloud-execution", false, "explicit opt-in to qualified real AWS/EKS execution; unavailable until qualification")
	deploymentRoot := flag.String("deployment-data", "data/deployments", "single-process development deployment journal")
	flag.Parse()
	explicitMode := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "mode" {
			explicitMode = true
		}
	})
	if *localGrants != "" && !explicitMode {
		log.Fatal("local bearer mode requires explicit -mode local-development")
	}
	if *localOIDC && (!explicitMode || *mode != "local-development" || *localGrants != "" || *cloudConfig != "" || (*deploymentMode != "disabled" && *deploymentMode != "secure-local" && *deploymentMode != "secure-eks")) {
		log.Fatal("local OIDC requires explicit local-development, no local token grants or cloud targets, and disabled or guarded secure-local execution")
	}
	production := *mode == "production"
	if *localGrants != "" && (*cloudConfig != "" || (*deploymentMode != "disabled" && *deploymentMode != "mock" && *deploymentMode != "secure-local")) {
		log.Fatal("local token qualification permits only disabled, mock or secure-local execution; cloud targets are forbidden")
	}
	if *localGrants != "" && *mode != "local-development" {
		log.Fatal("local token authentication is restricted to explicit local-development")
	}
	if *deploymentMode != "disabled" && *deploymentMode != "mock" && *deploymentMode != "real" && *deploymentMode != "local-kubernetes" && *deploymentMode != "eks-qualification" && *deploymentMode != "secure-local" && *deploymentMode != "secure-eks" {
		log.Fatal("invalid deployment mode")
	}
	if *deploymentMode == "real" || *cloudExecution {
		log.Fatal("production cloud execution is not qualified; use the separately approved EKS qualification mode")
	}
	if production && *deploymentMode != "disabled" {
		log.Fatal("development deployment runners are restricted to local-development mode")
	}
	if *mode != "local-development" && !production {
		log.Fatal("invalid service mode")
	}
	host, port, e := net.SplitHostPort(*addr)
	if e != nil || net.ParseIP(host) == nil || (!production && !net.ParseIP(host).IsLoopback()) {
		log.Fatal("local-development API must bind a loopback IP address")
	}
	startup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	connection := os.Getenv("DATABASE_URL")
	var database *persistence.Database
	if connection != "" {
		database, e = persistence.Open(startup, connection, production)
		if e != nil {
			log.Fatal("PostgreSQL connection failed; verify database access and TLS configuration")
		}
		defer database.Close()
	}
	if *migrate {
		if database == nil {
			log.Fatal("DATABASE_URL is required for migrations")
		}
		if e = database.Migrate(startup); e != nil {
			log.Fatal("database migration failed")
		}
		log.Print("PostgreSQL schema migration complete")
		return
	}
	if production && database == nil {
		log.Fatal("production mode requires PostgreSQL; local file storage is not allowed")
	}
	configuration := identity.Config{Mode: "local-development"}
	if production || *localOIDC {
		configuration = identity.Config{Mode: "okta", Environment: *mode, Issuer: os.Getenv("OKTA_ISSUER"), ClientID: os.Getenv("OKTA_CLIENT_ID"), Audience: os.Getenv("OKTA_AUDIENCE"), TenantClaim: os.Getenv("OKTA_TENANT_CLAIM"), ProjectClaim: os.Getenv("OKTA_PROJECT_CLAIM")}
	}
	if *localOIDC {
		configuration.ServerBoundProject = true
		configuration.LocalIdentityProbe = true
		if database == nil {
			log.Fatal("local OIDC requires PostgreSQL")
		}
	}
	if production || *localOIDC {
		source := persistence.MembershipSource{Pool: database.Pool()}
		if e = source.Ready(startup); e != nil {
			log.Fatal("OIDC membership registry unavailable; run the explicit administrative migration")
		}
		configuration.MembershipSource = source
	}
	authentication, e := identity.New(startup, configuration)
	if e != nil {
		log.Fatal(e)
	}
	authConfig := authentication.Config()
	middleware := authentication.Middleware
	if *localGrants != "" {
		if database == nil {
			log.Fatal("local bearer authentication requires PostgreSQL project isolation")
		}
		localAuth, err := identity.LoadLocalTokens(*mode, *localGrants)
		if err != nil {
			log.Fatal("local token grant configuration rejected")
		}
		authConfig = localAuth.Config()
		middleware = localAuth.Middleware
	}
	registry, e := cloud.LoadRegistry(*cloudConfig)
	if e != nil {
		log.Fatal("invalid EKS target registry configuration")
	}
	var resolve httpapi.RepositoryResolver
	storage := "single-process local drafts"
	if database != nil {
		storage = "PostgreSQL tenant-scoped"
		resolve = func(r *http.Request) httpapi.Repository {
			return database.ForTenant(r.Context(), identity.FromContext(r.Context()).StorageTenant())
		}
	} else {
		store, err := drafts.Open(*data)
		if err != nil {
			log.Fatal("could not open local draft store")
		}
		resolve = httpapi.FileResolver(store)
	}
	allowedHosts := []string{net.JoinHostPort(host, port), net.JoinHostPort("localhost", port)}
	origins := []string{"http://" + allowedHosts[0], "http://" + allowedHosts[1], "http://localhost:5173", "http://127.0.0.1:5173"}
	if production {
		allowedHosts = strings.Split(os.Getenv("API_ALLOWED_HOSTS"), ",")
		origins = strings.Split(os.Getenv("CONSOLE_ALLOWED_ORIGINS"), ",")
		if len(allowedHosts) == 0 || allowedHosts[0] == "" || len(origins) == 0 || origins[0] == "" {
			log.Fatal("production mode requires explicit API_ALLOWED_HOSTS and CONSOLE_ALLOWED_ORIGINS")
		}
		for _, origin := range origins {
			if !strings.HasPrefix(origin, "https://") {
				log.Fatal("production console origins must use HTTPS")
			}
		}
	}
	var local *localfabric.Manager
	if !production && *localGrants == "" && !*localOIDC {
		local, e = localfabric.New(*localRoot)
		if e != nil {
			log.Fatal("cannot initialize local Fabric runtime")
		}
	}
	var deployments *deployment.Manager
	if *deploymentMode == "secure-eks" {
		if !*localOIDC || authConfig.Mode != "okta" || database == nil || *secureEKSConfig == "" {
			log.Fatal("secure EKS qualification requires server-owned OIDC project memberships, PostgreSQL and an explicit approved secure EKS configuration")
		}
		runner, err := deployment.LoadSecureEKSRunner(*secureEKSConfig)
		if err != nil {
			log.Fatal("secure EKS qualification configuration rejected")
		}
		deployments, e = deployment.OpenDatabase(startup, database.Pool(), runner)
		if e != nil {
			log.Fatal("secure EKS qualification executor initialization failed")
		}
		defer deployments.Close()
	}

	if *deploymentMode == "secure-local" {
		if (authConfig.Mode != "local-token" && !(*localOIDC && authConfig.Mode == "okta")) || database == nil || *secureConfig == "" {
			log.Fatal("secure-local requires explicit local bearer grants, PostgreSQL and trusted secure configuration")
		}
		runner, err := deployment.LoadSecureRunner(*secureConfig)
		if err != nil {
			log.Fatal("secure local runtime configuration rejected")
		}
		deployments, e = deployment.OpenDatabase(startup, database.Pool(), runner)
		if e != nil {
			log.Fatal("secure local executor initialization failed")
		}
		defer deployments.Close()
	}
	if *deploymentMode == "local-kubernetes" {
		if database == nil || *localKubeconfig == "" {
			log.Fatal("local Kubernetes execution requires PostgreSQL and an explicit local kubeconfig")
		}
		deployments, e = deployment.OpenDatabase(startup, database.Pool(), deployment.KubernetesRunner{Kubeconfig: *localKubeconfig, Image: *workerImage})
		if e != nil {
			log.Fatal("cannot initialize PostgreSQL-backed local Kubernetes executor")
		}
		defer deployments.Close()
	}
	if *deploymentMode == "eks-qualification" {
		if database == nil || *cloudConfig == "" || *approvalFile == "" {
			log.Fatal("EKS qualification requires PostgreSQL, an approved registry and an approval file")
		}
		file, err := os.Open(*approvalFile)
		if err != nil {
			log.Fatal("EKS approval file unavailable")
		}
		var approvals []cloud.Approval
		decoder := json.NewDecoder(file)
		decoder.DisallowUnknownFields()
		err = decoder.Decode(&approvals)
		var extra any
		if err == nil && decoder.Decode(&extra) != io.EOF {
			err = io.ErrUnexpectedEOF
		}
		file.Close()
		if err != nil || len(approvals) == 0 {
			log.Fatal("invalid EKS approval file")
		}
		approved := false
		for _, target := range registry.Targets {
			if target.Operator != nil {
				for _, approval := range approvals {
					if approval.Check(target, time.Now().UTC()) == nil {
						approved = true
					}
				}
			}
		}
		if !approved {
			log.Fatal("no registered operator target has a current matching qualification approval")
		}
		deployments, e = deployment.OpenDatabase(startup, database.Pool(), deployment.EKSRunner{Registry: registry, Approvals: approvals})
		if e != nil {
			log.Fatal("cannot initialize EKS qualification executor")
		}
		defer deployments.Close()
	}
	if *deploymentMode == "mock" {
		deployments, e = deployment.Open(*deploymentRoot, deployment.MockRunner{Delay: 700 * time.Millisecond})
		if e != nil {
			log.Fatal("cannot initialize mock deployment runner")
		}
		defer deployments.Close()
	}
	var history func(context.Context, string) (*deployment.Manager, error)
	if *localOIDC && *deploymentMode == "disabled" {
		history = func(ctx context.Context, scope string) (*deployment.Manager, error) {
			return deployment.OpenHistoryDatabase(ctx, database.Pool(), []string{scope})
		}
	}
	handler := httpapi.NewConfigured(resolve, allowedHosts, origins, httpapi.Options{ReadHistory: history, Auth: authConfig, Registry: registry, Storage: storage, Local: local, Deployments: deployments, Readiness: readiness.Environment()})
	server := &http.Server{Addr: *addr, Handler: middleware(handler), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 150 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	log.Printf("Fabric SMP API listening at %s (mode=%s; production cloud release gate remains closed)", *addr, *mode)
	if e = server.ListenAndServe(); e != nil && e != http.ErrServerClosed {
		log.Fatal(e)
	}
}
