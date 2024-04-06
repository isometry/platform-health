{{/*
Expand the name of the chart.
*/}}
{{- define "platform-health.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
*/}}
{{- define "platform-health.fullname" -}}
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

{{- define "platform-health.rules" -}}
{{- $role := .Values.role | default dict }}
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["get"]
  {{- if $role.enableDeployments }}
  - apiGroups: ["apps"]
    resources: ["deployments", "replicasets", "statefulsets", "daemonsets"]
    verbs: ["get"]
  {{- end }}
  {{- if $role.enableSecrets }}
  - apiGroups: [""]
    resources: ["secrets"]
    verbs: ["get"]
  {{- end }}
  {{- if $role.enableConfigMaps }}
  - apiGroups: [ "" ]
    resources: [ "configmaps" ]
    verbs: [ "get" ]
  {{- end }}
  {{- if $role.enableArgoApplications }}
  - apiGroups: ["argoproj.io"]
    resources: ["applications"]
    verbs: ["get"]
  {{- end }}
  {{- if $role.enableArgoApplicationSets }}
  - apiGroups: ["argoproj.io"]
    resources: ["applicationsets"]
    verbs: ["get"]
  {{- end }}
  {{- if $role.enableJobs }}
  - apiGroups: ["batch"]
    resources: ["jobs", "cronjobs"]
    verbs: ["get"]
  {{- end }}
  {{- if $role.enableCertManager }}
  - apiGroups: ["cert-manager.io"]
    resources: ["clusterissuers", "issuers", "certificates"]
    verbs: ["get"]
  {{- end }}
  {{- if $role.enableNetworking }}
  - apiGroups: ["networking.k8s.io"]
    resources: ["ingressclasses", "ingresses", "networkpolicies"]
    verbs: ["get"]
  - apiGroups: [""]
    resources: ["services"]
    verbs: ["get"]
  {{- end }}
  {{- if $role.enableStorage }}
  - apiGroups: ["storage.k8s.io"]
    resources: ["storageclasses", "volumeattachments", "csinodes", "csidrivers", "persistentvolumeclaims", "persistentvolumes"]
    verbs: ["get"]
  {{- end }}
  {{- if $role.enablePodDisruptionBudgets }}
  - apiGroups: ["policy"]
    resources: ["poddisruptionbudgets", "podsecuritypolicies"]
    verbs: ["get"]
  {{- end }}
{{- end -}}
