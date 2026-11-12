{{- define "sathonate.name" -}}
{{- printf "%s-sathonate" .Release.Name | trunc 44 | trimSuffix "-" -}}
{{- end -}}
{{- define "sathonate.validate" -}}
{{- range $image := list .Values.apiImage .Values.consoleImage -}}
{{- if not (regexMatch `^[a-zA-Z0-9][a-zA-Z0-9._:/-]*@sha256:[a-f0-9]{64}$` $image) -}}
{{- fail "apiImage and consoleImage require immutable OCI image digests" -}}
{{- end -}}
{{- end -}}
{{- if or (gt (len .Values.publicHost) 253) (not (regexMatch `^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)+$` .Values.publicHost)) -}}
{{- fail "publicHost must be an explicit DNS hostname" -}}
{{- end -}}
{{- if not (and .Values.postgres.existingSecret .Values.postgres.dsnKey .Values.postgres.caKey) -}}
{{- fail "a customer PostgreSQL Secret with DSN and CA keys is required" -}}
{{- end -}}
{{- if not (and (regexMatch `^https://[^/]+/oauth2/[^/?#]+$` .Values.okta.issuer) .Values.okta.clientId .Values.okta.audience (ne .Values.okta.clientId .Values.okta.audience)) -}}
{{- fail "custom HTTPS Okta issuer, SPA client ID and distinct API audience are required" -}}
{{- end -}}
{{- if and .Values.customerPilot.existingTargetInventoryConfigMap (not .Values.customerPilot.enabled) -}}
{{- fail "customer target inventory requires the enabled customer pilot" -}}
{{- end -}}
{{- if and .Values.ingress.enabled (not (and .Values.ingress.className .Values.ingress.tlsSecret)) -}}
{{- fail "ingress requires a class and existing TLS Secret" -}}
{{- end -}}
{{- if not (and .Values.traffic.namespace .Values.traffic.podLabels) -}}
{{- fail "API and console require narrowly selected traffic sources" -}}
{{- end -}}
{{- if and .Values.monitoring.enabled (not (and .Values.monitoring.namespace .Values.monitoring.podLabels)) -}}
{{- fail "monitoring requires narrowly selected scraper sources" -}}
{{- end -}}
{{- end -}}
