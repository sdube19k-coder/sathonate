# Fabric SMP - step-one design standard

Updated 2026-10-21. The screenshot embedded on PDF page 6 (step 1, 1536x1024) defines the shared shell. The former dark navigation rail and horizontal step strip were a mismatch and have been replaced.

## Shared shell

- White 60px header with Sathonate/Fabric SMP branding and workspace controls.
- Light 318px vertical step rail at reference desktop width, numbered circles, connecting lines, descriptions and pale-blue active selection.
- One white bordered content panel beginning at x=329, y=79 at 1536px width. Every step places its heading, fields and footer actions inside this panel.
- Four provider cards, blue selected outlines, a three-column authentication row and two-column AWS details ordered like the reference.
- No external summary rail. Review resources, validation and cost assumptions remain inside the review panel, expanded by default.
- The same shell for configuration, review, real progress and the verified dashboard. Mobile uses a collapsible step drawer and reflowed cards with no horizontal overflow.

SmpApp.tsx owns the shell; SmpScreens.tsx handles configuration and review; SmpRuntime.tsx handles real progress and dashboard; smp-model.ts serializes intent; reference-design.css applies the reference geometry and typography over the component styles in smp.css. LaunchReadiness.tsx is the new platform setup screen; the original saved-design detail view retains its separate revision-bound target checks.

## Functionality preserved

React and Go remain separate repositories. Desired topology and non-secret cloud intent are saved through the revision-bound API in PostgreSQL. Session storage resumes the current tab and selected operation; server state remains authoritative. No provider credentials, private keys or kubeconfig tokens are collected in browser forms.

Local Kubernetes accurately labels kind, PVC storage and internal ClusterIP services. Its real Bevel worker enrolls CA identities, creates three Raft orderers, two peers and a channel, then verifies Go chaincode writes/queries. Docker Compose remains a separate profile. Actual job events drive progress; neither saved drafts nor mock success become a verified network.

Cloud Launch Readiness distinguishes missing identifiers, configured values, read-only access evidence and release qualification. AWS/EKS and Okta are uncreated. The local setup API never provisions paid resources and never uses ambient credentials/context when explicit bindings are missing. The cloud execution adapter still needs implementation and production qualification; configuration alone cannot enable it.

## Visual and functional verification

reference-design.spec.ts checks header, vertical rail and content-panel positions and sizes on steps 1-4, four provider cards, six steps, mobile width, readiness reporting and configuration-template download. local-kubernetes.spec.ts captures all six steps and checks the same geometry while deploying and verifying a real network, retrying its interrupted worker and checking degraded-peer recovery. Screenshots live under ignored test-results/. Retained reference/comparison images are in E:/Sathonate/.cache/design-reference/ outside source control.

Intentional differences: Sathonate branding; six steps rather than the screenshot's seven menu entries; no collection of access keys or session tokens; GCP disabled; truthful setup notices; additional workspace navigation. These preserve the reference's shared light-shell layout rather than copying unsafe or unsupported functionality.
