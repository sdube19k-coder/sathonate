import React, { lazy, Suspense } from "react";
import ReactDOM from "react-dom/client";
import App from "./App";
import AuthGate from "./AuthGate";
import "@fontsource-variable/dm-sans";
import "@fontsource-variable/manrope";
import "./foundation.css";

const AuthAcceptance =
  import.meta.env.DEV && import.meta.env.VITE_OIDC_ACCEPTANCE === "true"
    ? lazy(() => import("../tools/AuthAcceptance"))
    : null;
function Application() {
  return AuthAcceptance &&
    new URLSearchParams(window.location.search).has("auth-acceptance") ? (
    <Suspense fallback={<p>Loading development acceptance...</p>}>
      <AuthAcceptance />
    </Suspense>
  ) : (
    <App />
  );
}
ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <AuthGate>
      <Application />
    </AuthGate>
  </React.StrictMode>,
);
