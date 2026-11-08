import { useEffect, useState, useRef } from "react";
import OperationDiagnosticsButton from "./OperationDiagnostics";
import {
  Activity,
  ArrowLeft,
  ArrowRight,
  Boxes,
  Check,
  CheckCircle2,
  ChevronDown,
  Cloud,
  Download,
  Code2,
  HelpCircle,
  LoaderCircle,
  Menu,
  Network as NetworkIcon,
  Plus,
  Save,
  Search,
  Server,
  Settings,
  ShieldCheck,
  X,
} from "lucide-react";
import {
  api,
  type CloudTarget,
  type LocalState,
  type Operation,
  type RuntimeHealth,
  type DeploymentState,
} from "./api";
import type { Issue, Meta, Network } from "./types";
import { fromNetwork, stepError } from "./network-form";
import {
  cidrError,
  deploymentSpec,
  flowSteps,
  initialDraft,
  secureDraft,
  sameDesign,
  readDraft,
  selectProvider,
  type FlowDraft,
  type Provider,
} from "./smp-model";
import {
  Card,
  CloudAccount,
  Kubernetes,
  NetworkDesign,
  Note,
  ProviderMark,
  Review,
  Status,
} from "./SmpScreens";
import { DashboardScreen, ProgressScreen } from "./SmpRuntime";
import { formatDate } from "./components";
import { useWorkspace, workspaceKey } from "./workspace";
import { SignOut } from "./AuthGate";
import { useDialog } from "./use-dialog";
import { readWithRecovery } from "./read-recovery";
import { CloudReadiness } from "./LaunchReadiness";
import CustomNetworks from "./CustomNetworks";
import "./smp.css";
import "./reference-design.css";
import "./console-theme.css";

type View =
  | "create"
  | "networks"
  | "nodes"
  | "chaincode"
  | "operations"
  | "settings"
  | "custom";
const titles = [
  "Connect Your Cloud Account",
  "Kubernetes Configuration",
  "Network Design",
  "Review & Cost Estimate",
  "Deployment Progress",
  "Network Dashboard",
];
const subtitles = [
  "Choose your provider and connect a secure identity for your Hyperledger Fabric network.",
  "Set up or select the runtime where your Hyperledger Fabric network will be deployed.",
  "Configure your network. Add organizations, peers, orderers, certificate authorities and channels.",
  "Review your network configuration. Go back and edit any section before creating the network.",
  "Follow actual deployment stages, timestamps and events from the backend worker.",
  "View live network health, topology and resources, and manage your deployed network.",
];

