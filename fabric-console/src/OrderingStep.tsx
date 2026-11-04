import { Info, ShieldCheck } from "lucide-react";
import { Badge, Field } from "./components";
import { withOrdererCount, type NetworkForm } from "./network-form";

export default function OrderingStep({
  form,
  onChange,
}: {
  form: NetworkForm;
  onChange: (form: NetworkForm) => void;
}) {
  const bft = form.consensus === "bft";
  const allowed = bft
    ? [4, 7, 10]
    : form.environment === "production"
      ? [3, 5, 7]
      : [1, 3, 5, 7];
  const counts = allowed.includes(form.orderers)
    ? allowed
    : [form.orderers, ...allowed];
  const faults = bft
    ? Math.floor((form.orderers - 1) / 3)
    : Math.floor((form.orderers - 1) / 2);
  const choose = (consensus: "raft" | "bft") => {
    const count =
      consensus === "bft"
        ? form.orderers >= 4 && (form.orderers - 1) % 3 === 0
          ? form.orderers
          : 4
        : form.orderers % 2 === 1
          ? form.orderers
          : 3;
    const version =
      consensus === "bft" && !form.version.startsWith("3.")
        ? "3.1.5"
        : consensus === "raft" &&
            form.destination !== "private" &&
            form.version.startsWith("3.")
          ? "2.5.16"
          : form.version;
    onChange(withOrdererCount({ ...form, consensus, version }, count));
  };
  return (
    <>
      <h2>Configure the ordering service.</h2>
      <p className="section-description">
        Orderers agree on transaction order and create blocks. Choose the
        consensus model, then decide who owns each node.
      </p>
      <div
        className="consensus-options"
        role="group"
        aria-label="Consensus mechanism"
      >
        <button
          className={`consensus-card ${!bft ? "selected" : ""}`}
          aria-pressed={!bft}
          onClick={() => choose("raft")}
        >
          <div>
            <strong>Raft</strong>
            <Badge tone="green">Recommended</Badge>
          </div>
          <p>Crash fault tolerance for trusted consortium members.</p>
          <small>Cloud: Fabric 2.5 · Native: Fabric 2.5 or 3.x</small>
        </button>
        <button
          className={`consensus-card ${bft ? "selected" : ""}`}
          aria-pressed={bft}
          disabled={form.destination !== "private"}
          onClick={() => choose("bft")}
        >
          <div>
            <strong>SmartBFT</strong>
            <Badge tone="blue">Fabric 3.x</Badge>
          </div>
          <p>Byzantine fault tolerance for potentially malicious nodes.</p>
          <small>
            {form.destination === "private"
              ? "Native design only · Choosing this sets Fabric 3.1.5 if needed."
              : "Unavailable for this Bevel cloud integration."}
          </small>
        </button>
        <button className="consensus-card retired" disabled>
          <div>
            <strong>Solo</strong>
            <Badge>Legacy</Badge>
          </div>
          <p>Test-only consensus, removed in Fabric 3.0.</p>
          <small>Use one-node Raft for a development-only design.</small>
        </button>
      </div>
      <div className="ordering-profile">
        <Field
          label="Number of orderers"
          hint={
            bft
              ? "Profiles use 3f+1 nodes: 4 tolerates 1 faulty node; 7 tolerates 2."
              : "Production designs use odd counts: 3 tolerates 1 crash; 5 tolerates 2."
          }
        >
          <select
            value={form.orderers}
            onChange={(e) =>
              onChange(withOrdererCount(form, Number(e.target.value)))
            }
          >
            {counts.map((count) => (
              <option value={count} key={count}>
                {count} orderer{count === 1 ? "" : "s"}
                {count === 1 ? " · Development only" : ""}
              </option>
            ))}
          </select>
        </Field>
        <div className="ordering-tolerance">
          <ShieldCheck size={20} />
          <strong>
            {faults} {bft ? "Byzantine" : "crashed"} node
            {faults === 1 ? "" : "s"}
          </strong>
          <span>
            Theoretical fault budget, assuming independent failures and correct
            configuration.
          </span>
        </div>
      </div>
      <div className="ordering-version">
        <span>Design profile</span>
        <code>Fabric {form.version}</code>
        <Badge>
          {bft ? "V3_0" : form.version.startsWith("3.") ? "V3_0" : "V2_0"}{" "}
          channel capability
        </Badge>
      </div>
      <div className="orderer-editor">
        {form.ordererNodes.map((node, index) => (
          <div className="orderer-config" key={index}>
            <div className="orderer-config-heading">
              <span className="org-number">
                {String(index + 1).padStart(2, "0")}
              </span>
              <strong>Orderer {index + 1}</strong>
              <Badge>Planned</Badge>
            </div>
            <div className="two-fields">
              <Field label={`Orderer ${index + 1} name`}>
                <input
                  value={node.id}
                  maxLength={63}
                  onChange={(e) =>
                    onChange({
                      ...form,
                      ordererNodes: form.ordererNodes.map((item, i) =>
                        i === index ? { ...item, id: e.target.value } : item,
                      ),
                    })
                  }
                />
              </Field>
              <Field label={`Orderer ${index + 1} organization`}>
                <select
                  value={node.organizationKey}
                  onChange={(e) =>
                    onChange({
                      ...form,
                      ordererNodes: form.ordererNodes.map((item, i) =>
                        i === index
                          ? { ...item, organizationKey: e.target.value }
                          : item,
                      ),
                    })
                  }
                >
                  <option value="" disabled>
                    Choose an organization
                  </option>
                  {form.organizations.map((org) => (
                    <option key={org.key} value={org.key}>
                      {org.name}
                    </option>
                  ))}
                </select>
              </Field>
            </div>
            <details className="advanced">
              <summary>Placement references</summary>
              <div className="two-fields">
                <Field
                  label={`Orderer ${index + 1} placement`}
                  hint="Future host or cluster placement reference."
                >
                  <input
                    value={node.placementRef}
                    onChange={(e) =>
                      onChange({
                        ...form,
                        ordererNodes: form.ordererNodes.map((item, i) =>
                          i === index
                            ? { ...item, placementRef: e.target.value }
                            : item,
                        ),
                      })
                    }
                  />
                </Field>
                <Field
                  label={`Orderer ${index + 1} failure domain`}
                  hint="Future rack, zone, or independent failure boundary."
                >
                  <input
                    value={node.failureDomain}
                    onChange={(e) =>
                      onChange({
                        ...form,
                        ordererNodes: form.ordererNodes.map((item, i) =>
                          i === index
                            ? { ...item, failureDomain: e.target.value }
                            : item,
                        ),
                      })
                    }
                  />
                </Field>
              </div>
            </details>
          </div>
        ))}
      </div>
      <div className="info-note">
        <Info size={17} />
        <p>
          These references describe desired placement only. Actual hosts,
          independence, organization trust, certificates, and engine
          compatibility must be verified before deployment. BFT execution is not
          implemented.
        </p>
      </div>
    </>
  );
}
