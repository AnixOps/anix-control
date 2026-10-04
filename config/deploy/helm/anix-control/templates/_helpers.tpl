{{- define "anix-control.name" -}}
{{- .Chart.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "anix-control.fullname" -}}
{{- if contains .Chart.Name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name .Chart.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{- define "anix-control.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" }}
{{ include "anix-control.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "anix-control.selectorLabels" -}}
app.kubernetes.io/name: {{ include "anix-control.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/*
Network module labels. They must not match the Control selector, which would
put module pods behind the Control Service.
*/}}
{{- define "anix-control.moduleSelectorLabels" -}}
app.kubernetes.io/name: anix-module
app.kubernetes.io/instance: {{ .root.Release.Name }}
app.kubernetes.io/component: {{ .id }}
{{- end -}}

{{- define "anix-control.moduleLabels" -}}
helm.sh/chart: {{ printf "%s-%s" .root.Chart.Name .root.Chart.Version | replace "+" "_" }}
{{ include "anix-control.moduleSelectorLabels" . }}
app.kubernetes.io/part-of: {{ include "anix-control.name" .root }}
app.kubernetes.io/managed-by: {{ .root.Release.Service }}
{{- end -}}

{{- define "anix-control.image" -}}
{{- if .Values.image.digest -}}
{{- printf "%s@%s" .Values.image.repository .Values.image.digest -}}
{{- else -}}
{{- printf "%s:%s" .Values.image.repository (default .Chart.AppVersion .Values.image.tag) -}}
{{- end -}}
{{- end -}}

{{- define "anix-control.secretName" -}}
{{- default (printf "%s-secrets" (include "anix-control.fullname" .)) .Values.secrets.existingSecret -}}
{{- end -}}

{{- define "anix-control.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "anix-control.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{/* "true" when secrets.files maps a key to the CA key variable. */}}
{{- define "anix-control.caKekMapped" -}}
{{- $mapped := false }}
{{- range $key, $variable := .Values.secrets.files }}
{{- if eq $variable "ANIX_CONTROL_MODULE_RUNTIME_CA_KEK" }}{{ $mapped = true }}{{ end }}
{{- end }}
{{- $mapped -}}
{{- end -}}

{{/* "true" when the CA key comes from caKek (existing or chart Secret). */}}
{{- define "anix-control.caKekEnabled" -}}
{{- if and (ne (include "anix-control.caKekMapped" .) "true") (or .Values.caKek.existingSecret .Values.caKek.generate) -}}
true
{{- else -}}
false
{{- end -}}
{{- end -}}

{{- define "anix-control.caKekSecretName" -}}
{{- default (printf "%s-ca-kek" (include "anix-control.fullname" .)) .Values.caKek.existingSecret -}}
{{- end -}}

{{/* Environment shared by the migrate init container and the server. */}}
{{- define "anix-control.env" -}}
{{- range $key, $variable := .Values.secrets.files }}
- name: {{ printf "%s_FILE" $variable }}
  value: {{ printf "/run/secrets/anix-control/%s" $key | quote }}
{{- end }}
{{- if eq (include "anix-control.caKekEnabled" .) "true" }}
- name: ANIX_CONTROL_MODULE_RUNTIME_CA_KEK_FILE
  value: /run/secrets/anix-control-ca-kek/ca_kek
{{- end }}
{{- if .Values.grpc.enabled }}
- name: ANIX_CONTROL_GRPC_ENABLED
  value: "true"
{{- with .Values.grpc.tls.secretName }}
- name: ANIX_CONTROL_GRPC_TLS_CERT_FILE
  value: /run/secrets/anix-control-grpc-tls/tls.crt
- name: ANIX_CONTROL_GRPC_TLS_KEY_FILE
  value: /run/secrets/anix-control-grpc-tls/tls.key
{{- end }}
{{- end }}
{{- if .Values.moduleRuntime.enabled }}
- name: ANIX_CONTROL_MODULE_RUNTIME_ENABLED
  value: "true"
- name: ANIX_CONTROL_MODULE_RUNTIME_LISTEN
  value: {{ printf ":%d" (int .Values.moduleRuntime.port) | quote }}
- name: ANIX_CONTROL_MODULE_RUNTIME_CLUSTER
  value: {{ .Values.moduleRuntime.cluster | quote }}
- name: ANIX_CONTROL_MODULE_RUNTIME_PKI
  value: {{ .Values.moduleRuntime.pki | quote }}
{{- with .Values.moduleRuntime.databaseHost }}
- name: ANIX_CONTROL_MODULE_RUNTIME_DATABASE_HOST
  value: {{ . | quote }}
{{- end }}
{{- end }}
{{- end -}}

{{/* Fail early when the module runtime lacks its CA key. */}}
{{- define "anix-control.validateModuleRuntime" -}}
{{- if and .Values.moduleRuntime.enabled (eq .Values.moduleRuntime.pki "builtin") }}
{{- if and (ne (include "anix-control.caKekMapped" .) "true") (ne (include "anix-control.caKekEnabled" .) "true") }}
{{- fail "moduleRuntime.enabled with pki=builtin needs the CA key: caKek.generate, caKek.existingSecret, or secrets.files.<key>: ANIX_CONTROL_MODULE_RUNTIME_CA_KEK" }}
{{- end }}
{{- end }}
{{- end -}}

{{- define "anix-control.moduleImage" -}}
{{- $image := .image -}}
{{- if $image.digest -}}
{{- printf "%s@%s" $image.repository $image.digest -}}
{{- else -}}
{{- printf "%s:%s" $image.repository (default .appVersion $image.tag) -}}
{{- end -}}
{{- end -}}

{{- define "anix-control.envFrom" -}}
- configMapRef:
    name: {{ include "anix-control.fullname" . }}
{{- end -}}

{{- define "anix-control.volumeMounts" -}}
- name: tmp
  mountPath: /tmp
- name: secrets
  mountPath: /run/secrets/anix-control
  readOnly: true
{{- if eq (include "anix-control.caKekEnabled" .) "true" }}
- name: ca-kek
  mountPath: /run/secrets/anix-control-ca-kek
  readOnly: true
{{- end }}
{{- if and .Values.grpc.enabled .Values.grpc.tls.secretName }}
- name: grpc-tls
  mountPath: /run/secrets/anix-control-grpc-tls
  readOnly: true
{{- end }}
{{- if .Values.ansible.existingSecret }}
- name: ansible
  mountPath: /app/config/deploy/ansible/inventory.ini
  subPath: inventory.ini
  readOnly: true
- name: ansible
  mountPath: /home/anixops/.ssh
  readOnly: true
{{- end }}
{{- end -}}