export default function SmpApp() {
  const project = useWorkspace(),
    secure = project !== null;
  const submitting = useRef(false),
    retrying = useRef(false);
  const [verification, setVerification] = useState<DeploymentState | null>(
    null,
  );
  const [draft, setDraft] = useState<FlowDraft>(readDraft);
  const [step, setStep] = useState(() => {
    const n = Number(location.hash.match(/^#\/create\/(\d)$/)?.[1]);
    return n >= 1 &&
      n <= 6 &&
      (n <= 4 || draft.provider === "local" || draft.deploymentOperationId)
      ? n - 1
      : 0;
  });
  const [view, setView] = useState<View>("create");
  const [customNetworkId, setCustomNetworkId] = useState("");
  const [selectedOperation, setSelectedOperation] = useState(
    draft.deploymentOperationId ?? "",
  );
  const [meta, setMeta] = useState<Meta | null>(null),
    [targets, setTargets] = useState<CloudTarget[]>([]),
    [networks, setNetworks] = useState<Network[]>([]);
  const [runtime, setRuntime] = useState<LocalState | null>(null),
    [health, setHealth] = useState<RuntimeHealth | null>(null),
    [operation, setOperation] = useState<Operation | null>(null),
    [history, setHistory] = useState<Operation[]>([]);
  const [saved, setSaved] = useState<Network | null>(null),
    [issues, setIssues] = useState<Issue[]>([]),
    [valid, setValid] = useState(false),
    [testResult, setTestResult] = useState("");
  const [error, setError] = useState(""),
    [toast, setToast] = useState(""),
    [busy, setBusy] = useState(false),
    [healthBusy, setHealthBusy] = useState(false),
    [menu, setMenu] = useState(false),
    [help, setHelp] = useState(false);
  const [confirmCleanup, setConfirmCleanup] = useState(false);
  const [logs, setLogs] = useState(""),
    [node, setNode] = useState("peer1"),
    [search, setSearch] = useState("");
  useEffect(() => {
    if (!menu) return;
    const close = (event: KeyboardEvent) => {
      if (event.key === "Escape") setMenu(false);
    };
    window.addEventListener("keydown", close);
    return () => window.removeEventListener("keydown", close);
  }, [menu]);
  const helpRef = useDialog(help, () => setHelp(false));
  const cleanupRef = useDialog(confirmCleanup, () => setConfirmCleanup(false));
  useEffect(() => {
    if (
      health?.peers.length &&
      !health.peers.some((peer) => peer.peer === node)
    )
      setNode(health.peers[0].peer);
  }, [health, node]);
  const local = draft.provider === "local",
    kube = draft.provider === "kubernetes",
    running =
      runtime?.phase === "starting" ||
      runtime?.phase === "stopping" ||
      runtime?.phase === "verifying";
  useEffect(() => {
    try {
      sessionStorage.setItem(workspaceKey(), JSON.stringify(draft));
    } catch {
      /* Server-side design saving remains available. */
    }
  }, [draft]);
  useEffect(() => {
    let active = true;
    void readWithRecovery(() =>
      Promise.all([api.meta(), api.cloudTargets(), api.networks()]),
    )
      .then(([m, t, n]) => {
        if (active) {
          setMeta(m);
          setTargets(t.items);
          setNetworks(n.items);
          const resumed = n.items.find((item) => item.id === draft.recordId);
          if (resumed) {
            setSaved(resumed);
            if (secure && !draft.deploymentOperationId)
              void api
                .operations()
                .then((result) => {
                  if (!active) return;
                  const latest = result.items
                    .filter(
                      (op) =>
                        op.networkId === resumed.id &&
                        op.kind === "secure-local-deploy",
                    )
                    .sort((a, b) => b.createdAt.localeCompare(a.createdAt))[0];
                  if (latest) {
                    setSelectedOperation(latest.operationId);
                    setDraft((old) => ({
                      ...old,
                      deploymentOperationId: latest.operationId,
                    }));
                    setStep(4);
                  }
                })
                .catch(() =>
                  setError(
                    "Cannot restore operation history. Reconnect and reload.",
                  ),
                );
          }
        }
      })
      .catch((e) => {
        if (active) setError(e.message);
      });
    return () => {
      active = false;
    };
  }, []);
  useEffect(() => {
    if (
      meta?.mode !== "local-development" &&
      meta?.mode !== "local-token" &&
      meta?.mode !== "okta"
    )
      return;
    let active = true;
    const load = async () => {
      try {
        if (kube) {
          const ops = await api.operations();
          if (!active) return;
          setHistory(ops.items);
          const id = selectedOperation || draft.deploymentOperationId;
          if (id) {
            const [op, state] = await Promise.all([
              api.operation(id),
              api.deployment(id),
            ]);
            if (!active) return;
            setOperation(op);
            setVerification(state);
            setError("");
            setRuntime({
              operationId: id,
              phase: state.phase,
              ready: state.ready,
              verified: state.verified,
              version: state.network.spec.fabricVersion,
              channel: state.network.spec.channels[0].id,
              transactionId: state.transactionId,
              verifiedAt: state.checkedAt,
              events: [],
              containers: [],
              dockerAvailable: false,
            });
            if (state.health)
              setHealth((previous) =>
                previous?.checkedAt === state.health?.checkedAt
                  ? previous
                  : state.health!,
              );
          }
          return;
        }
        if (!local) {
          const ops = await api.operations();
          if (active) setHistory(ops.items);
          return;
        }
        const [r, ops] = await Promise.all([
          api.localStatus(),
          api.operations(),
        ]);
        if (!active) return;
        setRuntime(r);
        setHistory(ops.items);
        if (selectedOperation || r.operationId) {
          const op = await api.operation(selectedOperation || r.operationId!);
          if (active) setOperation(op);
        }
      } catch (reason) {
        if (active)
          setError(
            reason instanceof Error
              ? reason.message
              : "Connection interrupted. Displaying the last recorded state; reconnect to refresh.",
          );
        /* Never synthesize a successful runtime response. */
      }
    };
    void load();
    const id = setInterval(() => void load(), 2500);
    return () => {
      active = false;
      clearInterval(id);
    };
  }, [meta, selectedOperation, kube, local]);
  useEffect(() => {
    if (!toast) return;
    const id = setTimeout(() => setToast(""), 5000);
    return () => clearTimeout(id);
  }, [toast]);
  useEffect(() => {
    if (view === "create")
      window.history.replaceState(null, "", `#/create/${step + 1}`);
    window.scrollTo({ top: 0, behavior: "instant" });
  }, [step, view]);
  useEffect(() => {
    if (step !== 5 && view !== "chaincode" && view !== "nodes") return;
    if (kube && !selectedOperation) return;
    let active = true;
    const load = async () => {
      setHealthBusy(true);
      try {
        const h = kube
          ? await api.deploymentHealth(selectedOperation)
          : await api.localHealth();
        if (active) {
          setHealth(h);
          setError("");
        }
      } catch (e) {
        if (active)
          setError(e instanceof Error ? e.message : "Health query failed");
      } finally {
        if (active) setHealthBusy(false);
      }
    };
    void load();
    const timer = setInterval(() => void load(), 20000);
    return () => {
      active = false;
      clearInterval(timer);
    };
  }, [step, view, kube, selectedOperation]);
  const update = (d: FlowDraft) => {
    if (
      draft.provider === "kubernetes" &&
      draft.deploymentOperationId &&
      JSON.stringify(d.form) !== JSON.stringify(draft.form)
    ) {
      d = { ...d, deploymentOperationId: undefined };
      setSelectedOperation("");
      setOperation(null);
      setRuntime(null);
      setHealth(null);
    }
    setDraft(d);
    setValid(false);
    setIssues([]);
    setTestResult("");
    setError("");
  };
  const navigate = (v: View) => {
    setView(v);
    setMenu(false);
    setError("");
  };
  const go = (n: number) => {
    if (n === 5 && (!runtime?.verified || runtime.phase !== "running")) return;
    if (n > 3 && ((!local && !kube) || (!operation && !runtime?.verified)))
      return;
    setStep(n);
    setError("");
    setTestResult("");
    window.scrollTo({ top: 0, behavior: "instant" });
  };
  const newFlow = () => {
    setSelectedOperation("");
    setOperation(null);
    setRuntime(null);
    setHealth(null);
    setSaved(null);
    setDraft(secure ? secureDraft() : initialDraft());
    setStep(0);
    setView("create");
    setValid(false);
    setIssues([]);
    setError("");
  };
  const provider = (p: Provider) => {
    if (secure && p !== "kubernetes") return;
    setSelectedOperation("");
    setOperation(null);
    setRuntime(null);
    setHealth(null);
    setSaved(null);
    update(selectProvider(draft, p));
  };
  const validate = async () => {
    setBusy(true);
    setError("");
    try {
      for (const s of [0, 1, 2, 3]) {
        const e = stepError(draft.form, s);
        if (e) throw new Error(e);
      }
      const result = await api.validate(deploymentSpec(draft));
      setIssues(result.issues);
      const accepted =
        result.valid &&
        (!secure ||
          (result.deploymentProfile?.id === "smp-secure-local-v1" &&
            result.deploymentProfile.executionEnabled));
      setValid(accepted);
      if (!accepted && result.valid)
        throw new Error(
          "The secure local runner is not configured on this API.",
        );
      setTestResult(
        result.valid
          ? "Backend topology validation passed. Infrastructure checks are separate."
          : "Some topology settings need attention.",
      );
      if (!result.valid)
        throw new Error(
          "Some settings need attention. Review the validation findings.",
        );
      return true;
    } catch (e) {
      setError(e instanceof Error ? e.message : "Validation failed");
      return false;
    } finally {
      setBusy(false);
    }
  };
  const next = async () => {
    setError("");
    if (step === 0) {
      if (
        !local &&
        !kube &&
        draft.accountId &&
        !/^\d{12}$/.test(draft.accountId)
      ) {
        setError("AWS Account ID must contain 12 digits.");
        return;
      }
      go(1);
    } else if (step === 1) {
      const e = cidrError(draft);
      if (e) {
        setError(e);
        return;
      }
      go(2);
    } else if (step === 2) {
      if (await validate()) go(3);
    }
  };
  const save = async () => {
    if (!(await validate())) return;
    setBusy(true);
    try {
      const desired = deploymentSpec(draft);
      const n = saved
        ? sameDesign(saved.desired, desired)
          ? saved
          : await api.update(saved, desired)
        : await api.create(desired);
      setSaved(n);
      setDraft((previous) => ({
        ...previous,
        recordId: n.id,
        form: {
          ...previous.form,
          networkId: n.desired.metadata.id,
          original: n.desired,
        },
      }));
      setNetworks((old) => [n, ...old.filter((x) => x.id !== n.id)]);
      setToast(
        "Network design, cloud identity references and cluster configuration saved.",
      );
      return n;
    } catch (e) {
      setError(e instanceof Error ? e.message : "Draft could not be saved");
    } finally {
      setBusy(false);
    }
  };
  const prepareDeployment = async () => {
    const n = await save();
    if (!n) return;
    setBusy(true);
    try {
      const p = await api.deploymentPackage(n);
      const url = URL.createObjectURL(
        new Blob([JSON.stringify(p, null, 2)], { type: "application/json" }),
      );
      const link = document.createElement("a");
      link.href = url;
      link.download = `${n.desired.metadata.id}-deployment-r${n.revision}.json`;
      link.click();
      setTimeout(() => URL.revokeObjectURL(url), 1000);
      setTestResult(
        p.checks
          .filter((c) => c.status === "blocked")
          .map((c) => c.message)
          .join(" "),
      );
      setToast("Deployment JSON prepared. No infrastructure has been created.");
    } catch (e) {
      setError(
        e instanceof Error ? e.message : "Deployment preparation failed",
      );
    } finally {
      setBusy(false);
    }
  };
  const test = async () => {
    setBusy(true);
    setError("");
    try {
      if (secure) {
        const result = await api.validate(deploymentSpec(draft));
        setTestResult(
          result.valid && result.deploymentProfile?.executionEnabled
            ? "Secure local profile is available. Cluster and material checks run during deployment."
            : "Secure runner unavailable or configuration invalid.",
        );
      } else if (kube) {
        const targets = await api.cloudTargets();
        setTestResult(
          targets.items.some(
            (t) => t.id === "smp-qualification" && t.executionEnabled,
          )
            ? "Local Kubernetes runner configured. Live cluster checks run before deployment; no AWS identity is used."
            : "Local Kubernetes runner is disabled. Start the API with the documented local configuration.",
        );
      } else if (local) {
        const r = await api.localStatus();
        setRuntime(r);
        setTestResult(
          r.dockerAvailable
            ? "Connected to the Linux Docker engine. The local profile is available."
            : "Docker is unavailable. Start Docker Desktop with its Linux engine.",
        );
      } else if (saved && draft.targetRef) {
        const r = await api.cloudPreflight(saved);
        setTestResult(
          r.ready
            ? "Connected and verified."
            : r.checks
                .filter((c) => c.status === "blocked")
                .map((c) => c.message)
                .slice(0, 2)
                .join(" "),
        );
      } else
        setTestResult(
          "Not connected. Cloud role onboarding is not enabled. Choose a registered target or continue with a design only.",
        );
    } catch (e) {
      setTestResult(e instanceof Error ? e.message : "Connection check failed");
    } finally {
      setBusy(false);
    }
  };
  const create = async () => {
    if (submitting.current || (secure && draft.deploymentOperationId)) return;
    submitting.current = true;
    try {
      await createNetwork();
    } finally {
      submitting.current = false;
    }
  };
  const createNetwork = async () => {
    if (!local && !kube) return;
    const network = kube ? await save() : null;
    if (kube && !network) return;
    setBusy(true);
    setError("");
    try {
      const op =
        kube && network ? await api.deploy(network) : await api.localStart();
      setSelectedOperation(op.operationId);
      setOperation(op);
      if (kube) {
        setRuntime(null);
        setHealth(null);
        setDraft((d) => ({ ...d, deploymentOperationId: op.operationId }));
      } else {
        setRuntime(await api.localStatus());
      }
      setStep(4);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Deployment could not start");
    } finally {
      setBusy(false);
    }
  };
  const refreshHealth = async () => {
    setHealthBusy(true);
    try {
      if (kube && selectedOperation) {
        setHealth(await api.deploymentHealth(selectedOperation));
      } else {
        setHealth(await api.localHealth());
        setRuntime(await api.localStatus());
      }
      setError("");
    } catch (e) {
      setError(e instanceof Error ? e.message : "Health check failed");
    } finally {
      setHealthBusy(false);
    }
  };
  const transaction = async () => {
    if (secure && selectedOperation) {
      try {
        const recorded = await api.deployment(selectedOperation);
        setVerification(recorded);
        setToast(
          "Loaded server-recorded transaction and peer readback verification.",
        );
      } catch (reason) {
        setError(
          reason instanceof Error ? reason.message : "Verification unavailable",
        );
      }
      return;
    }
    if (kube) {
      await create();
      return;
    }
    setBusy(true);
    try {
      const op = await api.localTest();
      setOperation(op);
      setSelectedOperation(op.operationId);
      setView("create");
      setStep(4);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Transaction check failed");
    } finally {
      setBusy(false);
    }
  };
  const stop = async () => {
    setBusy(true);
    try {
      if (kube && selectedOperation) {
        await api.deploymentCleanup(selectedOperation);
        setRuntime(null);
        setHealth(null);
        setDraft((d) => ({ ...d, deploymentOperationId: undefined }));
        setSelectedOperation("");
        setStep(3);
        setToast(
          "Local Kubernetes network removed. PostgreSQL operation history is retained.",
        );
        return;
      }
      const op = await api.localStop();
      setOperation(op);
      setSelectedOperation(op.operationId);
      setView("create");
      setStep(4);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Stop failed");
    } finally {
      setBusy(false);
    }
  };
  const retry = async () => {
    if (!operation || retrying.current) return;
    retrying.current = true;
    setBusy(true);
    try {
      const op = await api.retryOperation(operation.operationId);
      setOperation(op);
      setSelectedOperation(op.operationId);
      if (kube)
        setDraft((d) => ({ ...d, deploymentOperationId: op.operationId }));
      setError("");
    } catch (e) {
      setError(e instanceof Error ? e.message : "Retry failed");
    } finally {
      retrying.current = false;
      setBusy(false);
    }
  };
  const openLocal = () => {
    update(secure ? secureDraft() : selectProvider(draft, "local"));
    setView("create");
    setStep(runtime?.verified ? 5 : 0);
  };
  const resume = (n: Network) => {
    if (n.desired.spec.deployment.engine === "native") {
      setError(
        "This saved private-server design is outside the supported cloud/local guided profiles. It remains unchanged.",
      );
      return;
    }
    setSaved(n);
    setSelectedOperation("");
    setOperation(null);
    setRuntime(null);
    setHealth(null);
    setDraft({
      ...(secure ? secureDraft() : initialDraft()),
      ...n.desired.spec.deployment.cloud,
      recordId: n.id,
      provider:
        n.desired.spec.deployment.provider === "local"
          ? "kubernetes"
          : (n.desired.spec.deployment.provider ?? "aws"),
      form: fromNetwork(n.desired),
      targetRef: n.desired.spec.deployment.targetRef,
    });
    setView("create");
    setStep(2);
    setValid(false);
    setIssues(n.issues);
    if (n.desired.spec.deployment.provider === "local") {
      void api
        .operations()
        .then((ops) => {
          const latest = ops.items.find(
            (op) =>
              op.networkId === n.id &&
              ["local-kubernetes-deploy", "secure-local-deploy"].includes(
                op.kind,
              ),
          );
          if (!latest) return;
          setSelectedOperation(latest.operationId);
          setOperation(latest);
          setDraft((current) => ({
            ...current,
            deploymentOperationId: latest.operationId,
          }));
          setStep(4);
        })
        .catch((e) => setError(e.message));
    }
  };
  return (
    <div
      className={`smp smp-reference ${view === "create" ? "is-wizard" : "is-workspace"} ${menu ? "navigation-open" : ""}`}
      data-step={step + 1}
    >
      <div className="smp-main">
        <a
          className="smp-brand"
          href="#/create/1"
          onClick={(e) => {
            e.preventDefault();
            go(0);
            setView("create");
          }}
        >
          <span className="smp-brand-icon">
            <Boxes size={29} />
          </span>
          <span>
            Sathonate<small>FABRIC SMP</small>
          </span>
        </a>
        <header className="smp-header">
          <button
            className="smp-mobile-menu smp-icon-btn"
            aria-label={menu ? "Close navigation" : "Open navigation"}
            aria-expanded={menu}
            aria-controls="workspace-navigation"
            onClick={() => setMenu(!menu)}
          >
            <Menu size={22} />
          </button>

          <strong className="smp-header-title">
            {view === "create"
              ? "Create Hyperledger Fabric Network"
              : "Fabric SMP Workspace"}
          </strong>
          <div className="smp-user">
            <span
              className={`smp-api-state ${meta ? "connected" : ""}`}
              role="img"
              aria-label={
                meta ? "Control plane connected" : "Connecting to API"
              }
              title={meta ? "Control plane connected" : "Connecting to API"}
            />

            <button
              className="smp-icon-btn"
              aria-label="Help"
              onClick={() => setHelp(true)}
            >
              <HelpCircle size={20} />
            </button>
            <span className="smp-avatar">SA</span>
            <div>
              <strong>
                {project
                  ? `${project.tenantId} / ${project.id}`
                  : "My workspace"}
              </strong>
              <small>
                {meta?.mode === "okta"
                  ? "Okta organization"
                  : "Local development"}
              </small>
            </div>
            <SignOut />
            <ChevronDown size={15} />
          </div>
        </header>
        {menu && (
          <button
            className="smp-nav-scrim"
            aria-label="Close navigation overlay"
            onClick={() => setMenu(false)}
          />
        )}
        <aside
          className="smp-app-navigation"
          id="workspace-navigation"
          aria-label="Workspace navigation"
        >
          <span className="smp-nav-caption">WORKSPACE</span>
          <nav aria-label="Main navigation">
            {(
              [
                ["networks", "Networks", NetworkIcon],
                ["custom", "Customer configurations", Boxes],
                ["nodes", "Nodes", Server],
                ["chaincode", "Chaincode", Code2],
                ["operations", "Operations", Activity],
                ["settings", "Cloud readiness", ShieldCheck],
              ] as const
            ).map(([id, label, Icon]) => (
              <button
                key={id}
                className={view === id ? "active" : ""}
                aria-current={view === id ? "page" : undefined}
                onClick={() => navigate(id)}
              >
                <Icon size={19} />
                <span>{label}</span>
                {view === id && <span className="smp-nav-dot" />}
              </button>
            ))}
          </nav>
          <div className="smp-nav-context">
            <ShieldCheck size={21} />
            <strong>
              {meta?.deploymentMode === "history-only"
                ? "History workspace"
                : "Secure control plane"}
            </strong>
            <p>
              {meta?.deploymentMode === "history-only"
                ? "Recorded results. Live execution is disabled."
                : "Project-scoped access. Server-enforced permissions."}
            </p>
            <span>LOCAL DEVELOPMENT</span>
          </div>
        </aside>
        {view === "create" && (
          <nav className="smp-stepper" aria-label="Network creation steps">
            {flowSteps.map((s, i) => (
              <button
                key={s.title}
                className={`${i === step ? "current" : ""} ${i < step ? "done" : ""}`}
                onClick={() => {
                  go(i);
                  setMenu(false);
                }}
                disabled={
                  (i > 3 &&
                    ((!local && !kube) ||
                      (!operation && !runtime?.verified))) ||
                  (i === 5 &&
                    (!runtime?.verified || runtime.phase !== "running"))
                }
                aria-current={step === i ? "step" : undefined}
              >
                <span className="smp-step-number">
                  {i < step ? <Check size={16} /> : i + 1}
                </span>
                <span className="smp-step-copy">
                  <strong>
                    {i === 0
                      ? secure
                        ? "Project & Profile"
                        : "Connect Cloud Account"
                      : s.title}
                  </strong>
                  <small>{s.detail}</small>
                </span>
                {i < 5 && <i />}
              </button>
            ))}
          </nav>
        )}
        <main className="smp-content">
          {meta?.deploymentMode === "history-only" && (
            <div role="status" className="smp-history-banner">
              <ShieldCheck size={20} aria-hidden="true" />
              <div>
                <strong>Recorded history · Read only</strong>
                <p>
                  Live Fabric state was not queried. Deployment execution is
                  disabled.
                </p>
              </div>
              <span className="smp-history-tag">HISTORY MODE</span>
            </div>
          )}
          {error && (
            <div className="smp-error" role="alert">
              <HelpCircle size={18} />
              {error}
              <button
                className="smp-icon-btn"
                aria-label="Dismiss error"
                onClick={() => setError("")}
              >
                <X size={16} />
              </button>
            </div>
          )}
          {view === "create" ? (
            <>
              <div className="smp-page-heading">
                <div>
                  <span className="smp-eyebrow">
                    {step < 4
                      ? `CREATE NETWORK · STEP ${step + 1} OF 6`
                      : "FABRIC SMP"}
                  </span>
                  <h1>
                    <span className="smp-title-prefix" aria-hidden="true">
                      Step {step + 1}:{" "}
                    </span>
                    {secure && step === 0
                      ? "Select your project and secure profile"
                      : titles[step]}
                  </h1>
                  <p>
                    {secure && step === 2
                      ? "Name your qualified two-organization Fabric network. The qualified topology is fixed for this local profile."
                      : subtitles[step]}
                  </p>
                </div>
                {step === 5 && (
                  <button
                    className="smp-btn primary"
                    disabled={healthBusy}
                    onClick={() => void refreshHealth()}
                  >
                    <Activity size={16} />
                    Refresh Health
                  </button>
                )}
                {step === 4 && operation && (
                  <Status
                    tone={
                      operation.status === "succeeded"
                        ? "green"
                        : operation.status === "running"
                          ? "blue"
                          : "amber"
                    }
                  >
                    {operation.status === "running"
                      ? "Deployment in progress"
                      : operation.status}
                  </Status>
                )}
              </div>
              <div
                className={`smp-flow-layout ${step === 5 ? "dashboard-layout" : ""}`}
              >
                <div className="smp-flow-primary">
                  {step === 0 && secure && (
                    <Card title="Authorized deployment project">
                      <p>
                        {project?.tenantId} / {project?.id}
                      </p>
                      <label>
                        Secure deployment profile
                        <select value="smp-secure-local-v1" onChange={() => {}}>
                          <option value="smp-secure-local-v1">
                            Secure local v1 - Fabric 2.5.15
                          </option>
                        </select>
                      </label>
                      <Note>
                        Local development only. Two organizations, two peers,
                        three Raft orderers, LevelDB and one channel. Kubernetes
                        target and namespace are bound by the server.
                      </Note>
                    </Card>
                  )}
                  {step === 0 && !secure && (
                    <CloudAccount
                      draft={draft}
                      setDraft={update}
                      targets={targets}
                      onProvider={provider}
                      onTest={() => void test()}
                      testResult={testResult}
                    />
                  )}
                  {step === 1 && secure && (
                    <Card title="Secure local runtime">
                      <Note>
                        The API provisions this project into its approved
                        disposable kind cluster. AWS and production are
                        disabled. No Kubernetes context or namespace is accepted
                        from this form.
                      </Note>
                      <p>Profile: smp-secure-local-v1</p>
                    </Card>
                  )}
                  {step === 1 && !secure && (
                    <Kubernetes
                      draft={draft}
                      setDraft={update}
                      targets={targets}
                      onTest={() => void test()}
                      testResult={testResult}
                    />
                  )}
                  {step === 2 && secure && (
                    <label>
                      Secure network name
                      <input
                        value={draft.form.name}
                        onChange={(event) =>
                          update({
                            ...draft,
                            form: { ...draft.form, name: event.target.value },
                          })
                        }
                      />
                    </label>
                  )}
                  {step === 2 && (
                    <fieldset disabled={secure} className="qualified-topology">
                      <NetworkDesign
                        draft={draft}
                        setDraft={update}
                        onValidate={() => void validate()}
                        validation={testResult}
                      />
                    </fieldset>
                  )}
                  {step === 3 && (
                    <Review
                      draft={draft}
                      onEdit={go}
                      issues={issues}
                      valid={valid}
                    />
                  )}
                  {step === 4 && (
                    <ProgressScreen
                      operation={operation}
                      recorded={meta?.deploymentMode === "history-only"}
                      retrying={busy}
                      onRetry={() => void retry()}
                      onDashboard={() => go(5)}
                      error={error}
                    />
                  )}
                  {step === 5 && runtime?.verified && (
                    <DashboardScreen
                      onLifecycle={(op) => {
                        setSelectedOperation(op.operationId);
                        setOperation(op);
                        setRuntime(null);
                        setHealth(null);
                        setVerification(null);
                        setDraft((d) => ({
                          ...d,
                          deploymentOperationId: op.operationId,
                        }));
                        setStep(4);
                        setError("");
                      }}
                      draft={kube ? draft : selectProvider(draft, "local")}
                      verification={verification}
                      runtime={runtime}
                      health={health}
                      loading={healthBusy}
                      onRefresh={() => void refreshHealth()}
                      onTest={() => void transaction()}
                      onStop={() =>
                        kube ? setConfirmCleanup(true) : void stop()
                      }
                      onLogs={() =>
                        void (
                          kube && selectedOperation
                            ? api.deploymentLogs(selectedOperation, node)
                            : api.localLogs(node)
                        )
                          .then((r) => setLogs(r.logs))
                          .catch((e) => setError(e.message))
                      }
                      logs={logs}
                      node={node}
                      setNode={setNode}
                      busy={busy || running}
                    />
                  )}
                  {step === 5 && !runtime?.verified && (
                    <Card title="Deployment Verification">
                      <Note tone="amber">
                        {runtime
                          ? meta?.deploymentMode === "history-only"
                            ? "Live verification is unavailable in this read-only history session. View deployment progress for the recorded result."
                            : "This network has not passed deployment verification. Check its operation before opening the dashboard."
                          : "Loading the saved runtime and verification status…"}
                      </Note>
                      <button
                        className="smp-btn secondary"
                        onClick={() => setStep(4)}
                      >
                        View Deployment Progress
                      </button>
                    </Card>
                  )}
                </div>
                {step === 3 && (
                  <details className="smp-summary-details" open>
                    <summary>Detailed network summary</summary>
                    <aside className="smp-summary" aria-label="Network summary">
                      <Summary
                        draft={draft}
                        step={step}
                        valid={valid}
                        runtime={runtime}
                        operation={operation}
                        onEdit={go}
                      />
                    </aside>
                  </details>
                )}
              </div>
              <footer className="smp-flow-footer">
                <div>
                  {step > 0 && (
                    <button
                      className="smp-btn secondary"
                      disabled={busy || running}
                      onClick={() => go(step - 1)}
                    >
                      <ArrowLeft size={16} />
                      Back
                      {step > 0 && step < 5
                        ? `: ${flowSteps[step - 1].title}`
                        : ""}
                    </button>
                  )}
                  {step === 0 && (
                    <button
                      className="smp-btn secondary"
                      onClick={() => navigate("networks")}
                    >
                      Cancel
                    </button>
                  )}
                </div>
                <div>
                  {step === 0 && (
                    <button
                      className="smp-btn primary"
                      disabled={busy}
                      onClick={() => void test()}
                    >
                      <NetworkIcon size={15} />
                      Test Connection
                    </button>
                  )}
                  {step === 3 && !local && !kube && (
                    <button
                      className="smp-btn secondary"
                      disabled={busy || !valid}
                      onClick={() => void prepareDeployment()}
                    >
                      <Download size={15} />
                      Prepare Deployment JSON
                    </button>
                  )}
                  {step >= 2 && step < 4 && !local && (
                    <button
                      className="smp-btn secondary"
                      disabled={busy}
                      onClick={() => void save()}
                    >
                      <Save size={15} />
                      {saved ? "Save Changes" : "Save Draft"}
                    </button>
                  )}
                  {step < 3 ? (
                    <button
                      className={`smp-btn ${step === 0 ? "secondary" : "primary"}`}
                      disabled={busy}
                      onClick={() => void next()}
                    >
                      {busy && <LoaderCircle size={16} className="spin" />}Next:{" "}
                      {flowSteps[step + 1].title}
                      <ArrowRight size={16} />
                    </button>
                  ) : step === 3 ? (
                    <button
                      className="smp-btn primary"
                      disabled={
                        busy ||
                        (secure && !!draft.deploymentOperationId) ||
                        !valid ||
                        (!local &&
                          !(
                            kube &&
                            (secure
                              ? meta?.deploymentMode === "secure-local"
                              : meta?.deploymentMode === "local-kubernetes")
                          )) ||
                        running
                      }
                      onClick={() => void create()}
                    >
                      Create Network
                      <ArrowRight size={16} />
                    </button>
                  ) : step === 4 ? (
                    <button
                      className="smp-btn primary"
                      disabled={
                        operation?.status !== "succeeded" ||
                        operation.kind === "stop" ||
                        !runtime?.verified ||
                        runtime.phase !== "running"
                      }
                      onClick={() => go(5)}
                    >
                      View Network Dashboard
                      <ArrowRight size={16} />
                    </button>
                  ) : (
                    <button
                      className="smp-btn secondary"
                      onClick={() => navigate("networks")}
                    >
                      All Networks
                    </button>
                  )}
                </div>
              </footer>
            </>
          ) : (
            <>
              <div className="smp-page-heading">
                <div>
                  <span className="smp-eyebrow">FABRIC SMP WORKSPACE</span>
                  <h1>
                    {view === "custom"
                      ? "Customer configurations"
                      : view === "settings"
                        ? "Cloud readiness"
                        : view.charAt(0).toUpperCase() + view.slice(1)}
                  </h1>
                  <p>
                    {view === "networks"
                      ? "Your deployed networks and saved network designs."
                      : view === "operations"
                        ? "Persisted operations and real deployment stage history."
                        : view === "custom"
                          ? "Design your topology. Track every change from configuration to committed state."
                          : view === "nodes"
                            ? "Peers, orderers and certificate authorities in the selected deployment."
                            : view === "chaincode"
                              ? "Committed contracts, definitions and transaction verification."
                              : "Review deployment requirements and the security gates still to resolve."}
                  </p>
                </div>
                <button className="smp-btn primary" onClick={newFlow}>
                  <Plus size={16} />
                  Create Network
                </button>
              </div>
              {view === "custom" && (
                <CustomNetworks
                  key={workspaceKey()}
                  initialId={customNetworkId}
                  capabilities={meta?.dynamicLocal}
                />
              )}
              {(view === "networks" || view === "operations") && (
                <div
                  className="smp-workspace-summary"
                  aria-label="Recorded workspace summary"
                >
                  <div>
                    <span>
                      <NetworkIcon size={17} /> Saved networks
                    </span>
                    <strong>{networks.length}</strong>
                    <small>In this project</small>
                  </div>
                  <div>
                    <span>
                      <CheckCircle2 size={17} /> Completed operations
                    </span>
                    <strong>
                      {history.filter((op) => op.status === "succeeded").length}
                    </strong>
                    <small>Recorded successes</small>
                  </div>
                  <div>
                    <span>
                      <Activity size={17} /> Needs attention
                    </span>
                    <strong>
                      {
                        history.filter((op) =>
                          ["failed", "interrupted"].includes(op.status),
                        ).length
                      }
                    </strong>
                    <small>Failed or interrupted operations</small>
                  </div>
                </div>
              )}
              {view === "networks" && (
                <>
                  <div className="smp-search">
                    <Search size={17} />
                    <input
                      aria-label="Search networks"
                      value={search}
                      onChange={(e) => setSearch(e.target.value)}
                      placeholder="Search networks…"
                    />
                  </div>
                  <div className="smp-network-grid">
                    {runtime && local && (
                      <button
                        className="smp-network-card smp-card"
                        onClick={openLocal}
                      >
                        <div>
                          <span className="smp-tile">
                            <NetworkIcon size={24} />
                          </span>
                          <Status tone={runtime.ready ? "green" : "amber"}>
                            {runtime.ready ? "Running" : runtime.phase}
                          </Status>
                        </div>
                        <h2>Sathonate Development Network</h2>
                        <p>Local Docker · Fabric {runtime.version} · Raft</p>
                        <div className="smp-network-counts">
                          <span>2 organizations</span>
                          <span>3 orderers</span>
                          <span>1 channel</span>
                        </div>
                        <footer>
                          Open network dashboard
                          <ArrowRight size={16} />
                        </footer>
                      </button>
                    )}
                    {networks
                      .filter((n) =>
                        n.desired.metadata.name
                          .toLowerCase()
                          .includes(search.toLowerCase()),
                      )
                      .map((n) => (
                        <button
                          key={n.id}
                          className="smp-network-card smp-card"
                          onClick={() => {
                            if (
                              n.desired.spec.deployment.targetRef ===
                              "smp-dynamic-local-v1"
                            ) {
                              setCustomNetworkId(n.id);
                              navigate("custom");
                              return;
                            }
                            resume(n);
                          }}
                        >
                          <div>
                            <span className="smp-tile">
                              <Cloud size={24} />
                            </span>
                            <Status
                              tone={
                                history.some(
                                  (op) =>
                                    op.networkId === n.id &&
                                    [
                                      "local-kubernetes-deploy",
                                      "secure-local-deploy",
                                    ].includes(op.kind),
                                )
                                  ? "blue"
                                  : "amber"
                              }
                            >
                              {history.find(
                                (op) =>
                                  op.networkId === n.id &&
                                  [
                                    "local-kubernetes-deploy",
                                    "secure-local-deploy",
                                  ].includes(op.kind),
                              )?.status ?? "Design draft"}
                            </Status>
                          </div>
                          <h2>{n.desired.metadata.name}</h2>
                          <p>
                            {n.desired.spec.deployment.provider?.toUpperCase() ??
                              "Private servers"}{" "}
                            · Fabric {n.desired.spec.fabricVersion}
                          </p>
                          <div className="smp-network-counts">
                            <span>
                              {n.desired.spec.organizations.length}{" "}
                              organizations
                            </span>
                            <span>Revision {n.revision}</span>
                          </div>
                          <footer>
                            Resume network design
                            <ArrowRight size={16} />
                          </footer>
                        </button>
                      ))}
                  </div>
                </>
              )}
              {view === "operations" && (
                <Card title="Operation History">
                  <div className="smp-table-wrap">
                    <table className="smp-table">
                      <thead>
                        <tr>
                          <th>Operation</th>
                          <th>Action</th>
                          <th>Started</th>
                          <th>Status</th>
                          <th />
                        </tr>
                      </thead>
                      <tbody>
                        {history.map((op) => (
                          <tr key={op.operationId}>
                            <td>
                              <code>{op.operationId}</code>
                            </td>
                            <td>{op.kind}</td>
                            <td>{formatDate(op.createdAt)}</td>
                            <td>
                              <Status
                                tone={
                                  op.status === "succeeded"
                                    ? "green"
                                    : op.status === "running"
                                      ? "blue"
                                      : "amber"
                                }
                              >
                                {op.status}
                              </Status>
                            </td>
                            <td>
                              <button
                                className="smp-text-btn"
                                onClick={() => {
                                  setOperation(op);
                                  setSelectedOperation(op.operationId);
                                  setRuntime(null);
                                  setHealth(null);
                                  const n = networks.find(
                                    (n) => n.id === op.networkId,
                                  );
                                  if (
                                    n?.desired.spec.deployment.targetRef ===
                                    "smp-dynamic-local-v1"
                                  ) {
                                    setCustomNetworkId(n.id);
                                    navigate("custom");
                                    return;
                                  }
                                  setSaved(n ?? null);
                                  setDraft({
                                    ...selectProvider(
                                      draft,
                                      [
                                        "local-kubernetes-deploy",
                                        "secure-local-deploy",
                                      ].includes(op.kind)
                                        ? "kubernetes"
                                        : "local",
                                    ),
                                    ...(n
                                      ? {
                                          form: fromNetwork(n.desired),
                                          recordId: n.id,
                                        }
                                      : {}),
                                    deploymentOperationId: [
                                      "local-kubernetes-deploy",
                                      "secure-local-deploy",
                                    ].includes(op.kind)
                                      ? op.operationId
                                      : undefined,
                                  });
                                  setView("create");
                                  setStep(4);
                                }}
                              >
                                View
                              </button>
                              <OperationDiagnosticsButton
                                operationId={op.operationId}
                              />
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                    {!history.length && (
                      <p className="smp-empty">No operations recorded yet.</p>
                    )}
                  </div>
                </Card>
              )}
              {view === "nodes" && (
                <Card
                  title={
                    kube
                      ? "Kubernetes Fabric Node Inventory"
                      : "Live Node Inventory"
                  }
                >
                  {kube && (
                    <Note>
                      These nodes belong to the selected local deployment.
                      Availability reflects the last verification or health
                      refresh.
                    </Note>
                  )}
                  <div className="smp-table-wrap">
                    <table className="smp-table">
                      <thead>
                        <tr>
                          <th>{kube ? "Fabric node" : "Container"}</th>
                          <th>Status</th>
                          <th>Published endpoints</th>
                        </tr>
                      </thead>
                      <tbody>
                        {kube &&
                          health &&
                          [
                            ...health.peers.map((p) => ({
                              name: p.peer,
                              state: p.available ? "Available" : "Unavailable",
                              kind: "Peer",
                            })),
                            ...health.orderers.map((o) => ({
                              name: o.name,
                              state: o.status,
                              kind: "Raft orderer",
                            })),
                            ...health.certificateAuthorities.map((c) => ({
                              name: c.name,
                              state: c.available ? "Available" : "Unavailable",
                              kind: "Fabric CA",
                            })),
                          ].map((n) => (
                            <tr key={n.name}>
                              <td>
                                {n.name}
                                <small>{n.kind}</small>
                              </td>
                              <td>
                                <Status
                                  tone={
                                    n.state === "Unavailable" ||
                                    n.state === "unavailable"
                                      ? "amber"
                                      : "green"
                                  }
                                >
                                  {n.state}
                                </Status>
                              </td>
                              <td>Internal cluster DNS · TLS</td>
                            </tr>
                          ))}
                        {!kube &&
                          runtime?.containers.map((c) => (
                            <tr key={c.name}>
                              <td>{c.name}</td>
                              <td>
                                <Status
                                  tone={
                                    c.state === "running" ? "green" : "amber"
                                  }
                                >
                                  {c.state}
                                </Status>
                              </td>
                              <td>
                                <small>
                                  {c.ports || "Internal Docker network"}
                                </small>
                              </td>
                            </tr>
                          ))}
                      </tbody>
                    </table>
                  </div>
                </Card>
              )}
              {view === "chaincode" && (
                <Card title="Committed Chaincode" icon={Code2}>
                  {health?.chaincode.available ? (
                    <>
                      <div className="smp-contract">
                        <Code2 size={28} />
                        <div>
                          <strong>{health.chaincode.name}</strong>
                          <p>
                            Go · Version {health.chaincode.version} · Sequence{" "}
                            {health.chaincode.sequence}
                          </p>
                        </div>
                        <Status tone="green">Committed</Status>
                      </div>
                      <button
                        className="smp-btn secondary"
                        disabled={busy || !runtime?.ready}
                        onClick={() => void transaction()}
                      >
                        Run Transaction Check
                      </button>
                    </>
                  ) : (
                    <Note>
                      No committed contract has been verified. Open a running
                      network to check its inventory.
                    </Note>
                  )}
                </Card>
              )}
              {view === "settings" && <CloudReadiness />}
              {view === "settings" && (
                <Card title="Workspace Configuration" icon={Settings}>
                  <dl className="smp-facts">
                    {[
                      ["Product", "Fabric SMP"],
                      ["Company", "Sathonate"],
                      ["Authentication", meta?.mode ?? "Unavailable"],
                      ["Storage", meta?.storage ?? "Unavailable"],
                      ["Cloud execution", "Not enabled"],
                      ["Registered cloud targets", String(targets.length)],
                    ].map(([k, v]) => (
                      <div key={k}>
                        <dt>{k}</dt>
                        <dd>{v}</dd>
                      </div>
                    ))}
                  </dl>
                  <Note>
                    Cloud IAM, Kubernetes access, GitOps access and Fabric CA
                    identities are separate trust boundaries. Credentials are
                    not collected in ordinary forms.
                  </Note>
                </Card>
              )}
            </>
          )}
          <div className="smp-bottom-note">
            Fabric SMP{" "}
            <span>
              Built by Sathonate · Desired configuration and verified state stay
              separate.
            </span>
          </div>
        </main>
      </div>
      {toast && (
        <div className="smp-toast" role="status">
          <CheckCircle2 size={18} />
          {toast}
          <button
            className="smp-icon-btn"
            aria-label="Dismiss notification"
            onClick={() => setToast("")}
          >
            <X size={16} />
          </button>
        </div>
      )}
      {help && (
        <div className="modal-backdrop">
          <div
            ref={helpRef}
            role="dialog"
            aria-modal="true"
            aria-labelledby="smp-help-title"
            className="smp-help smp-card"
          >
            <div className="smp-card-heading">
              <h2 id="smp-help-title">Create a network in six steps</h2>
              <button
                className="smp-icon-btn"
                aria-label="Close help"
                onClick={() => setHelp(false)}
              >
                <X size={20} />
              </button>
            </div>
            <ol>
              {flowSteps.map((s) => (
                <li key={s.title}>
                  <strong>{s.title}</strong>
                  <p>{s.detail}</p>
                </li>
              ))}
            </ol>
            <Note>
              Choose Local Kubernetes for the real Bevel / kind workflow, or
              Local / Existing for the Docker profile. AWS and Azure remain
              design-only; paid cloud execution is disabled.
            </Note>
          </div>
        </div>
      )}
      {confirmCleanup && (
        <div className="modal-backdrop">
          <div
            role="dialog"
            aria-modal="true"
            aria-labelledby="cleanup-title"
            ref={cleanupRef}
            className="smp-help smp-card"
          >
            <h2 id="cleanup-title">Delete this local Kubernetes network?</h2>
            <p>
              This removes this network's CAs, peers, orderers, channel
              resources, secrets and persistent volumes from the dedicated kind
              cluster. PostgreSQL design and operation history are retained.
            </p>
            <div className="smp-inline-actions">
              <button
                className="smp-btn secondary"
                onClick={() => setConfirmCleanup(false)}
              >
                Cancel
              </button>
              <button
                className="smp-btn primary"
                onClick={() => {
                  setConfirmCleanup(false);
                  void stop();
                }}
              >
                Delete Network and Volumes
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

function Summary({
  draft: d,
  step,
  valid,
  runtime,
  operation,
  onEdit,
}: {
  draft: FlowDraft;
  step: number;
  valid: boolean;
  runtime: LocalState | null;
  operation: Operation | null;
  onEdit: (n: number) => void;
}) {
  const kube = d.provider === "kubernetes",
    local = d.provider === "local" || kube,
    f = d.form;
  return (
    <>
      <Note>
        {kube
          ? "Fabric CA identities and Bevel-managed workloads run in the dedicated local kind cluster."
          : local
            ? "Your local network uses Docker and Fabric CA. No cloud account, kubeconfig or permanent key is required."
            : "Use short-lived cloud identity and securely registered cluster access."}
      </Note>
      <Card title="Cloud Account">
        <div className="smp-account-mini">
          <ProviderMark provider={d.provider} />
          <div>
            <strong>
              {local
                ? kube
                  ? "Local Kubernetes"
                  : "Local Docker"
                : d.provider === "aws"
                  ? "Amazon Web Services"
                  : "Microsoft Azure"}
            </strong>
            <p>{local ? "This computer" : d.region || "Region not selected"}</p>
          </div>
          <Status
            tone={
              (kube ? runtime?.verified : local && runtime?.dockerAvailable)
                ? "green"
                : "amber"
            }
          >
            {kube
              ? runtime?.verified
                ? "Verified"
                : "Unchecked"
              : local
                ? runtime?.dockerAvailable
                  ? "Available"
                  : "Unchecked"
                : "Not verified"}
          </Status>
        </div>
      </Card>
      <Card
        title={step > 1 ? "Network Summary" : "Cluster Configuration Summary"}
        action={
          <button
            className="smp-text-btn"
            onClick={() => onEdit(step > 1 ? 2 : 1)}
          >
            Edit
          </button>
        }
      >
        <dl className="smp-facts">
          {[
            [
              "Deployment Target",
              local
                ? kube
                  ? "Local Kubernetes"
                  : "Local Docker"
                : d.clusterMode === "new"
                  ? "New managed cluster"
                  : "Existing cluster",
            ],
            [
              "Cluster Name",
              kube
                ? "kind-smp-qualification"
                : local
                  ? "Not required"
                  : d.clusterName || "Not selected",
            ],
            ["Fabric Version", f.version],
            ["Organizations", String(f.organizations.length)],
            [
              "Peers (total)",
              String(f.organizations.reduce((n, o) => n + o.peers, 0)),
            ],
            ["Orderers", `${f.orderers} (Raft)`],
            ["Channels", "1"],
          ].map(([k, v]) => (
            <div key={k}>
              <dt>{k}</dt>
              <dd>{v}</dd>
            </div>
          ))}
        </dl>
      </Card>
      {step === 1 && !local && (
        <Card title="Selected Add-ons">
          {d.addons.length ? (
            d.addons.map((a) => (
              <div className="smp-addon-summary" key={a}>
                <CheckCircle2 size={15} />
                {a}
                <small>Planned</small>
              </div>
            ))
          ) : (
            <p className="smp-subcopy">No add-ons selected.</p>
          )}
        </Card>
      )}
      {step >= 3 ? (
        <>
          <Card
            title={
              step === 5 ? "Deployment Resources" : "Cost Estimate (Monthly)"
            }
          >
            <div className="smp-cost">
              <span>{local ? "Local resources" : "Estimate unavailable"}</span>
              <p>
                {local
                  ? "No cloud infrastructure is provisioned."
                  : "Provider pricing is not connected."}
              </p>
            </div>
            <dl className="smp-cost-lines">
              <div>
                <dt>
                  {kube
                    ? "Fabric workloads"
                    : local
                      ? "Docker containers"
                      : "Cluster & compute"}
                </dt>
                <dd>{kube ? "8" : local ? "10" : "Not quoted"}</dd>
              </div>
              <div>
                <dt>
                  {local ? "Persistent ledger volumes" : "Storage & networking"}
                </dt>
                <dd>{kube ? "7" : local ? "5" : "Not quoted"}</dd>
              </div>
            </dl>
            <Note>
              {local
                ? "Hardware, electricity and Docker licensing costs are excluded."
                : "A verified quote needs provider, region, resources and usage assumptions. No sample prices are shown."}
            </Note>
          </Card>
          <Card title="Validation Checks">
            <ul className="smp-checklist">
              <li>
                <CheckCircle2 size={17} />
                <span>
                  {valid || (local && runtime?.verified)
                    ? "Backend topology validation passed"
                    : "Configuration validation required"}
                </span>
              </li>
              <li>
                <ShieldCheck size={17} />
                <span>Fabric CA identities; no inline credentials</span>
              </li>
              <li className={!local ? "pending" : ""}>
                <Server size={17} />
                <span>
                  {local
                    ? "Local profile and versions fixed"
                    : "Cloud permissions not verified"}
                </span>
              </li>
              <li className={!local ? "pending" : ""}>
                <Cloud size={17} />
                <span>
                  {local
                    ? runtime?.verified
                      ? "Deployment verification passed"
                      : "Real deployment verification required"
                    : "Cloud execution is not enabled"}
                </span>
              </li>
            </ul>
          </Card>
          {step === 4 && operation && (
            <Card title="Operation Details">
              <dl className="smp-facts">
                <div>
                  <dt>Action</dt>
                  <dd>{operation.kind}</dd>
                </div>
                <div>
                  <dt>Status</dt>
                  <dd>{operation.status}</dd>
                </div>
                <div>
                  <dt>Started</dt>
                  <dd>{formatDate(operation.createdAt)}</dd>
                </div>
              </dl>
            </Card>
          )}
        </>
      ) : (
        <Card title="What happens next?" className="smp-next-card">
          <ol className="smp-next-list">
            <li>
              <span>1</span>Configure your runtime and network topology.
            </li>
            <li>
              <span>2</span>Review validation, resources and cost assumptions.
            </li>
            <li>
              <span>3</span>Create the supported network and follow live
              progress.
            </li>
          </ol>
        </Card>
      )}
      {!local && (
        <Note tone="amber">
          Cloud design only. Save your design; choose Local / Existing to run
          containers now.
        </Note>
      )}
    </>
  );
}
