{{- define "keldock.labels" -}}
app.kubernetes.io/part-of: keldock
app.kubernetes.io/instance: {{ .Release.Name | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service | quote }}
{{- end -}}

{{- define "keldock.runtimeSecurity" -}}
runAsNonRoot: true
runAsUser: 65532
runAsGroup: 65532
fsGroup: 65532
fsGroupChangePolicy: OnRootMismatch
seccompProfile: {type: RuntimeDefault}
{{- end -}}

{{- define "keldock.containerSecurity" -}}
allowPrivilegeEscalation: false
readOnlyRootFilesystem: true
capabilities: {drop: [ALL]}
{{- end -}}

{{- define "keldock.assert" -}}
{{- if not (regexMatch "^[a-z0-9][a-z0-9./:_-]*@sha256:[a-f0-9]{64}$" .Values.images.runtime) -}}
{{- fail "images.runtime requires the reviewed immutable release digest" -}}
{{- end -}}
{{- if not (regexMatch "^[a-z0-9][a-z0-9./:_-]*@sha256:[a-f0-9]{64}$" .Values.images.console) -}}
{{- fail "images.console requires the reviewed immutable release digest" -}}
{{- end -}}
{{- required "installation.existingSecret is required; generate private inputs first" .Values.installation.existingSecret -}}
{{- if not .Values.storageClass -}}{{- fail "an explicit tested storageClass is required" -}}{{- end -}}
{{- if and .Values.ingress.enabled (or (not .Values.ingress.host) (not .Values.ingress.tlsSecret)) -}}{{- fail "ingress requires an exact hostname and existing TLS Secret" -}}{{- end -}}
{{- end -}}

{{- define "keldock.trustMount" -}}
{{- if .Values.extraTrustSecret }}
- {name: trust, mountPath: /trust, readOnly: true}
{{- end }}
{{- end -}}

{{- define "keldock.trustEnv" -}}
{{- if .Values.extraTrustSecret }}
- {name: SSL_CERT_FILE, value: /trust/ca.crt}
{{- end }}
{{- end -}}

{{- define "keldock.trustVolume" -}}
{{- if .Values.extraTrustSecret }}
- name: trust
  secret: {secretName: {{ .Values.extraTrustSecret | quote }}, defaultMode: 292}
{{- end }}
{{- end -}}
