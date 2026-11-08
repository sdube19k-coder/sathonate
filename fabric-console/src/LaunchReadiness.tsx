import { useEffect, useState } from "react";
import {
  ClipboardList,
  Cloud,
  Download,
  RefreshCw,
  ShieldCheck,
} from "lucide-react";
import { api, type LaunchReadiness } from "./api";
import { Card, Note, Status } from "./SmpScreens";
import { formatDate } from "./components";

const template = `# Non-secret server configuration. Configure AWS SSO locally; never add keys here.
CLOUD_AWS_ACCOUNT_ID=
CLOUD_AWS_PROFILE=
CLOUD_AWS_REGION=
CLOUD_EKS_CLUSTER=
CLOUD_KUBE_CONTEXT=
CLOUD_STORAGE_CLASS=
CONSOLE_PUBLIC_URL=
API_PUBLIC_URL=
OKTA_ISSUER=
OKTA_CLIENT_ID=
OKTA_AUDIENCE=
OKTA_TENANT_CLAIM=sathonate_tenant
`;
export function CloudReadiness() {
  const [report, setReport] = useState<LaunchReadiness | null>(null),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  useEffect(() => {
    let active = true;
    api
      .readiness()
      .then((r) => {
        if (active) setReport(r);
      })
      .catch((e) => {
        if (active) setError(e.message);
      });
    return () => {
      active = false;
    };
  }, []);
  const probe = async () => {
    setBusy(true);
    setError("");
    try {
      setReport(await api.probeReadiness());
    } catch (e) {
      setError(e instanceof Error ? e.message : "Readiness checks failed");
    } finally {
      setBusy(false);
    }
  };
  const download = () => {
    const u = URL.createObjectURL(new Blob([template], { type: "text/plain" }));
    const a = document.createElement("a");
    a.href = u;
    a.download = "fabric-smp-cloud.env.template";
    a.click();
    URL.revokeObjectURL(u);
  };
  return (
    <div className="smp-stack smp-launch-readiness">
      <Card
        title="Cloud Launch Readiness"
        icon={Cloud}
        action={
          <Status tone={report?.ready ? "green" : "amber"}>
            {report?.ready ? "Qualified" : "Setup required"}
          </Status>
        }
      >
        <p className="smp-readiness-intro">
          Prepare the AWS/EKS account and Okta sign-in setup below, then verify
          access. Configuration alone does not qualify a cloud deployment. The
          local Fabric network remains available under Networks.
        </p>
        <Note>
          Configuration, live access and production qualification are separate
          checks. This screen runs read-only checks and never creates paid
          resources. Cloud execution remains disabled.
        </Note>
        <div className="smp-inline-actions">
          <button
            className="smp-btn primary"
            disabled={busy || !report}
            onClick={() => void probe()}
          >
            <RefreshCw size={16} className={busy ? "spin" : ""} />
            {busy ? "Checking access…" : "Run Read-only Cloud Checks"}
          </button>
          <button className="smp-btn secondary" onClick={download}>
            <Download size={16} />
            Download Configuration Template
          </button>
          <small>
            {report
              ? `Last checked ${formatDate(report.checkedAt)}`
              : error
                ? "Readiness unavailable for this session"
                : "Loading server readiness…"}
          </small>
        </div>
        {error && (
          <p role="alert" className="smp-error">
            {error}
          </p>
        )}
      </Card>
      <Card title="What Sathonate needs to provide" icon={ClipboardList}>
        <ol className="smp-launch-actions">
          <li>
            <strong>AWS account administrator</strong>
            <p>
              Create the company AWS account, configure MFA and AWS IAM Identity
              Center/SSO, choose a region and a budget. Configure an authorized
              short-lived profile locally.
            </p>
          </li>
          <li>
            <strong>Cluster and infrastructure decision</strong>
            <p>
              Approve the EKS/VPC/node/storage design and expected cost before
              creation. Use a dedicated EKS context, encrypted EBS storage and
              scoped deployment permissions.
            </p>
          </li>
          <li>
            <strong>Okta administrator</strong>
            <p>
              Create a SPA application using authorization code + PKCE and a
              custom authorization server. Configure the API audience,
              tenant/role claims and fabric.read/fabric.write scopes.
            </p>
          </li>
          <li>
            <strong>Owned domain and HTTPS</strong>
            <p>
              Choose the console/API domains and configure DNS, certificates,
              Okta callback URLs and trusted origins. No passwords, access keys,
              client secrets or kubeconfig tokens belong in the network JSON.
            </p>
          </li>
        </ol>
      </Card>
      {[
        "AWS",
        "Kubernetes",
        "Domains",
        "Okta",
        "Release gate",
        "Live checks",
      ].map((group) => {
        const checks = report?.checks.filter((c) => c.group === group) ?? [];
        return checks.length ? (
          <Card
            key={group}
            title={group}
            icon={group === "Okta" ? ShieldCheck : undefined}
          >
            <div className="smp-readiness-checks">
              {checks.map((check) => (
                <div key={check.id} className="smp-readiness-check">
                  <div>
                    <strong>{check.title}</strong>
                    <p>{check.detail}</p>
                    {check.required && <small>{check.required}</small>}
                  </div>
                  <Status
                    tone={
                      check.status === "passed"
                        ? "green"
                        : check.status === "configured"
                          ? "blue"
                          : "amber"
                    }
                  >
                    {check.status.replace("_", " ")}
                  </Status>
                </div>
              ))}
            </div>
          </Card>
        ) : null;
      })}
    </div>
  );
}
