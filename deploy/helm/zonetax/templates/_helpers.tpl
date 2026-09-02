{{- define "zonetax.name" -}}
zonetax
{{- end -}}

{{- define "zonetax.fullname" -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "zonetax.labels" -}}
app.kubernetes.io/name: {{ include "zonetax.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "zonetax.selectorLabels" -}}
app.kubernetes.io/name: {{ include "zonetax.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "zonetax.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{ default (include "zonetax.fullname" .) .Values.serviceAccount.name }}
{{- else -}}
{{ default "default" .Values.serviceAccount.name }}
{{- end -}}
{{- end -}}
