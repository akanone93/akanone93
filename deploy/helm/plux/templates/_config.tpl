{{- /*
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0

The plux-server configuration of each role (spec Appendix H), built from the
values. The shape mirrors deploy/compose/plux-*.yaml. Credentials are never
part of it: they arrive as PLUX_* environment variables (plux.dataEnv), which
the server reads in place of the file's values.
*/ -}}

{{- /* The sections the api and worker roles share. Context: root. */ -}}
{{- define "plux.config.data" -}}
database:
  maxConnections: {{ .Values.postgresql.maxConnections }}
  migrateOnStart: {{ .Release.IsInstall }}
objectStorage:
  backend: s3
  bucket: {{ .Values.objectStorage.bucket | quote }}
  pathStyle: {{ .Values.objectStorage.pathStyle }}
  {{- if eq .Values.objectStorage.mode "seaweedfs" }}
  endpoint: {{ printf "http://%s:8333" (include "plux.seaweedName" .) | quote }}
  {{- else }}
  endpoint: {{ .Values.objectStorage.external.endpoint | quote }}
  {{- end }}
  {{- if .Values.objectStorage.region }}
  region: {{ .Values.objectStorage.region | quote }}
  {{- end }}
  {{- if .Values.objectStorage.cdnBaseURL }}
  cdnBaseURL: {{ .Values.objectStorage.cdnBaseURL | quote }}
  {{- else if eq .Values.objectStorage.mode "seaweedfs" }}
  cdnBaseURL: {{ printf "%s/v1/objects" (trimSuffix "/" .Values.server.publicBaseURL) | quote }}
  {{- end }}
cache:
  backend: valkey
signing:
  backend: {{ .Values.signing.backend | quote }}
  {{- if eq .Values.signing.backend "file" }}
  directory: /var/lib/plux/keys
  {{- else }}
  vault:
    address: {{ required "signing.vault.address is required when signing.backend is vault" .Values.signing.vault.address | quote }}
    {{- with .Values.signing.vault.mount }}
    mount: {{ . | quote }}
    {{- end }}
    {{- with .Values.signing.vault.namespace }}
    namespace: {{ . | quote }}
    {{- end }}
    {{- with .Values.signing.vault.wrapKey }}
    wrapKey: {{ . | quote }}
    {{- end }}
  {{- end }}
{{- if .Values.license.existingSecret }}
license:
  file: {{ printf "/etc/plux/license/%s" .Values.license.key | quote }}
{{- end }}
{{- end -}}

{{- /* The identity of a role on the internal network. Context: root, role. */ -}}
{{- define "plux.config.internalTLS" -}}
caFile: /etc/plux/internal/ca.crt
certFile: /etc/plux/internal/tls.crt
keyFile: /etc/plux/internal/tls.key
{{- end -}}

{{- define "plux.config.observability" -}}
observability:
  logFormat: json
  logLevel: {{ .Values.server.logLevel | quote }}
  {{- with .Values.server.otlpEndpoint }}
  otlpEndpoint: {{ . | quote }}
  {{- end }}
{{- end -}}

{{- /* Merges the chart's sections with the user's. Context: root, own, role. */ -}}
{{- define "plux.config.merged" -}}
{{- $role := index .root.Values .role -}}
{{- toYaml (mergeOverwrite (dict) (fromYaml .own) (deepCopy .root.Values.config) (deepCopy $role.config)) -}}
{{- end -}}

{{- define "plux.config.api" -}}
{{- $own := include "plux.config.api.own" . -}}
{{- include "plux.config.merged" (dict "root" . "own" $own "role" "api") -}}
{{- end -}}

{{- define "plux.config.api.own" -}}
server:
  roles: [api]
  listen: ":8080"
  publicBaseURL: {{ required "server.publicBaseURL is required" .Values.server.publicBaseURL | quote }}
  production: {{ .Values.server.production }}
  {{- if include "plux.apiTLS" . }}
  tls:
    certFile: /etc/plux/tls/tls.crt
    keyFile: /etc/plux/tls/tls.key
  {{- else }}
  behindTLSProxy: {{ .Values.server.behindTLSProxy }}
  {{- end }}
  {{- with .Values.server.trustedProxies }}
  trustedProxies: {{ toJson . }}
  {{- end }}
  shutdownGrace: {{ .Values.server.shutdownGrace | quote }}
  {{- if .Values.functions.enabled }}
  internalListen: ":8443"
  internalTLS:
    {{- include "plux.config.internalTLS" . | nindent 4 }}
  {{- end }}
{{ include "plux.config.data" . }}
{{ include "plux.config.observability" . }}
{{- if .Values.functions.enabled }}
functions:
  runnerURL: {{ printf "https://%s-fnrunner:8445" (include "plux.fullname" .) | quote }}
{{- end }}
{{- end -}}

