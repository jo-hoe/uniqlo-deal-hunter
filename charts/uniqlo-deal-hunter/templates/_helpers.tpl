{{/*
Expand the name of the chart.
*/}}
{{- define "uniqlo-deal-hunter.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "uniqlo-deal-hunter.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Chart label.
*/}}
{{- define "uniqlo-deal-hunter.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Standard labels attached to every resource.
*/}}
{{- define "uniqlo-deal-hunter.labels" -}}
helm.sh/chart: {{ include "uniqlo-deal-hunter.chart" . }}
{{ include "uniqlo-deal-hunter.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{/*
Selector labels for CronJob pod template.
*/}}
{{- define "uniqlo-deal-hunter.selectorLabels" -}}
app.kubernetes.io/name: {{ include "uniqlo-deal-hunter.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/*
Resolved ServiceAccount name.
*/}}
{{- define "uniqlo-deal-hunter.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
    {{ default (include "uniqlo-deal-hunter.fullname" .) .Values.serviceAccount.name }}
{{- else -}}
    {{ default "default" .Values.serviceAccount.name }}
{{- end -}}
{{- end -}}

{{/*
Effective PVC name (existing or generated).
*/}}
{{- define "uniqlo-deal-hunter.pvcName" -}}
{{- if .Values.persistence.existingClaim -}}
{{ .Values.persistence.existingClaim }}
{{- else -}}
{{ include "uniqlo-deal-hunter.fullname" . }}-state
{{- end -}}
{{- end -}}
