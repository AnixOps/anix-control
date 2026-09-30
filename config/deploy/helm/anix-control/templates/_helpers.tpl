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

{{/* Environment shared by the migrate init container and the server. */}}
{{- define "anix-control.env" -}}
{{- range $key, $variable := .Values.secrets.files }}
- name: {{ printf "%s_FILE" $variable }}
  value: {{ printf "/run/secrets/anix-control/%s" $key | quote }}
{{- end }}
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
