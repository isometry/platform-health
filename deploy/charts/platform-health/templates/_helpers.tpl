{{/*
Expand the name of the chart.
*/}}
{{- define "chart.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
*/}}
{{- define "chart.fullname" -}}
{{-   if default false .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{-   else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{-     if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{-     else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{-     end }}
{{-   end }}
{{- end }}

{{/*
Deployment namespace
*/}}
{{- define "chart.namespace" -}}
{{- default .Release.Namespace .Values.namespace }}
{{- end }}

{{/*
Common labels that should be on every resource
*/}}
{{- define "labels" -}}
app: {{ include "chart.name" . }}
{{ include "selectorLabels" . }}
{{-   if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{-   end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{-   with .Values.commonLabels }}
{{-     range $key, $value := . }}
{{ $key }}: {{ $value | quote }}
{{-     end }}
{{-   end }}
{{- end }}

{{/*
Selector labels for Deployment and Service
*/}}
{{- define "selectorLabels" -}}
app.kubernetes.io/name: {{ include "chart.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Name of the service account to use
*/}}
{{- define "chart.serviceAccountName" -}}
{{-   if .Values.serviceAccount.create }}
{{- default (include "chart.fullname" .) .Values.serviceAccount.name }}
{{-   else }}
{{- default "default" .Values.serviceAccount.name }}
{{-   end }}
{{- end }}

{{/*
Fully-assembled container image reference.

Resolution rules:
  - Repository path:
      image.repository (if set)   -> used verbatim
      else                        -> image.registry + "/" + image.name
  - Tag:
      image.tag non-empty         -> used verbatim
      image.tag == "" (explicit)  -> omitted if image.digest is set, else falls back
      image.tag unset/nil         -> Chart.AppVersion, else "latest"
  - Digest (image.digest):
      when set                    -> appended as "@<digest>"
      combined with a tag renders as "<repo>:<tag>@<digest>" (OCI-valid).
  - Values are concatenated raw and the fully-assembled reference is passed
    through `tpl` once at the end, so individual fields (or the whole thing)
    may contain template expressions (e.g.
    "{{ `{{ .Values.global.oci_registry }}` }}").
*/}}
{{- define "platform-health.image" -}}
{{- $img := .Values.image -}}
{{- $repo := "" -}}
{{- if $img.repository -}}
{{-   $repo = $img.repository -}}
{{- else -}}
{{-   $registry := "" -}}
{{-   if $img.registry -}}{{- $registry = $img.registry | trimSuffix "/" -}}{{- end -}}
{{-   $name := "" -}}
{{-   if $img.name -}}{{- $name = $img.name | trimPrefix "/" -}}{{- end -}}
{{-   if $registry -}}
{{-     $repo = printf "%s/%s" $registry $name -}}
{{-   else -}}
{{-     $repo = $name -}}
{{-   end -}}
{{- end -}}
{{- $digest := default "" $img.digest -}}
{{- $tag := "" -}}
{{- if $img.tag -}}
{{-   $tag = $img.tag -}}
{{- else if kindIs "string" $img.tag -}}
{{-   if not $digest -}}
{{-     $tag = default "latest" .Chart.AppVersion -}}
{{-   end -}}
{{- else -}}
{{-   $tag = default "latest" .Chart.AppVersion -}}
{{- end -}}
{{- $ref := $repo -}}
{{- if $tag -}}{{- $ref = printf "%s:%s" $ref $tag -}}{{- end -}}
{{- if $digest -}}{{- $ref = printf "%s@%s" $ref $digest -}}{{- end -}}
{{- tpl $ref . -}}
{{- end }}
