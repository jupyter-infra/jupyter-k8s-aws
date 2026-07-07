{{/*
Expand the name of the chart.
*/}}
{{- define "jupyter-k8s-aws-hyperpod.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "jupyter-k8s-aws-hyperpod.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "jupyter-k8s-aws-hyperpod.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "jupyter-k8s-aws-hyperpod.labels" -}}
helm.sh/chart: {{ include "jupyter-k8s-aws-hyperpod.chart" . }}
{{ include "jupyter-k8s-aws-hyperpod.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "jupyter-k8s-aws-hyperpod.selectorLabels" -}}
app.kubernetes.io/name: {{ include "jupyter-k8s-aws-hyperpod.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Resolve the effective list of remote access mechanisms.
Returns the JSON-encoded list of {type, configuration} entries.

If accessMechanisms is set, it is used verbatim. Otherwise, for backwards
compatibility, legacy flat SSM config (remoteAccess.ssmManagedNodeRole) is
shimmed into a single ssm mechanism. Returns "[]" when nothing is configured.

Usage: {{- $mechs := include "remoteAccess.mechanisms" . | fromJsonArray -}}
*/}}
{{- define "remoteAccess.mechanisms" -}}
{{- if not .Values.remoteAccess.enabled -}}
[]
{{- else if .Values.remoteAccess.accessMechanisms -}}
{{- toJson .Values.remoteAccess.accessMechanisms -}}
{{- else if .Values.remoteAccess.ssmManagedNodeRole -}}
{{- list (dict "type" "ssm" "configuration" (dict "ssmManagedNodeRole" .Values.remoteAccess.ssmManagedNodeRole)) | toJson -}}
{{- else -}}
[]
{{- end -}}
{{- end -}}

{{/*
Return "true" if a mechanism of the given type is configured.
Usage: {{- if eq (include "remoteAccess.hasMechanism" (list . "directSSH")) "true" }}
*/}}
{{- define "remoteAccess.hasMechanism" -}}
{{- $ctx := index . 0 -}}
{{- $type := index . 1 -}}
{{- $found := "" -}}
{{- range (include "remoteAccess.mechanisms" $ctx | fromJsonArray) -}}
{{- if eq .type $type -}}{{- $found = "true" -}}{{- end -}}
{{- end -}}
{{- $found -}}
{{- end -}}

{{/*
Return the configuration block (as JSON) for the mechanism of the given type,
or "{}" if not present.
Usage: {{- $cfg := include "remoteAccess.mechanismConfig" (list . "directSSH") | fromJson -}}
*/}}
{{- define "remoteAccess.mechanismConfig" -}}
{{- $ctx := index . 0 -}}
{{- $type := index . 1 -}}
{{- $cfg := dict -}}
{{- range (include "remoteAccess.mechanisms" $ctx | fromJsonArray) -}}
{{- if eq .type $type -}}{{- $cfg = (.configuration | default dict) -}}{{- end -}}
{{- end -}}
{{- $cfg | toJson -}}
{{- end -}}

{{/*
Convert rotation interval to cron schedule.
Supports common durations: "Xm" (minutes), "Xh" (hours).
Examples: "5m" converts to every 5 minutes, "1h" converts to every hour.
*/}}
{{- define "hyperpod.rotationCronSchedule" -}}
{{- $interval := .Values.clusterWebUI.rotator.rotationInterval -}}
{{- if hasSuffix "m" $interval -}}
  {{- $minutes := trimSuffix "m" $interval | int -}}
  {{- if eq $minutes 60 -}}
0 * * * *
  {{- else if le $minutes 59 -}}
*/{{ $minutes }} * * * *
  {{- else -}}
  {{- fail (printf "Invalid rotation interval: %s (minutes must be <= 59)" $interval) -}}
  {{- end -}}
{{- else if hasSuffix "h" $interval -}}
  {{- $hours := trimSuffix "h" $interval | int -}}
  {{- if eq $hours 1 -}}
0 * * * *
  {{- else if le $hours 23 -}}
0 */{{ $hours }} * * *
  {{- else -}}
  {{- fail (printf "Invalid rotation interval: %s (hours must be <= 23)" $interval) -}}
  {{- end -}}
{{- else -}}
  {{- fail (printf "Unsupported rotation interval format: %s (use Xm for minutes or Xh for hours)" $interval) -}}
{{- end -}}
{{- end -}}
