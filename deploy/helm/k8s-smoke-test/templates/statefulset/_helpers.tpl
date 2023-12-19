
{{- define "k8s-smoke-test.statefulset.extraLabels" -}}
app.kubernetes.io/component: statefulset
{{- end -}}

{{/*
Common labels
*/}}
{{- define "k8s-smoke-test.statefulset.labels" -}}
{{ include "k8s-smoke-test.labels" . }}
{{ include "k8s-smoke-test.statefulset.extraLabels" . }}

{{- end }}

{{/*
Selector labels
*/}}
{{- define "k8s-smoke-test.statefulset.selectorLabels" -}}
{{ include "k8s-smoke-test.selectorLabels" . }}
{{ include "k8s-smoke-test.statefulset.extraLabels" . }}
{{- end }}
