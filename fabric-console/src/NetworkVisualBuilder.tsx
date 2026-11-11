import { useState } from "react";
import {
  ArrowLeft,
  ArrowRight,
  Building2,
  CheckCircle2,
  Code2,
  GitBranch,
  Layers,
  Plus,
  Server,
} from "lucide-react";
import type {
  DynamicCapabilities,
  DynamicResources,
  NetworkSpec,
} from "./types";
import { addVisualEntity, visualWarnings } from "./visual-network";
import { customerSummary } from "./custom-configuration";

const steps = [
  "Network",
  "Organizations & CAs",
  "Peers & orderers",
  "Channels",
  "Chaincode",
  "Review",
];
function Add({
  kind,
  value,
  capabilities,
  onChange,
  onError,
}: {
  kind: Parameters<typeof addVisualEntity>[1];
  value: NetworkSpec;
  capabilities?: DynamicCapabilities;
  onChange: (n: NetworkSpec) => void;
  onError: (message: string) => void;
}) {
  const s = value.spec,
    placements = Object.keys(s.configuration!.placements);
  const [id, setId] = useState(""),
    [org, setOrg] = useState(s.organizations[0]?.id ?? ""),
    [placement, setPlacement] = useState(placements[0] ?? "");
  return (
    <div className="visual-add">
      <label>
        New {kind} identifier
        <input
          value={id}
          placeholder={`${kind}-identifier`}
          onChange={(e) => setId(e.target.value)}
        />
      </label>
      {!["organization", "channel"].includes(kind) && (
        <>
          <label>
            Organization
            <select
              aria-label={`${kind} organization`}
              value={org}
              onChange={(e) => setOrg(e.target.value)}
            >
              <option value="">Select organization</option>
              {s.organizations.map((o) => (
                <option key={o.id}>{o.id}</option>
              ))}
            </select>
          </label>
          <label>
            Placement
            <select
              value={placement}
              onChange={(e) => setPlacement(e.target.value)}
            >
              <option value="">Select approved placement</option>
              {placements.map((p) => (
                <option key={p}>{p}</option>
              ))}
            </select>
          </label>
        </>
      )}
      <button
        className="smp-btn secondary"
        type="button"
        onClick={() => {
          try {
            onChange(
              addVisualEntity(
                value,
                kind,
                id,
                org,
                placement,
                capabilities?.storageClass ?? "",
              ),
            );
            setId("");
            onError("");
          } catch (e) {
            onError(
              e instanceof Error ? e.message : "Cannot add this component.",
            );
          }
        }}
      >
        <Plus size={16} />
        Add {kind}
      </button>
    </div>
  );
}
function Resources({
  id,
  resources,
  onUpdate,
}: {
  id: string;
  resources: DynamicResources;
  onUpdate: (r: DynamicResources) => void;
}) {
  return (
    <div className="visual-resources">
      {(
        [
          ["cpuRequestMilli", "CPU request (mCPU)"],
          ["cpuLimitMilli", "CPU limit (mCPU)"],
          ["memoryRequestMiB", "Memory request (MiB)"],
          ["memoryLimitMiB", "Memory limit (MiB)"],
        ] as const
      ).map(([key, label]) => (
        <label key={key}>
          {label}
          <input
            aria-label={`${id} ${label}`}
            type="number"
            min={1}
            value={resources[key]}
            onChange={(e) =>
              onUpdate({ ...resources, [key]: Number(e.target.value) })
            }
          />
        </label>
      ))}
    </div>
  );
}

