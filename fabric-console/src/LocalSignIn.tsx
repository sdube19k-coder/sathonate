import { useEffect, useRef, useState, type ReactNode } from "react";
import { LogOut } from "lucide-react";
import { api, setTokenProvider } from "./api";
import {
  setWorkspace,
  WorkspaceContext,
  WorkspaceSignOut,
  type Project,
} from "./workspace";
export default function LocalSignIn({ children }: { children: ReactNode }) {
  const token = useRef<string | undefined>(undefined);
  const [projects, setProjects] = useState<Project[]>([]),
    [selected, setSelected] = useState("");
  const [project, setProject] = useState<Project | null>(null),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  const clear = () => {
    token.current = undefined;
    setTokenProvider(null);
    setWorkspace(null);
    setProjects([]);
    setProject(null);
    setSelected("");
  };
  useEffect(() => {
    const expire = () => {
      clear();
      setError(
        "Your token is invalid or expired. Sign in again to restore saved deployment progress.",
      );
    };
    window.addEventListener("local-auth-expired", expire);
    return () => {
      window.removeEventListener("local-auth-expired", expire);
      token.current = undefined;
      setTokenProvider(null);
      setWorkspace(null);
    };
  }, []);
  if (project)
    return (
      <WorkspaceContext.Provider value={project}>
        <WorkspaceSignOut.Provider
          value={
            <button
              className="smp-icon-btn"
              aria-label="Sign out of local workspace"
              onClick={clear}
            >
              <LogOut size={17} />
            </button>
          }
        >
          {children}
        </WorkspaceSignOut.Provider>
      </WorkspaceContext.Provider>
    );
  return (
    <main className="auth-screen">
      <section className="auth-card">
        <div className="eyebrow">SATHONATE ... LOCAL DEVELOPMENT</div>
        <h1>Sign in to your project</h1>
        <p>
          Use a short-lived development token issued for your identity and
          project. Tokens remain in memory. After a refresh, sign in again to
          restore server-persisted progress.
        </p>
        {error && (
          <div role="alert" className="error-box">
            {error}
          </div>
        )}
        {!projects.length ? (
          <form
            onSubmit={async (event) => {
              event.preventDefault();
              if (busy) return;
              setBusy(true);
              setError("");
              const form = event.currentTarget;
              const value = String(new FormData(form).get("token") ?? "");
              form.reset();
              try {
                if (!/^[a-f0-9]{64}$/.test(value))
                  throw new Error("Enter a valid development token.");
                token.current = value;
                setTokenProvider(() => token.current);
                const result = await api.projects();
                if (!result.items.length)
                  throw new Error("No authorized project is available.");
                setProjects(result.items);
              } catch (reason) {
                clear();
                setError(
                  reason instanceof Error
                    ? reason.message
                    : "Authentication failed.",
                );
              } finally {
                setBusy(false);
              }
            }}
          >
            <label>
              Development token
              <input
                name="token"
                type="password"
                autoComplete="off"
                required
                spellCheck={false}
              />
            </label>
            <button className="button primary" disabled={busy}>
              {busy ? "Authenticating..." : "Authenticate"}
            </button>
          </form>
        ) : (
          <form
            onSubmit={(event) => {
              event.preventDefault();
              const chosen = projects.find((p) => p.id === selected);
              if (!chosen) {
                setError("Select an authorized project.");
                return;
              }
              setWorkspace(chosen);
              setProject(chosen);
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
            <button type="button" className="button secondary" onClick={clear}>
              Use another token
            </button>
          </form>
        )}
      </section>
    </main>
  );
}
