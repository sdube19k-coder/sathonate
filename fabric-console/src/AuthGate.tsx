import { useEffect, useState, type ReactNode } from "react";
import { OktaAuth, toRelativeUrl, type AuthState } from "@okta/okta-auth-js";
import { Boxes, LoaderCircle, LockKeyhole, LogOut } from "lucide-react";
import { setTokenProvider } from "./api";
import LocalSignIn from "./LocalSignIn";
import ProjectGate from "./ProjectGate";
import { localAuthAllowed, setWorkspace } from "./workspace";

type AuthConfig = {
  mode: "okta" | "local-development" | "local-token";
  issuer?: string;
  clientId?: string;
  audience?: string;
  requiredACR?: string;
};
let activeClient: OktaAuth | null = null;
export function hasOktaDeploymentScope() {
  return (
    activeClient?.authStateManager
      .getAuthState()
      ?.accessToken?.scopes.includes("fabric.write") ?? false
  );
}
export async function authorizeCustomerChanges() {
  if (!activeClient) throw new Error("An Okta session is required.");
  await activeClient.signInWithRedirect({
    scopes: ["openid", "profile", "email", "fabric.read", "fabric.write"],
    originalUri: window.location.origin + "/#/customer",
  });
}
export async function authorizeEKSDeployment() {
  if (!activeClient) throw new Error("An Okta session is required.");
  await activeClient.signInWithRedirect({
    scopes: ["openid", "profile", "email", "fabric.read", "fabric.write"],
    originalUri: window.location.origin + "/#/create/4",
  });
}
export function SignOut() {
  const [busy, setBusy] = useState(false);
  return activeClient ? (
    <button
      className="smp-icon-btn"
      aria-label="Sign out of Okta"
      disabled={busy}
      onClick={async () => {
        const client = activeClient;
        if (!client || busy) return;
        setBusy(true);
        setTokenProvider(null);
        setWorkspace(null);
        try {
          const hasIDToken = Boolean(client.getIdToken());
          await client.signOut({
            postLogoutRedirectUri: window.location.origin,
            clearTokensBeforeRedirect: true,
          });
          // The SDK returns without navigation when its session API fallback is used.
          if (!hasIDToken) window.location.replace(window.location.origin);
        } catch {
          client.tokenManager.clear();
          window.dispatchEvent(new Event("okta-logout-error"));
        }
      }}
    >
      <LogOut size={17} />
    </button>
  ) : null;
}

export default function AuthGate({ children }: { children: ReactNode }) {
  const [config, setConfig] = useState<AuthConfig | null>(null);
  const [state, setState] = useState<AuthState | null>(null);
  const [error, setError] = useState("");
  const [retry, setRetry] = useState(0);
  useEffect(() => {
    const failed = () => {
      setState(null);
      setError(
        "Local credentials cleared. Okta session logout could not be verified. Check Trusted Origins and browser cookie policy.",
      );
    };
    window.addEventListener("okta-logout-error", failed);
    return () => window.removeEventListener("okta-logout-error", failed);
  }, []);
  useEffect(() => {
    let disposed = false;
    let client: OktaAuth | undefined;
    const abort = new AbortController();
    setError("");
    setConfig(null);
    setState(null);
    const run = async () => {
      const response = await fetch("/api/v1/auth/config", {
        signal: abort.signal,
      });
      if (!response.ok)
        throw new Error(
          "Cannot load authentication settings from the control plane.",
        );
      const configuration: AuthConfig = await response.json();
      if (disposed) return;
      setConfig(configuration);
      if (configuration.mode === "local-token") return;
      if (configuration.mode === "local-development") {
        setTokenProvider(null);
        return;
      }
      if (
        configuration.mode !== "okta" ||
        !configuration.issuer ||
        !configuration.clientId
      )
        throw new Error("Okta sign-in is not fully configured.");
      client = new OktaAuth({
        issuer: configuration.issuer,
        clientId: configuration.clientId,
        redirectUri: window.location.origin + "/login/callback",
        scopes: ["openid", "profile", "email", "fabric.read"],
        pkce: true,
        ...(configuration.requiredACR
          ? { acrValues: configuration.requiredACR }
          : {}),
        tokenManager: { storage: "memory" },
        storageManager: { transaction: { storageType: "sessionStorage" } },
        restoreOriginalUri: async (_client, originalUri) => {
          window.history.replaceState(
            {},
            "",
            toRelativeUrl(originalUri ?? "/", window.location.origin),
          );
        },
      });
      activeClient = client;
      setTokenProvider(() => client?.getAccessToken());
      client.authStateManager.subscribe((next) => {
        if (!disposed) setState(next);
      });
      if (client.isLoginRedirect()) await client.handleRedirect();
      if (disposed) return;
      await client.start();
      await client.authStateManager.updateAuthState();
      if (!disposed) setState(client.authStateManager.getAuthState());
    };
    void run().catch((reason) => {
      if (!disposed)
        setError(
          reason instanceof Error
            ? reason.message
            : "Sign-in could not be initialized.",
        );
    });
    return () => {
      disposed = true;
      abort.abort();
      if (activeClient === client) activeClient = null;
      setTokenProvider(null);
      void client?.stop();
    };
  }, [retry]);
  if (config?.mode === "local-token") {
    if (!localAuthAllowed(import.meta.env))
      return (
        <main className="auth-screen">
          <div role="alert" className="auth-card">
            Local token authentication is disabled. Explicit local-development
            configuration is required.
          </div>
        </main>
      );
    return <LocalSignIn>{children}</LocalSignIn>;
  }
  if (config?.mode === "local-development")
    return (
      <main className="auth-screen">
        <div role="alert" className="auth-card">
          Fixed developer authentication is disabled in this console. Configure
          explicit local bearer authentication.
        </div>
      </main>
    );
  if (state?.isAuthenticated && !error)
    return <ProjectGate signOut={<SignOut />}>{children}</ProjectGate>;
  return (
    <main className="auth-screen">
      <div className="auth-card">
        <span className="brand-mark">
          <Boxes size={29} />
        </span>
        <div className="eyebrow">SATHONATE FABRIC SMP</div>
        <h1>
          Your organization.
          <br />
          Your network.
        </h1>
        <p>
          Sign in with your company’s Okta account to access its cloud network
          workspace.
        </p>
        {error ? (
          <div className="error-box" role="alert">
            {error}
          </div>
        ) : !config ? (
          <div className="auth-loading">
            <LoaderCircle className="spin" size={19} />
            Connecting to the control plane…
          </div>
        ) : (
          <button
            className="button primary"
            onClick={() =>
              void activeClient
                ?.signInWithRedirect({ originalUri: window.location.href })
                .catch((reason) =>
                  setError(
                    reason instanceof Error
                      ? reason.message
                      : "Sign-in failed.",
                  ),
                )
            }
          >
            <LockKeyhole size={17} />
            Continue with Okta
          </button>
        )}
        {error && (
          <button
            className="button secondary"
            onClick={() => setRetry((value) => value + 1)}
          >
            Retry connection
          </button>
        )}
        <small>
          Authorization Code + PKCE · No passwords handled by Sathonate
        </small>
      </div>
    </main>
  );
}