{{- define "plux.config.worker" -}}
{{- $own := include "plux.config.worker.own" . -}}
{{- include "plux.config.merged" (dict "root" . "own" $own "role" "worker") -}}
{{- end -}}

{{- define "plux.config.worker.own" -}}
server:
  roles: [worker]
  publicBaseURL: {{ required "server.publicBaseURL is required" .Values.server.publicBaseURL | quote }}
  production: {{ .Values.server.production }}
  shutdownGrace: {{ .Values.server.shutdownGrace | quote }}
  {{- if and .Values.functions.enabled .Values.builder.enabled }}
  internalTLS:
    {{- include "plux.config.internalTLS" . | nindent 4 }}
  {{- end }}
{{ include "plux.config.data" . }}
{{ include "plux.config.observability" . }}
{{- if and .Values.functions.enabled .Values.builder.enabled }}
builder:
  address: {{ printf "https://%s-builder:8446" (include "plux.fullname" .) | quote }}
{{- end }}
{{- end -}}

{{- /* The runner holds no database, storage or signing credential (L-4). */ -}}
{{- define "plux.config.fnrunner" -}}
{{- $own := include "plux.config.fnrunner.own" . -}}
{{- include "plux.config.merged" (dict "root" . "own" $own "role" "fnrunner") -}}
{{- end -}}

{{- define "plux.config.fnrunner.own" -}}
server:
  roles: [fnrunner]
  production: {{ .Values.server.production }}
  shutdownGrace: {{ .Values.server.shutdownGrace | quote }}
{{ include "plux.config.observability" . }}
fnrunner:
  listen: ":8445"
  internalTLS:
    {{- include "plux.config.internalTLS" . | nindent 4 }}
  apiURL: {{ printf "https://%s-api-internal:8443" (include "plux.fullname" .) | quote }}
  egressAddr: {{ printf "%s-egress:8444" (include "plux.fullname" .) | quote }}
  artifactKeys:
    {{- required "functions.artifactKeys must list the public key of signing.keys.functionArtifact when functions.enabled" .Values.functions.artifactKeys | toYaml | nindent 4 }}
{{- end -}}

{{- /* The only route from the runner to the internet; no credential. */ -}}
{{- define "plux.config.egress" -}}
{{- $own := include "plux.config.egress.own" . -}}
{{- include "plux.config.merged" (dict "root" . "own" $own "role" "egress") -}}
{{- end -}}

{{- define "plux.config.egress.own" -}}
server:
  roles: [egress]
  production: {{ .Values.server.production }}
  shutdownGrace: {{ .Values.server.shutdownGrace | quote }}
{{ include "plux.config.observability" . }}
egress:
  listen: ":8444"
  internalTLS:
    {{- include "plux.config.internalTLS" . | nindent 4 }}
{{- end -}}

{{- /* Compiles function sources in a sandbox; no credential, signs nothing. */ -}}
{{- define "plux.config.builder" -}}
{{- $own := include "plux.config.builder.own" . -}}
{{- include "plux.config.merged" (dict "root" . "own" $own "role" "builder") -}}
{{- end -}}

{{- define "plux.config.builder.own" -}}
server:
  roles: [builder]
  production: {{ .Values.server.production }}
  shutdownGrace: {{ .Values.server.shutdownGrace | quote }}
{{ include "plux.config.observability" . }}
builder:
  listen: ":8446"
  internalTLS:
    {{- include "plux.config.internalTLS" . | nindent 4 }}
  sandbox: namespaces
  sdkDir: /opt/plux/sdk/plux-fn
  wasmOpt: [bun, /opt/binaryen/node_modules/binaryen/bin/wasm-opt]
  workDir: /var/lib/plux/build
  concurrency: 1
  identity: {{ printf "%s-builder" (include "plux.fullname" .) | quote }}
{{- end -}}
