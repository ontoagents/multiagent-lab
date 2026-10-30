{{- define "ontoagents.name" -}}
{{- printf "%s-%s" .Release.Name .component -}}
{{- end -}}

{{- define "ontoagents.labels" -}}
app.kubernetes.io/name: ontoagents
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: {{ .component }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "ontoagents.fullimage" -}}
{{- if .image.registry -}}
{{- printf "%s/%s:%s" .image.registry .image.name .image.tag -}}
{{- else -}}
{{- printf "%s:%s" .image.name .image.tag -}}
{{- end -}}
{{- end -}}
