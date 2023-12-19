
{{- define "k8s-smoke-test.deployment.extraLabels" -}}
app.kubernetes.io/component: deployment
{{- end -}}

{{/*
Common labels
*/}}
{{- define "k8s-smoke-test.deployment.labels" -}}
{{ include "k8s-smoke-test.labels" . }}
{{ include "k8s-smoke-test.deployment.extraLabels" . }}

{{- end }}

{{/*
Selector labels
*/}}
{{- define "k8s-smoke-test.deployment.selectorLabels" -}}
{{ include "k8s-smoke-test.selectorLabels" . }}
{{ include "k8s-smoke-test.deployment.extraLabels" . }}
{{- end }}
