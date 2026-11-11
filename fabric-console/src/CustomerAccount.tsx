import { useEffect, useRef, useState } from "react";
import { request } from "./api";
import { authorizeCustomerChanges } from "./AuthGate";

type Invitation = {
  id: string;
  recipient: { subject: string };
  expiresAt: string;
  state: string;
};
type Customer = {
  account: {
    name: string;
    isOwner: boolean;
    projects: { id: string; name: string; state: string }[];
    subscription: { plan: string; status: string; until: string };
    invoices: {
      id: string;
      currency: string;
      amountMinor: number;
      status: string;
    }[];
    invitations: Invitation[];
    pendingInvitations: Invitation[];
    usage?: { month: string; verifiedOperations: number }[];
    activeEntitlement: boolean;
  };
  plans: {
    id: string;
    monthlyMinor: number;
    currency: string;
    activeProjects: number;
  }[];
  canManage: boolean;
  paymentsEnabled: false;
  infrastructureEnabled: false;
};

function money(amount: number, currency: string) {
  return new Intl.NumberFormat(undefined, {
    style: "currency",
    currency,
  }).format(amount / 100);
}

export default function CustomerAccount() {
  const [record, setRecord] = useState<Customer | null>(null);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);
  const [name, setName] = useState("");
  const [subject, setSubject] = useState("");
  const [organizationName, setOrganizationName] = useState("");
  const pending = useRef<{ payload: string; key: string } | null>(null);
  const saving = useRef(false);
  useEffect(() => {
    const controller = new AbortController();
    request<Customer>("/customer", { signal: controller.signal })
      .then((data) => {
        setRecord(data);
        setOrganizationName(data.account.name);
      })
      .catch((e: Error) => {
        if (!controller.signal.aborted) setError(e.message);
      });
    return () => controller.abort();
  }, []);

  async function submit(
    action: string,
    fields: { name?: string; reference?: string } = {},
  ) {
    if (saving.current) return;
    const body = JSON.stringify({ action, ...fields });
    if (pending.current?.payload !== body)
      pending.current = { payload: body, key: crypto.randomUUID() };
    saving.current = true;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      await request<void>(
        action === "accept-invitation"
          ? "/customer/invitations/accept"
          : "/customer/actions",
        {
          method: "POST",
          headers: { "Idempotency-Key": pending.current.key },
          body,
        },
      );
      const updated = await request<Customer>("/customer");
      setRecord(updated);
      setOrganizationName(updated.account.name);
      pending.current = null;
      setNotice("Account updated.");
      if (action === "request-project") setName("");
      if (action === "invite") setSubject("");
    } catch (e) {
      setError(
        e instanceof Error ? e.message : "The account could not be updated.",
      );
    } finally {
      saving.current = false;
      setBusy(false);
    }
  }

  if (!record)
    return (
      <p role={error ? "alert" : "status"}>
        {error || "Loading customer account…"}
      </p>
    );
  const { account, plans, canManage } = record;
  const subscription = account.subscription;

  return (
    <div className="customer-account">
      {error && <p role="alert">{error}</p>}
      {notice && <p role="status">{notice}</p>}
      <section
        className="smp-card customer-panel"
        aria-labelledby="account-subscription"
      >
        <h2 id="account-subscription">
          {account.name || "Organization invitation"}
        </h2>
        {account.name && (
          <>
            <p>
              {subscription.status
                ? `${subscription.plan} · ${subscription.status.replaceAll("_", " ")}`
                : "No subscription"}
            </p>
            {subscription.status && (
              <p>
                {account.activeEntitlement
                  ? "Entitlement active"
                  : "Entitlement inactive"}
              </p>
            )}
            {canManage && !subscription.status && (
              <button
                type="button"
                className="smp-btn primary"
                disabled={busy}
                onClick={() => void submit("start-trial")}
              >
                Start 14-day trial
              </button>
            )}
            {canManage && (
              <form
                onSubmit={(e) => {
                  e.preventDefault();
                  void submit("rename-organization", {
                    name: organizationName,
                  });
                }}
              >
                <label htmlFor="customer-name">Organization name</label>
                <input
                  id="customer-name"
                  value={organizationName}
                  onChange={(e) => setOrganizationName(e.target.value)}
                  minLength={2}
                  maxLength={100}
                  required
                />
                <button className="smp-btn secondary" disabled={busy}>
                  Save organization name
                </button>
              </form>
            )}
          </>
        )}
        {account.isOwner && !canManage && (
          <button
            className="smp-btn secondary"
            onClick={() =>
              void authorizeCustomerChanges().catch((e: Error) =>
                setError(e.message),
              )
            }
          >
            Authorize account changes in Okta
          </button>
        )}
        <p>
          Payments and infrastructure provisioning are disabled. Your Kubernetes
          provider bills infrastructure separately.
        </p>
        <ul>
          {plans.map((plan) => (
            <li key={plan.id}>
              {plan.id} · {money(plan.monthlyMinor, plan.currency)} per month ·{" "}
              {plan.activeProjects} project slots
            </li>
          ))}
        </ul>
      </section>

      {account.pendingInvitations.length > 0 && (
        <section className="smp-card customer-panel">
          <h2>Pending invitations</h2>
          {account.pendingInvitations.map((invite) => (
            <button
              key={invite.id}
              className="smp-btn secondary"
              disabled={busy}
              onClick={() =>
                void submit("accept-invitation", { reference: invite.id })
              }
            >
              Accept invitation expiring{" "}
              {new Date(invite.expiresAt).toLocaleDateString()}
            </button>
          ))}
        </section>
      )}

      {account.name && (
        <section className="smp-card customer-panel">
          <h2>Projects</h2>
          {account.projects.length === 0 ? (
            <p>No customer projects yet.</p>
          ) : (
            <ul>
              {account.projects.map((project) => (
                <li key={project.id}>
                  {project.name} · {project.state.replaceAll("-", " ")}
                </li>
              ))}
            </ul>
          )}
          {canManage && account.activeEntitlement && (
            <form
              onSubmit={(e) => {
                e.preventDefault();
                void submit("request-project", { name });
              }}
            >
              <label htmlFor="customer-project-name">New project name</label>
              <input
                id="customer-project-name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                minLength={2}
                maxLength={100}
                required
              />
              <button className="smp-btn primary" disabled={busy}>
                Request project
              </button>
            </form>
          )}
        </section>
      )}

      {account.isOwner && (
        <>
          <section className="smp-card customer-panel">
            <h2>Invitations</h2>
            <p>
              Invite someone who already has an approved project grant in this
              tenant.
            </p>
            {canManage && (
              <form
                onSubmit={(e) => {
                  e.preventDefault();
                  void submit("invite", { reference: subject });
                }}
              >
                <label htmlFor="customer-subject">
                  Verified identity subject
                </label>
                <input
                  id="customer-subject"
                  value={subject}
                  onChange={(e) => setSubject(e.target.value)}
                  maxLength={255}
                  required
                />
                <button className="smp-btn secondary" disabled={busy}>
                  Create invitation
                </button>
              </form>
            )}
            <ul>
              {account.invitations.map((invite) => (
                <li key={invite.id}>
                  {invite.recipient.subject} · {invite.state}{" "}
                  {canManage && invite.state === "pending" && (
                    <button
                      type="button"
                      className="smp-btn secondary"
                      disabled={busy}
                      onClick={() =>
                        void submit("revoke-invitation", {
                          reference: invite.id,
                        })
                      }
                    >
                      Revoke invitation
                    </button>
                  )}
                </li>
              ))}
            </ul>
          </section>
          <section className="smp-card customer-panel">
            <h2>Invoices and usage</h2>
            {account.invoices.length === 0 ? (
              <p>No invoices.</p>
            ) : (
              <ul>
                {account.invoices.map((invoice) => (
                  <li key={invoice.id}>
                    {invoice.id} ·{" "}
                    {money(invoice.amountMinor, invoice.currency)} ·{" "}
                    {invoice.status}
                  </li>
                ))}
              </ul>
            )}
            {account.usage?.length ? (
              <ul>
                {account.usage.map((period) => (
                  <li key={period.month}>
                    {period.month} · {period.verifiedOperations} verified
                    platform operations
                  </li>
                ))}
              </ul>
            ) : (
              <p>No metered operations.</p>
            )}
          </section>
        </>
      )}
    </div>
  );
}
