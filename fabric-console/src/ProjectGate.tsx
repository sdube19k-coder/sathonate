import { useEffect, useState, type ReactNode } from "react";
import { api } from "./api";
import { setWorkspace, WorkspaceContext, type Project } from "./workspace";

export default function ProjectGate({
  children,
  signOut,
}: {
  children: ReactNode;
  signOut: ReactNode;
}) {
  const [projects, setProjects] = useState<Project[]>([]);
  const [selected, setSelected] = useState("");
  const [project, setProject] = useState<Project | null>(null);
  const [error, setError] = useState("");
  const [identity, setIdentity] = useState<{
    issuer: string;
    subject: string;
  } | null>(null);
  const [retry, setRetry] = useState(0);
  useEffect(() => {
    let active = true;
    setWorkspace(null);
    setError("");
    void api
      .projects()
      .then((result) => {
        if (active) setProjects(result.items);
      })
      .catch(() => {
        if (active)
          setError(
            "Your Okta session has no authorized project. An administrator must approve a server-owned membership.",
          );
        void api
          .verifiedIdentity()
          .then((value) => {
            if (active) setIdentity(value);
          })
          .catch(() => {});
      });
    return () => {
      active = false;
      setWorkspace(null);
    };
  }, [retry]);
  if (project)
    return (
      <WorkspaceContext.Provider value={project}>
        {children}
      </WorkspaceContext.Provider>
    );
  return (
    <main className="auth-screen">
      <section className="auth-card">
        <div className="eyebrow">SATHONATE - OKTA</div>
        <h1>Open your authorized project</h1>
        {error && (
          <div role="alert" className="error-box">
            {error}
          </div>
        )}
        {identity && (
          <dl>
            <dt>Verified issuer</dt>
            <dd>{identity.issuer}</dd>
            <dt>Verified subject</dt>
            <dd>{identity.subject}</dd>
          </dl>
        )}
        {projects.length ? (
          <form
            onSubmit={(event) => {
              event.preventDefault();
              const value = projects.find((p) => p.id === selected);
              if (!value) {
                setError("Select a server-authorized project.");
                return;
              }
              setWorkspace(value);
              setProject(value);
            }}
          >
            <label>
              Authorized project
              <select
                aria-label="Authorized project"
                required
                value={selected}
                onChange={(event) => setSelected(event.target.value)}
              >
                <option value="">Select your project</option>
                {projects.map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.tenantId} / {p.id}
                  </option>
                ))}
              </select>
            </label>
            <button className="button primary">Open project</button>
          </form>
        ) : (
          !error && <p>Loading server-authorized projects...</p>
        )}
        {error && (
          <button
            className="button secondary"
            onClick={() => {
              setIdentity(null);
              setRetry((value) => value + 1);
            }}
          >
            Retry project authorization
          </button>
        )}
        {signOut}
      </section>
    </main>
  );
}
