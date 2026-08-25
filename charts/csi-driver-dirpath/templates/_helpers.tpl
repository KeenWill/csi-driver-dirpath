{{- define "csi-driver-dirpath.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "csi-driver-dirpath.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := include "csi-driver-dirpath.name" . }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{- define "csi-driver-dirpath.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" }}
app.kubernetes.io/name: {{ include "csi-driver-dirpath.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "csi-driver-dirpath.selectorLabels" -}}
app.kubernetes.io/name: {{ include "csi-driver-dirpath.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "csi-driver-dirpath.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "csi-driver-dirpath.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- required "serviceAccount.name is required when serviceAccount.create is false" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{- define "csi-driver-dirpath.fenceSecretName" -}}
{{- default (printf "%s-fence" (include "csi-driver-dirpath.fullname" .)) .Values.fence.existingSecret }}
{{- end }}

{{- define "csi-driver-dirpath.image" -}}
{{- $tag := default .Chart.AppVersion .Values.image.tag }}
{{- if .Values.image.digest }}
{{- printf "%s:%s@%s" .Values.image.repository $tag .Values.image.digest }}
{{- else }}
{{- printf "%s:%s" .Values.image.repository $tag }}
{{- end }}
{{- end }}
