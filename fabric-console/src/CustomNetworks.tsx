import { useEffect, useState } from "react";
import {
  api,
  type DeploymentState,
  type DynamicPreview,
  type Operation,
} from "./api";
import type { DynamicCapabilities, Network } from "./types";
import {
  parseCustomerConfiguration,
  customerSummary,
} from "./custom-configuration";
import OperationDiagnosticsButton from "./OperationDiagnostics";

export default function CustomNetworks({
  initialId,
  capabilities,
}: {
  initialId?: string;
  capabilities?: DynamicCapabilities;
}) {
  const [items, setItems] = useState<Network[]>([]),
    [network, setNetwork] = useState<Network | null>(null),
    [text, setText] = useState(""),
    [operation, setOperation] = useState<Operation | null>(null),
    [observed, setObserved] = useState<DeploymentState | null>(null),
    [preview, setPreview] = useState<DynamicPreview | null>(null),
    [approved, setApproved] = useState(false),
    [busy, setBusy] = useState(false),
    [error, setError] = useState(""),
    [notice, setNotice] = useState(""),
    [editing, setEditing] = useState(true);
  async function load(id?: string) {
    const list = await api.networks();
    const custom = list.items.filter(
      (n) => n.desired.spec.deployment.targetRef === "smp-dynamic-local-v1",
    );
    setItems(custom);
    if (id) {
      const selected = custom.find((n) => n.id === id);
      if (selected) {
        setNetwork(selected);
        setText(JSON.stringify(selected.desired, null, 2));
        const history = await api.operations();
        setOperation(history.items.find((o) => o.networkId === id) ?? null);
      }
    }
  }
  useEffect(() => {
    void load(initialId).catch(() =>
      setError("Cannot load customer-defined networks."),
    );
  }, [initialId]);
  useEffect(() => {
    if (!operation) return;
    let disposed = false;
    async function read() {
      try {
        const current = await api.operation(operation!.operationId);
        if (disposed) return;
        setOperation(current);
        if (current.status === "succeeded") {
          const state = await api.deployment(current.operationId);
          if (!disposed) setObserved(state);
        }
      } catch {
        if (!disposed)
          setError(
            "Cannot refresh recorded operation status. Reconnect before continuing.",
          );
      }
    }
    void read();
    const timer = setInterval(() => void read(), 2000);
    return () => {
      disposed = true;
      clearInterval(timer);
    };
  }, [operation?.operationId]);
  async function action(fn: () => Promise<void>) {
    if (busy) return;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      await fn();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Operation failed.");
    } finally {
      setBusy(false);
    }
  }
  function changed(value: string) {
    setText(value);
    setPreview(null);
    setApproved(false);
    setNotice("");
  }
  async function save() {
    const spec = parseCustomerConfiguration(text);
    if (network && spec.metadata.id !== network.desired.metadata.id)
      throw new Error(
        "A saved network identifier cannot change. Create a separate design.",
      );
    const saved = network
      ? await api.update(network, spec)
      : await api.create(spec);
    setNetwork(saved);
    setText(JSON.stringify(saved.desired, null, 2));
    setPreview(null);
    setApproved(false);
    await load();
    setNotice(
      "Desired configuration saved. Infrastructure has not been changed.",
    );
  }
  function requireSaved() {
    if (!network) throw new Error("Save this configuration first.");
    const candidate = parseCustomerConfiguration(text);
    if (JSON.stringify(candidate) !== JSON.stringify(network.desired))
      throw new Error("Save the latest changes before requesting execution.");
    return network;
  }
  const running = operation?.status === "running";
  const latestVerified =
    operation?.status === "succeeded" &&
    observed?.verified &&
    observed.operationId === operation.operationId;
  let summary = "No configuration loaded.";
  try {
    summary = customerSummary(parseCustomerConfiguration(text));
  } catch {
    /* editor can contain an unfinished document */
  }
  return (
    <section
      className="smp-card smp-custom-networks"
      aria-label="Customer-defined Fabric networks"
    >
      <h2>Customer-defined Fabric networks</h2>
      <p>
        Configure organizations, peers, authorities, Raft orderers, channels,
        anchor peers and approved chaincode artifacts. Saved designs and
        recorded verification are shown separately.
      </p>
      {!capabilities?.enabled && (
        <p role="status">
          Dynamic execution is disabled on this API. Saving a design does not
          deploy it.
        </p>
      )}
      <label>
        Saved customer network
        <select
          value={network?.id ?? ""}
          disabled={busy || running}
          onChange={(e) =>
            void action(async () => {
              setPreview(null);
              setApproved(false);
              setObserved(null);
              setOperation(null);
              if (e.target.value) await load(e.target.value);
              else {
                setNetwork(null);
                setText("");
              }
            })
          }
        >
          <option value="">New customer configuration</option>
          {items.map((n) => (
            <option key={n.id} value={n.id}>
              {n.desired.metadata.name} (revision {n.revision})
            </option>
          ))}
        </select>
      </label>
      <label>
        Import network JSON
        <input
          type="file"
          accept="application/json,.json"
          disabled={busy || running}
          onChange={(e) => {
            const file = e.target.files?.[0];
            if (file)
              void action(async () => {
                if (file.size > 1024 * 1024)
                  throw new Error("Configuration exceeds 1 MiB.");
                const raw = await file.text();
                parseCustomerConfiguration(raw);
                changed(raw);
              });
          }}
        />
      </label>
      <p>{summary}</p>
      <details
        open={editing}
        onToggle={(e) => setEditing(e.currentTarget.open)}
      >
        <summary>Edit desired configuration</summary>
        <label>
          Network specification
          <textarea
            aria-label="Customer network JSON"
            rows={22}
            value={text}
            disabled={busy || running}
            spellCheck={false}
            onChange={(e) => changed(e.target.value)}
          />
        </label>
        <p>
          JSON contains public configuration only. Passwords, tokens, private
          keys and executable templates are not accepted.
        </p>
      </details>
      <details>
        <summary>Approved target inventory and code artifacts</summary>
        <p>
          Storage class: {capabilities?.storageClass ?? "unavailable"}; maximum
          volume: {capabilities?.maximumVolumeSize ?? "unavailable"}.
        </p>
        <pre>
          {JSON.stringify(
            {
              placements: capabilities?.placements ?? {},
              artifacts: capabilities?.artifacts ?? {},
            },
            null,
            2,
          )}
        </pre>
      </details>
      <div className="smp-inline-actions">
        <button
          className="smp-btn secondary"
          disabled={busy || running || !text}
          onClick={() => void action(save)}
        >
          Save desired configuration
        </button>
        <button
          className="smp-btn primary"
          disabled={
            busy || running || !network || !capabilities?.enabled || !!operation
          }
          onClick={() =>
            void action(async () => {
              setOperation(await api.deploy(requireSaved()));
              setObserved(null);
            })
          }
        >
          Deploy saved network
        </button>
        <button
          className="smp-btn secondary"
          disabled={
            busy || running || !latestVerified || !capabilities?.enabled
          }
          onClick={() =>
            void action(async () => {
              setPreview(
                await api.previewDynamic(
                  requireSaved(),
                  operation!.operationId,
                ),
              );
              setApproved(false);
            })
          }
        >
          Review running-network change
        </button>
      </div>
      {preview && (
        <div className="notice-bar">
          <div>
            <h3>Review configuration change</h3>
            <p>Before: {customerSummary(preview.before)}</p>
            <p>After: {customerSummary(preview.after)}</p>
            <details>
              <summary>Complete before and after specifications</summary>
              <pre>
                {JSON.stringify(
                  { before: preview.before, after: preview.after },
                  null,
                  2,
                )}
              </pre>
            </details>
            <p>{preview.recovery}</p>
            <label>
              <input
                type="checkbox"
                checked={approved}
                disabled={busy}
                onChange={(e) => setApproved(e.target.checked)}
              />{" "}
              I approve this exact saved revision and its membership, policy and
              code changes.
            </label>
            <button
              className="smp-btn primary"
              disabled={busy || !approved}
              onClick={() =>
                void action(async () => {
                  setOperation(
                    await api.applyDynamic(
                      requireSaved(),
                      preview.parentOperationId,
                      preview.digest,
                    ),
                  );
                  setObserved(null);
                  setPreview(null);
                  setApproved(false);
                })
              }
            >
              Apply approved change
            </button>
          </div>
        </div>
      )}
      {operation && (
        <section aria-label="Recorded custom operation">
          <h3>Operation {operation.operationId}</h3>
          <p role="status">
            {operation.status} · {operation.currentStage ?? "queued"}
          </p>
          <ol>
            {operation.stages.map((s) => (
              <li key={s.key}>
                {s.label}: {s.status}
              </li>
            ))}
          </ol>
          {["failed", "interrupted"].includes(operation.status) && (
            <button
              className="smp-btn secondary"
              disabled={busy}
              onClick={() =>
                void action(async () => {
                  setOperation(await api.retryOperation(operation.operationId));
                  setObserved(null);
                })
              }
            >
              Retry recorded operation
            </button>
          )}
          <OperationDiagnosticsButton operationId={operation.operationId} />
        </section>
      )}
      {observed && (
        <section aria-label="Recorded Fabric verification">
          <h3>Recorded Fabric verification</h3>
          <p>
            {observed.verified
              ? "Committed transaction verification"
              : "Verification incomplete"}{" "}
            · {observed.checkedAt}
          </p>
          <p>These ledger observations are from this completed operation.</p>
          {observed.verifications?.map((v) => (
            <div key={v.health.channel + "/" + v.health.chaincode.name}>
              <h4>
                {v.health.channel} / {v.health.chaincode.name}
              </h4>
              <p>
                Executable {v.codeVersion}; definition{" "}
                {v.health.chaincode.version}, sequence{" "}
                {v.health.chaincode.sequence}
              </p>
              <p>
                Transaction: <code>{v.transactionId}</code>
              </p>
              <table>
                <thead>
                  <tr>
                    <th>Peer</th>
                    <th>MSP</th>
                    <th>Height</th>
                    <th>Block hash</th>
                  </tr>
                </thead>
                <tbody>
                  {v.health.peers.map((p) => (
                    <tr key={p.peer}>
                      <td>{p.peer}</td>
                      <td>{p.mspId}</td>
                      <td>{p.height}</td>
                      <td>
                        <code>{p.currentBlockHash}</code>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ))}
        </section>
      )}
      {error && (
        <p className="error-box" role="alert">
          {error}
        </p>
      )}
      {notice && <p role="status">{notice}</p>}
    </section>
  );
}