export default function NetworkVisualBuilder({
  value,
  onChange,
  capabilities,
  disabled = false,
  deployed = false,
}: {
  value: NetworkSpec;
  onChange: (next: NetworkSpec) => void;
  capabilities?: DynamicCapabilities;
  disabled?: boolean;
  deployed?: boolean;
}) {
  const [step, setStep] = useState(0),
    [error, setError] = useState("");
  const s = value.spec,
    cfg = s.configuration!,
    placements = Object.keys(cfg.placements);
  const update = (fn: (next: NetworkSpec) => void) => {
    const next = structuredClone(value);
    fn(next);
    onChange(next);
    setError("");
  };
  const checkList = (
    label: string,
    items: { id: string; label?: string }[],
    selected: string[],
    change: (ids: string[]) => void,
  ) => (
    <div className="visual-checklist">
      <span>{label}</span>
      {items.map((item) => (
        <label key={item.id}>
          <input
            type="checkbox"
            checked={selected.includes(item.id)}
            onChange={(e) =>
              change(
                e.target.checked
                  ? [...selected, item.id]
                  : selected.filter((id) => id !== item.id),
              )
            }
          />
          {item.label ?? item.id}
        </label>
      ))}
      {!items.length && <small>No eligible members yet.</small>}
    </div>
  );
  return (
    <section className="visual-builder" aria-label="Visual network builder">
      <div className="visual-builder-heading">
        <div>
          <span className="smp-eyebrow">CONFIGURATION-DRIVEN FABRIC</span>
          <h2>
            {deployed ? "Review and expand your network" : "Build your network"}
          </h2>
          <p>
            Desired configuration only. The server validates every save and
            approves every deployment change.
          </p>
        </div>
        <Layers size={26} />
      </div>
      <div
        role="tablist"
        aria-label="Network design steps"
        className="visual-steps"
      >
        {steps.map((title, i) => (
          <button
            type="button"
            role="tab"
            id={`visual-step-${i}`}
            aria-controls="visual-step-content"
            aria-selected={step === i}
            tabIndex={step === i ? 0 : -1}
            onKeyDown={(event) => {
              const next =
                event.key === "ArrowRight"
                  ? (i + 1) % steps.length
                  : event.key === "ArrowLeft"
                    ? (i + steps.length - 1) % steps.length
                    : event.key === "Home"
                      ? 0
                      : event.key === "End"
                        ? steps.length - 1
                        : -1;
              if (next >= 0) {
                event.preventDefault();
                setStep(next);
                event.currentTarget.parentElement
                  ?.querySelectorAll<HTMLButtonElement>("button")
                  [next]?.focus();
              }
            }}
            key={title}
            onClick={() => {
              setStep(i);
              setError("");
            }}
          >
            <span>{i + 1}</span>
            {title}
          </button>
        ))}
      </div>
      {error && (
        <p role="alert" className="smp-error">
          {error}
        </p>
      )}
      <div
        role="tabpanel"
        id="visual-step-content"
        aria-labelledby={`visual-step-${step}`}
      >
        <fieldset disabled={disabled} className="visual-panel">
          {step === 0 && (
            <>
              <div className="smp-fields two">
                <label>
                  Network name
                  <input
                    value={value.metadata.name}
                    onChange={(e) =>
                      update((n) => {
                        n.metadata.name = e.target.value;
                      })
                    }
                  />
                </label>
                <label>
                  Network identifier
                  <input
                    value={value.metadata.id}
                    disabled={deployed}
                    onChange={(e) =>
                      update((n) => {
                        n.metadata.id = e.target.value;
                      })
                    }
                  />
                </label>
              </div>
              <div className="visual-context">
                <GitBranch size={22} />
                <div>
                  <strong>Raft · Fabric {s.fabricVersion}</strong>
                  <p>
                    Local development · TLS/MSP validation · {placements.length}{" "}
                    configured placements
                  </p>
                </div>
              </div>
              <p>
                Runtime placements, storage and executable artifacts come from
                the administrator-approved target. BFT and production execution
                are not qualified.
              </p>
            </>
          )}
          {step === 1 && (
            <>
              <h3>
                <Building2 size={18} />
                Organizations
              </h3>
              {s.organizations.map((o, i) => (
                <div className="visual-entity" key={o.id}>
                  <strong>{o.id}</strong>
                  <div className="smp-fields two">
                    <label>
                      MSP identifier
                      <input
                        value={o.mspId}
                        disabled={deployed}
                        onChange={(e) =>
                          update((n) => {
                            n.spec.organizations[i].mspId = e.target.value;
                          })
                        }
                      />
                    </label>
                    <label>
                      Signing identity provider
                      <select
                        value={o.identityProviderRef}
                        disabled={deployed}
                        onChange={(e) =>
                          update((n) => {
                            n.spec.organizations[i].identityProviderRef =
                              e.target.value;
                          })
                        }
                      >
                        <option value="">Select signing CA</option>
                        {cfg.authorities
                          .filter(
                            (a) =>
                              a.organizationRef === o.id &&
                              a.roles.includes("signing"),
                          )
                          .map((a) => (
                            <option key={a.id}>{a.id}</option>
                          ))}
                      </select>
                    </label>
                  </div>
                </div>
              ))}
              <Add
                kind="organization"
                value={value}
                capabilities={capabilities}
                onChange={onChange}
                onError={setError}
              />
              <h3>Certificate authorities</h3>
              {cfg.authorities.map((a, i) => (
                <div className="visual-entity" key={a.id}>
                  <strong>{a.id}</strong>
                  <p>
                    {a.organizationRef} · {a.placementRef}
                  </p>
                  {checkList(
                    "Authority roles",
                    [{ id: "signing" }, { id: "tls" }],
                    a.roles,
                    (roles) =>
                      update((n) => {
                        n.spec.configuration!.authorities[i].roles = roles as (
                          "signing" | "tls"
                        )[];
                      }),
                  )}
                  <Resources
                    id={a.id}
                    resources={a.resources}
                    onUpdate={(r) =>
                      update((n) => {
                        n.spec.configuration!.authorities[i].resources = r;
                      })
                    }
                  />
                </div>
              ))}
              <Add
                kind="authority"
                value={value}
                capabilities={capabilities}
                onChange={onChange}
                onError={setError}
              />
            </>
          )}
          {step === 2 && (
            <>
              <h3>
                <Server size={18} />
                Peers and Raft orderers
              </h3>
              {s.nodes.map((node, i) => (
                <details className="visual-entity" key={node.id}>
                  <summary>
                    {node.id} · {node.role} · {node.organizationRef}
                  </summary>
                  <div className="smp-fields two">
                    <label>
                      Placement
                      <select
                        value={node.placementRef}
                        disabled={deployed}
                        onChange={(e) =>
                          update((n) => {
                            n.spec.nodes[i].placementRef = e.target.value;
                          })
                        }
                      >
                        {placements.map((p) => (
                          <option key={p}>{p}</option>
                        ))}
                      </select>
                    </label>
                    <label>
                      Volume capacity (GiB)
                      <input
                        type="number"
                        min={1}
                        value={cfg.nodes[node.id]?.storage.capacityGiB ?? 1}
                        disabled={deployed}
                        onChange={(e) =>
                          update((n) => {
                            n.spec.configuration!.nodes[
                              node.id
                            ].storage.capacityGiB = Number(e.target.value);
                          })
                        }
                      />
                    </label>
                  </div>
                  {cfg.nodes[node.id] && (
                    <Resources
                      id={node.id}
                      resources={cfg.nodes[node.id].resources}
                      onUpdate={(r) =>
                        update((n) => {
                          n.spec.configuration!.nodes[node.id].resources = r;
                        })
                      }
                    />
                  )}
                </details>
              ))}
              <Add
                kind="peer"
                value={value}
                capabilities={capabilities}
                onChange={onChange}
                onError={setError}
              />
              <Add
                kind="orderer"
                value={value}
                capabilities={capabilities}
                onChange={onChange}
                onError={setError}
              />
              <p>
                Existing node identifiers, placements and storage are preserved
                during expansion. New nodes use explicitly configured
                organizations and approved placements.
              </p>
            </>
          )}
          {step === 3 && (
            <>
              <h3>
                <GitBranch size={18} />
                Channels and anchors
              </h3>
              {s.channels.map((c, i) => (
                <div className="visual-entity" key={c.id}>
                  <strong>{c.id}</strong>
                  {checkList(
                    "Member organizations",
                    s.organizations,
                    c.organizationRefs,
                    (ids) =>
                      update((n) => {
                        n.spec.channels[i].organizationRefs = ids;
                      }),
                  )}
                  {checkList(
                    "Joining peers",
                    s.nodes.filter((n) => n.role === "peer"),
                    c.peerRefs,
                    (ids) =>
                      update((n) => {
                        n.spec.channels[i].peerRefs = ids;
                      }),
                  )}
                  {checkList(
                    "Raft members",
                    s.nodes.filter((n) => n.role === "orderer"),
                    c.ordererRefs,
                    (ids) =>
                      update((n) => {
                        n.spec.channels[i].ordererRefs = ids;
                      }),
                  )}
                  {checkList(
                    "Anchor peers",
                    s.nodes.filter(
                      (n) => n.role === "peer" && c.peerRefs.includes(n.id),
                    ),
                    cfg.anchors[c.id] ?? [],
                    (ids) =>
                      update((n) => {
                        n.spec.configuration!.anchors[c.id] = ids;
                      }),
                  )}
                </div>
              ))}
              <Add
                kind="channel"
                value={value}
                capabilities={capabilities}
                onChange={onChange}
                onError={setError}
              />
            </>
          )}
          {step === 4 && (
            <>
              <h3>
                <Code2 size={18} />
                Chaincode packages and policies
              </h3>
              {cfg.chaincodes.map((cc, i) => (
                <div className="visual-entity" key={`${cc.channelRef}/${i}`}>
                  <div className="smp-fields two">
                    <label>
                      Contract name
                      <input
                        value={cc.name}
                        disabled={deployed}
                        onChange={(e) =>
                          update((n) => {
                            n.spec.configuration!.chaincodes[i].name =
                              e.target.value;
                          })
                        }
                      />
                    </label>
                    <label>
                      Channel
                      <select
                        value={cc.channelRef}
                        disabled={deployed}
                        onChange={(e) =>
                          update((n) => {
                            n.spec.configuration!.chaincodes[i].channelRef =
                              e.target.value;
                          })
                        }
                      >
                        {s.channels.map((c) => (
                          <option key={c.id}>{c.id}</option>
                        ))}
                      </select>
                    </label>
                    <label>
                      Approved executable
                      <select
                        value={cc.runtimeImage}
                        onChange={(e) =>
                          update((n) => {
                            const c = n.spec.configuration!.chaincodes[i];
                            c.runtimeImage = e.target.value;
                            c.packageSHA256 =
                              capabilities?.artifacts?.[e.target.value]
                                ?.sha256 ?? "";
                            c.version =
                              capabilities?.artifacts?.[e.target.value]
                                ?.codeVersion ?? c.version;
                          })
                        }
                      >
                        <option value="">Select approved artifact</option>
                        {!capabilities?.artifacts?.[cc.runtimeImage] &&
                          cc.runtimeImage && (
                            <option value={cc.runtimeImage}>
                              Current recorded artifact
                            </option>
                          )}
                        {Object.entries(capabilities?.artifacts ?? {}).map(
                          ([image, a]) => (
                            <option value={image} key={image}>
                              {a.codeVersion} · {image.slice(0, 55)}
                            </option>
                          ),
                        )}
                      </select>
                    </label>
                    <label>
                      Definition sequence
                      <input
                        type="number"
                        min={1}
                        value={cc.sequence}
                        onChange={(e) =>
                          update((n) => {
                            n.spec.configuration!.chaincodes[i].sequence =
                              Number(e.target.value);
                          })
                        }
                      />
                    </label>
                  </div>
                  {checkList(
                    "Endorsing MSPs",
                    s.organizations.map((o) => ({ id: o.mspId })),
                    cc.endorsement.mspIds,
                    (ids) =>
                      update((n) => {
                        n.spec.configuration!.chaincodes[i].endorsement.mspIds =
                          ids;
                      }),
                  )}
                  <label>
                    Required endorsements
                    <input
                      type="number"
                      min={1}
                      max={cc.endorsement.mspIds.length}
                      value={cc.endorsement.minimum}
                      onChange={(e) =>
                        update((n) => {
                          n.spec.configuration!.chaincodes[
                            i
                          ].endorsement.minimum = Number(e.target.value);
                        })
                      }
                    />
                  </label>
                </div>
              ))}
              <button
                type="button"
                className="smp-btn secondary"
                disabled={!s.channels.length}
                onClick={() =>
                  update((n) => {
                    const channel = n.spec.channels[0];
                    n.spec.configuration!.chaincodes.push({
                      name: `contract-${n.spec.configuration!.chaincodes.length + 1}`,
                      channelRef: channel.id,
                      version: "1.0",
                      sequence: 1,
                      packageSHA256: "",
                      runtimeImage: "",
                      endorsement: {
                        mspIds: n.spec.organizations
                          .filter((o) =>
                            channel.organizationRefs.includes(o.id),
                          )
                          .map((o) => o.mspId),
                        minimum: channel.organizationRefs.length,
                      },
                    });
                  })
                }
              >
                <Plus size={16} />
                Add chaincode
              </button>
              <p>
                Only administrator-approved package digests and images can run.
                A code upgrade requires a new approved artifact and an increased
                definition sequence.
              </p>
            </>
          )}
          {step === 5 && (
            <>
              <h3>
                <CheckCircle2 size={18} />
                Review desired topology
              </h3>
              <p>{customerSummary(value)}</p>
              <div className="visual-topology">
                {s.organizations.map((o) => (
                  <div key={o.id}>
                    <Building2 size={20} />
                    <strong>{o.id}</strong>
                    <small>{o.mspId}</small>
                    {s.nodes
                      .filter((n) => n.organizationRef === o.id)
                      .map((n) => (
                        <span key={n.id}>
                          {n.role} · {n.id}
                        </span>
                      ))}
                    {cfg.authorities
                      .filter((a) => a.organizationRef === o.id)
                      .map((a) => (
                        <span key={a.id}>CA · {a.id}</span>
                      ))}
                  </div>
                ))}
              </div>
              {visualWarnings(value).length ? (
                <ul>
                  {visualWarnings(value).map((issue) => (
                    <li key={issue}>{issue}</li>
                  ))}
                </ul>
              ) : (
                <p>
                  Local completeness checks passed. Save to run authoritative
                  server validation, then review the exact deployment or
                  expansion approval below.
                </p>
              )}
              <p>
                No infrastructure has been changed by this editor. Read-only,
                retired-network, revision, ownership and tenant authorization
                checks remain enforced by the API.
              </p>
            </>
          )}
        </fieldset>
      </div>
      <div className="visual-footer">
        <button
          type="button"
          className="smp-btn secondary"
          disabled={step === 0}
          onClick={() => setStep(step - 1)}
        >
          <ArrowLeft size={15} />
          Back
        </button>
        <span>Step {step + 1} of 6</span>
        <button
          type="button"
          className="smp-btn primary"
          disabled={step === 5}
          onClick={() => setStep(step + 1)}
        >
          Next
          <ArrowRight size={15} />
        </button>
      </div>
    </section>
  );
}
