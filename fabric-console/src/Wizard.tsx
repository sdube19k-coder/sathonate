import { useEffect, useState } from "react";
import {
  ArrowLeft,
  ArrowRight,
  Check,
  CheckCircle2,
  ChevronDown,
  Cloud,
  Info,
  LoaderCircle,
  Plus,
  Save,
  Server,
  ShieldCheck,
  Trash2,
  X,
} from "lucide-react";
import { api, APIError, type CloudTarget } from "./api";
import { Badge, Field, ProgressSteps, Topology } from "./components";
import {
  fromNetwork,
  newForm,
  slug,
  stepError,
  toSpec,
  withOrdererCount,
  type NetworkForm,
} from "./network-form";
import type { Issue, Network } from "./types";
import { useDialog } from "./use-dialog";
import OrderingStep from "./OrderingStep";

export default function Wizard({
  existing,
  template,
  onClose,
  onSaved,
}: {
  existing?: Network;
  template?: boolean;
  onClose: () => void;
  onSaved: (network: Network) => void;
}) {
  const [form, setForm] = useState<NetworkForm>(() =>
    existing ? fromNetwork(existing.desired) : newForm(template),
  );
  const [step, setStep] = useState(0);
  const [cloudTargets, setCloudTargets] = useState<CloudTarget[]>([]);
  useEffect(() => {
    void api
      .cloudTargets()
      .then((result) => setCloudTargets(result.items))
      .catch(() => setCloudTargets([]));
  }, []);
  const [issues, setIssues] = useState<Issue[]>([]);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [validated, setValidated] = useState(false);
  const [confirmLeave, setConfirmLeave] = useState(false);
  const dialogRef = useDialog(confirmLeave, () => setConfirmLeave(false));
  const spec = toSpec(form);
  const update = <K extends keyof NetworkForm>(
    key: K,
    value: NetworkForm[K],
  ) => {
    setForm((previous) => ({ ...previous, [key]: value }));
    setValidated(false);
    setError("");
  };
  const next = async () => {
    const message = stepError(form, step);
    if (message) {
      setError(message);
      return;
    }
    if (step < 3) {
      setStep(step + 1);
      return;
    }
    setBusy(true);
    setError("");
    try {
      const result = await api.validate(spec);
      setIssues(result.issues);
      if (!result.valid) {
        setError("Some settings need attention. Review the details below.");
        return;
      }
      setValidated(true);
      setStep(4);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Validation failed.");
    } finally {
      setBusy(false);
    }
  };
  const save = async () => {
    setBusy(true);
    setError("");
    try {
      const saved = existing
        ? await api.update(existing, spec)
        : await api.create(spec);
      onSaved(saved);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Could not save the draft.");
      if (e instanceof APIError) setIssues(e.issues);
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className="wizard-page">
      <div className="wizard-heading">
        <div>
          <button
            className="text-button back"
            onClick={() => setConfirmLeave(true)}
          >
            <ArrowLeft size={15} />
            Networks
          </button>
          <h1>
            {existing ? "Edit your network" : "Let’s build your network."}
          </h1>
          <p>A few simple choices. We’ll put the technical pieces together.</p>
        </div>
        <button
          className="icon-button"
          aria-label="Close network builder"
          onClick={() => setConfirmLeave(true)}
        >
          <X size={20} />
        </button>
      </div>
      <ProgressSteps
        current={step}
        labels={[
          "The basics",
          "Deployment",
          "Organizations",
          "Orderers",
          "Review",
        ]}
      />
      <div className="wizard-layout">
        <div className="wizard-main panel">
          <div className="section-kicker">STEP {step + 1} OF 5</div>
          {step === 0 && (
            <>
              <h2>Start with a name.</h2>
              <p className="section-description">
                Give your network a clear purpose. You can fine-tune the details
                later.
              </p>
              <Field
                label="Network name"
                hint="A name your team will recognize."
              >
                <input
                  autoFocus
                  maxLength={60}
                  placeholder="e.g. Supply chain network"
                  value={form.name}
                  onChange={(e) => update("name", e.target.value)}
                />
              </Field>
              <div className="derived-id">
                Network ID{" "}
                <code>
                  {form.networkId ||
                    slug(form.name) ||
                    "generated-from-your-name"}
                </code>
              </div>
              <div className="field-label spaced">
                What are you designing for?
              </div>
              <div className="choice-grid">
                <button
                  className={`choice ${form.environment === "development" ? "selected" : ""}`}
                  onClick={() => update("environment", "development")}
                >
                  <span className="radio-mark" />
                  <strong>Development</strong>
                  <p>Experiment, prototype, and learn.</p>
                  <span className="small-tag">A good place to start</span>
                </button>
                <button
                  className={`choice ${form.environment === "production" ? "selected" : ""}`}
                  onClick={() => {
                    setForm(
                      withOrdererCount(
                        { ...form, environment: "production" },
                        Math.max(
                          form.consensus === "bft" ? 4 : 3,
                          form.orderers,
                        ),
                      ),
                    );
                    setValidated(false);
                    setError("");
                  }}
                >
                  <span className="radio-mark" />
                  <strong>Production design</strong>
                  <p>Plan for resilience and your team.</p>
                  <span className="small-tag">Additional checks</span>
                </button>
              </div>
              <div className="info-note">
                <Info size={17} />
                <p>
                  This workspace saves network designs. Infrastructure
                  deployment will be connected in a later release.
                </p>
              </div>
            </>
          )}
          {step === 1 && (
            <>
              <h2>Where will it live?</h2>
              <p className="section-description">
                The same experience, whether you use the cloud or your own
                servers.
              </p>
              <div className="destination-options">
                {(
                  [
                    {
                      id: "aws",
                      title: "Amazon Web Services",
                      text: "Plan a Fabric network on AWS.",
                      icon: Cloud,
                      tag: "Cloud",
                    },
                    {
                      id: "azure",
                      title: "Microsoft Azure",
                      text: "Plan a Fabric network on Azure.",
                      icon: Cloud,
                      tag: "Cloud",
                    },
                    {
                      id: "private",
                      title: "Your own servers",
                      text: "Keep your network inside your infrastructure.",
                      icon: Server,
                      tag: "Private",
                    },
                  ] as const
                ).map((choice) => (
                  <button
                    key={choice.id}
                    className={`destination-choice ${form.destination === choice.id ? "selected" : ""}`}
                    onClick={() => {
                      update("destination", choice.id);
                      update(
                        "target",
                        choice.id === "private"
                          ? "private-lab"
                          : `${choice.id}-workspace`,
                      );
                    }}
                  >
                    <span className={`destination-logo ${choice.id}`}>
                      <choice.icon size={24} />
                    </span>
                    <span>
                      <strong>{choice.title}</strong>
                      <small>{choice.text}</small>
                    </span>
                    <span className="destination-tag">{choice.tag}</span>
                    <span className="radio-mark" />
                  </button>
                ))}
              </div>
              {form.destination === "aws" && (
                <div className="cloud-target-picker">
                  <Field
                    label="Registered EKS target"
                    hint="Choose a target registered by your administrator. Unregistered references cannot deploy."
                  >
                    <select
                      value={
                        cloudTargets.some((target) => target.id === form.target)
                          ? form.target
                          : ""
                      }
                      onChange={(e) => update("target", e.target.value)}
                    >
                      <option value="" disabled>
                        {cloudTargets.length
                          ? "Choose an EKS cluster"
                          : "No EKS targets registered"}
                      </option>
                      {cloudTargets.map((target) => (
                        <option key={target.id} value={target.id}>
                          {target.name || target.id} · {target.region}
                        </option>
                      ))}
                    </select>
                  </Field>
                </div>
              )}
              <details className="advanced">
                <summary>
                  Target details <ChevronDown size={15} />
                </summary>
                <Field
                  label="Target reference"
                  hint="A placeholder for now. Actual account or server connection comes later."
                >
                  <input
                    value={form.target}
                    onChange={(e) => update("target", e.target.value)}
                  />
                </Field>
                <Field
                  label="Fabric version"
                  hint="Version 2.5.16 is a planning candidate; deployment compatibility is not certified."
                >
                  <input
                    value={form.version}
                    onChange={(e) => update("version", e.target.value)}
                  />
                </Field>
              </details>
              <div className="info-note">
                <ShieldCheck size={17} />
                <p>
                  No cloud credentials, passwords, or private keys are needed to
                  design your network.
                </p>
              </div>
            </>
          )}
          {step === 2 && (
            <>
              <h2>Who’s part of the network?</h2>
              <p className="section-description">
                An organization is a participating team or business. Each gets
                its own identity and peers.
              </p>
              <div className="organization-editor">
                {form.organizations.map((org, index) => (
                  <div className="organization-row" key={org.key}>
                    <div className="org-number">
                      {String(index + 1).padStart(2, "0")}
                    </div>
                    <Field label={`Organization ${index + 1}`}>
                      <input
                        maxLength={35}
                        value={org.name}
                        onChange={(e) =>
                          update(
                            "organizations",
                            form.organizations.map((o) =>
                              o.key === org.key
                                ? { ...o, name: e.target.value }
                                : o,
                            ),
                          )
                        }
                      />
                    </Field>
                    <Field label="Peers">
                      <select
                        value={org.peers}
                        onChange={(e) =>
                          update(
                            "organizations",
                            form.organizations.map((o) =>
                              o.key === org.key
                                ? { ...o, peers: Number(e.target.value) }
                                : o,
                            ),
                          )
                        }
                      >
                        {[0, 1, 2, 3].map((value) => (
                          <option key={value} value={value}>
                            {value === 0
                              ? "0 · Orderer-only"
                              : `${value} ${value === 1 ? "peer" : "peers"}`}
                          </option>
                        ))}
                      </select>
                    </Field>
                    <button
                      className="icon-button"
                      disabled={form.organizations.length <= 1}
                      aria-label={`Remove organization ${index + 1}`}
                      onClick={() =>
                        update(
                          "organizations",
                          form.organizations.filter((o) => o.key !== org.key),
                        )
                      }
                    >
                      <Trash2 size={16} />
                    </button>
                  </div>
                ))}
              </div>
              <button
                className="text-button teal"
                disabled={form.organizations.length >= 6}
                onClick={() =>
                  update("organizations", [
                    ...form.organizations,
                    {
                      key: crypto.randomUUID(),
                      name: `Organization ${form.organizations.length + 1}`,
                      peers: 1,
                    },
                  ])
                }
              >
                <Plus size={16} />
                Add organization
              </button>
              <div className="form-divider" />
              <div className="two-fields">
                <Field
                  label="Shared channel"
                  hint="A private ledger shared by these organizations."
                >
                  <input
                    value={form.channel}
                    onChange={(e) => update("channel", e.target.value)}
                  />
                </Field>
              </div>
              <div className="info-note">
                <Info size={17} />
                <p>
                  We generate the identity and placement references. Real hosts,
                  failure domains, and certificates must be verified before
                  deployment.
                </p>
              </div>
            </>
          )}
          {step === 3 && (
            <OrderingStep
              form={form}
              onChange={(value) => {
                setForm(value);
                setValidated(false);
                setError("");
              }}
            />
          )}
          {step === 4 && (
            <>
              <div className="review-title">
                <span className="success-circle">
                  <CheckCircle2 size={22} />
                </span>
                <div>
                  <h2>Your blueprint is ready.</h2>
                  <p>Review the design, then save it to your workspace.</p>
                </div>
              </div>
              <div className="review-facts">
                <div>
                  <span>Network</span>
                  <strong>{form.name}</strong>
                </div>
                <div>
                  <span>Destination</span>
                  <strong>
                    {form.destination === "private"
                      ? "Private servers"
                      : form.destination === "aws"
                        ? "Amazon Web Services"
                        : "Microsoft Azure"}
                  </strong>
                </div>
                <div>
                  <span>Environment</span>
                  <strong>{form.environment}</strong>
                </div>
                <div>
                  <span>Fabric version</span>
                  <strong>{form.version}</strong>
                </div>
                <div>
                  <span>Consensus</span>
                  <strong>
                    {form.consensus === "bft" ? "SmartBFT" : "Raft"}
                  </strong>
                </div>
                <div>
                  <span>Orderers</span>
                  <strong>{form.orderers} configured nodes</strong>
                </div>
              </div>
              <Topology spec={spec} />
              <div className="checks-title">
                <ShieldCheck size={17} />
                <strong>Design checks</strong>
                <Badge tone="green">Validated</Badge>
              </div>
              <p className="tiny muted">
                Configuration checks passed. Infrastructure and live Fabric
                state have not been verified.
              </p>
              <div className="warning-list">
                {issues.map((issue, i) => (
                  <div key={i}>
                    <Info size={15} />
                    <span>{issue.message}</span>
                  </div>
                ))}
              </div>
            </>
          )}
          {error && (
            <div className="error-box" role="alert">
              {error}
              {issues
                .filter((i) => i.severity === "error")
                .map((issue, i) => (
                  <p key={i}>
                    {issue.path}: {issue.message}
                  </p>
                ))}
            </div>
          )}
          <div className="wizard-actions">
            <button
              className="button secondary"
              disabled={step === 0 || busy}
              onClick={() => {
                setStep(step - 1);
                setError("");
              }}
            >
              <ArrowLeft size={16} />
              Back
            </button>
            <span>
              {step === 4
                ? "Saved as a draft. No resources deployed."
                : "Your changes stay here until you save."}
            </span>
            {step < 4 ? (
              <button
                className="button primary"
                disabled={busy}
                onClick={() => void next()}
              >
                {busy ? <LoaderCircle className="spin" size={17} /> : null}
                {step === 3 ? "Review network" : "Continue"}
                <ArrowRight size={16} />
              </button>
            ) : (
              <button
                className="button primary"
                disabled={busy || !validated}
                onClick={() => void save()}
              >
                {busy ? (
                  <LoaderCircle className="spin" size={17} />
                ) : (
                  <Save size={16} />
                )}
                Save network draft
              </button>
            )}
          </div>
        </div>
        <aside className="wizard-aside">
          <div className="aside-icon">
            <ShieldCheck size={25} />
          </div>
          <h3>
            Complex underneath.
            <br />
            Simple up here.
          </h3>
          <p>
            You describe the network. Sathonate organizes the components into a
            clear, consistent blueprint.
          </p>
          <ul>
            <li>
              <Check size={16} />
              Separate identities for every organization
            </li>
            <li>
              <Check size={16} />A private ledger for your participants
            </li>
            <li>
              <Check size={16} />
              Checks before infrastructure changes
            </li>
          </ul>
          <div className="aside-summary">
            <span className="eyebrow">YOUR NETWORK AT A GLANCE</span>
            <div>
              <span>Organizations</span>
              <b>{form.organizations.length}</b>
            </div>
            <div>
              <span>Peers</span>
              <b>
                {form.organizations.reduce((sum, org) => sum + org.peers, 0)}
              </b>
            </div>
            <div>
              <span>Orderers</span>
              <b>{form.orderers}</b>
            </div>
            <div>
              <span>Channels</span>
              <b>1</b>
            </div>
          </div>
          <span className="tiny muted">
            Planning workspace · Deployment coming later
          </span>
        </aside>
      </div>
      {confirmLeave && (
        <div className="modal-backdrop">
          <div
            ref={dialogRef}
            className="modal"
            role="dialog"
            aria-modal="true"
            aria-labelledby="leave-title"
          >
            <h2 id="leave-title">Leave this design?</h2>
            <p>Your unsaved changes will be discarded.</p>
            <div className="modal-actions">
              <button
                className="button secondary"
                autoFocus
                onClick={() => setConfirmLeave(false)}
              >
                Keep editing
              </button>
              <button className="button danger" onClick={onClose}>
                Discard changes
              </button>
            </div>
          </div>
        </div>
      )}
    </section>
  );
}
