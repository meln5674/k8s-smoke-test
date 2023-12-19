
{{- define "k8s-smoke-test.job.extraLabels" -}}
app.kubernetes.io/component: job
{{- end -}}

{{/*
Common labels
*/}}
{{- define "k8s-smoke-test.job.labels" -}}
{{ include "k8s-smoke-test.labels" . }}
{{ include "k8s-smoke-test.job.extraLabels" . }}

{{- end }}
