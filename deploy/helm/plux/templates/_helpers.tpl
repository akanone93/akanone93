{{- /*
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0

Helpers shared by the chart's templates. A helper that needs a role takes a
dict: root (the top-level context), role, and what else it documents.
*/ -}}

{{- define "plux.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 40 | trimSuffix "-" -}}
{{- end -}}

{{- /* Resource names are "<fullname>-<role>", so the fullname leaves room. */ -}}
{{- define "plux.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 40 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 40 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 40 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "plux.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "plux.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{- /* Selects the pods of one component. Context: root, component. */ -}}
{{- define "plux.selectorLabels" -}}
app.kubernetes.io/name: {{ include "plux.name" .root }}
app.kubernetes.io/instance: {{ .root.Release.Name }}
app.kubernetes.io/component: {{ .component }}
{{- end -}}

{{- define "plux.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .root.Chart.Name .root.Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{ include "plux.selectorLabels" . }}
app.kubernetes.io/version: {{ .root.Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .root.Release.Service }}
app.kubernetes.io/part-of: plux
{{- end -}}

{{- /* The labels of every pod of the release, for NetworkPolicy selectors. */ -}}
{{- define "plux.releaseSelector" -}}
app.kubernetes.io/name: {{ include "plux.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- /* repository[:tag][@digest]. Context: root, image, defaultTag. */ -}}
{{- define "plux.image" -}}
{{- $tag := default .defaultTag .image.tag -}}
{{- if and .image.digest $tag -}}
{{- printf "%s:%s@%s" .image.repository $tag .image.digest -}}
{{- else if .image.digest -}}
{{- printf "%s@%s" .image.repository .image.digest -}}
{{- else -}}
{{- printf "%s:%s" .image.repository $tag -}}
{{- end -}}
{{- end -}}

{{- define "plux.serverImage" -}}
{{- include "plux.image" (dict "image" .Values.image "defaultTag" .Chart.AppVersion) -}}
{{- end -}}

{{- define "plux.builderImage" -}}
{{- include "plux.image" (dict "image" .Values.builderImage "defaultTag" .Chart.AppVersion) -}}
{{- end -}}

{{- /* Names of the dependencies the chart runs or addresses. */ -}}
{{- define "plux.pgName" -}}
{{- default (printf "%s-pg" (include "plux.fullname" .)) .Values.postgresql.cnpg.clusterName -}}
{{- end -}}

{{- define "plux.valkeyName" -}}
{{- printf "%s-valkey" (include "plux.fullname" .) -}}
{{- end -}}

{{- define "plux.seaweedName" -}}
{{- printf "%s-seaweedfs" (include "plux.fullname" .) -}}
{{- end -}}

{{- /* The Secret that holds a role's mutual-TLS identity. Context: root, role. */ -}}
{{- define "plux.tlsSecretName" -}}
{{- if eq .root.Values.internalTLS.mode "existing" -}}
{{- required (printf "internalTLS.existingSecrets.%s is required when internalTLS.mode is existing" .role) (index .root.Values.internalTLS.existingSecrets .role) -}}
{{- else -}}
{{- printf "%s-tls-%s" (include "plux.fullname" .root) .role -}}
{{- end -}}
{{- end -}}

{{- /* Whether the api role terminates TLS itself. */ -}}
{{- define "plux.apiTLS" -}}
{{- if .Values.server.tls.existingSecret -}}true{{- end -}}
{{- end -}}

{{- /* Whether the chart-run Valkey uses TLS (and the pods must trust its CA). */ -}}
{{- define "plux.valkeyTLS" -}}
{{- if and (eq .Values.valkey.mode "sentinel") .Values.valkey.sentinel.tls -}}true{{- end -}}
{{- end -}}

{{- /* The Secret names the data roles read, resolved once. */ -}}
{{- define "plux.s3SecretName" -}}
{{- if eq .Values.objectStorage.mode "seaweedfs" -}}
{{- default (printf "%s-s3" (include "plux.seaweedName" .)) .Values.objectStorage.seaweedfs.existingSecret -}}
{{- else -}}
{{- .Values.objectStorage.external.existingSecret -}}
{{- end -}}
{{- end -}}

{{- define "plux.valkeySecretName" -}}
{{- default (printf "%s-auth" (include "plux.valkeyName" .)) .Values.valkey.sentinel.existingSecret -}}
{{- end -}}

{{- /* The hosts of the Sentinels, "pod.headless.ns.svc.domain:26379,...". */ -}}
{{- define "plux.sentinelHosts" -}}
{{- $name := include "plux.valkeyName" . -}}
{{- $hosts := list -}}
{{- range $i := until (int .Values.valkey.sentinel.replicas) -}}
{{- $hosts = append $hosts (printf "%s-%d.%s-headless.%s.svc.%s:26379" $name $i $name $.Release.Namespace $.Values.clusterDomain) -}}
{{- end -}}
{{- join "," $hosts -}}
{{- end -}}

{{- /*
Environment of the roles that hold data credentials (api, worker, migrate).
Kubernetes expands $(NAME) in a value from variables defined before it, so the
URLs are assembled from Secret keys and no credential appears in the manifest.
Context: root, role.
*/ -}}
{{- define "plux.dataEnv" -}}
{{- $root := .root -}}
{{- if eq $root.Values.postgresql.mode "cnpg" }}
- name: PLUX_PG_USER
  valueFrom:
    secretKeyRef: {name: {{ include "plux.pgName" $root }}-app, key: username}
- name: PLUX_PG_PASSWORD
  valueFrom:
    secretKeyRef: {name: {{ include "plux.pgName" $root }}-app, key: password}
- name: PLUX_DATABASE_URL
  value: "postgres://$(PLUX_PG_USER):$(PLUX_PG_PASSWORD)@{{ include "plux.pgName" $root }}-rw.{{ $root.Release.Namespace }}.svc:5432/{{ $root.Values.postgresql.database }}?sslmode=verify-full&sslrootcert=/etc/plux/postgres/ca.crt"
{{- else }}
- name: PLUX_DATABASE_URL
  valueFrom:
    secretKeyRef:
      name: {{ required "postgresql.external.existingSecret is required when postgresql.mode is external" $root.Values.postgresql.external.existingSecret }}
      key: {{ $root.Values.postgresql.external.key }}
{{- end }}
{{- if eq $root.Values.valkey.mode "sentinel" }}
- name: PLUX_VALKEY_PASSWORD
  valueFrom:
    secretKeyRef: {name: {{ include "plux.valkeySecretName" $root }}, key: password}
- name: PLUX_CACHE_VALKEY_URL
  value: "{{ if $root.Values.valkey.sentinel.tls }}valkeys{{ else }}valkey{{ end }}+sentinel://:$(PLUX_VALKEY_PASSWORD)@{{ include "plux.sentinelHosts" $root }}/{{ $root.Values.valkey.sentinel.masterName }}?sentinelPassword=$(PLUX_VALKEY_PASSWORD)"
{{- else }}
- name: PLUX_CACHE_VALKEY_URL
  valueFrom:
    secretKeyRef:
      name: {{ required "valkey.external.existingSecret is required when valkey.mode is external" $root.Values.valkey.external.existingSecret }}
      key: {{ $root.Values.valkey.external.key }}
{{- end }}
{{- $s3 := include "plux.s3SecretName" $root }}
{{- if $s3 }}
- name: PLUX_OBJECT_STORAGE_ACCESS_KEY_ID
  valueFrom:
    secretKeyRef: {name: {{ $s3 }}, key: {{ if eq $root.Values.objectStorage.mode "external" }}{{ $root.Values.objectStorage.external.accessKeyIDKey }}{{ else }}accessKeyID{{ end }}}
- name: PLUX_OBJECT_STORAGE_SECRET_ACCESS_KEY
  valueFrom:
    secretKeyRef: {name: {{ $s3 }}, key: {{ if eq $root.Values.objectStorage.mode "external" }}{{ $root.Values.objectStorage.external.secretAccessKeyKey }}{{ else }}secretAccessKey{{ end }}}
{{- end }}
{{- if eq $root.Values.signing.backend "vault" }}
- name: PLUX_SIGNING_VAULT_TOKEN
  valueFrom:
    secretKeyRef:
      name: {{ required "signing.vault.tokenSecret.name is required when signing.backend is vault" $root.Values.signing.vault.tokenSecret.name }}
      key: {{ $root.Values.signing.vault.tokenSecret.key }}
{{- end }}
{{- if or (include "plux.valkeyTLS" $root) $root.Values.valkey.external.caSecret }}
- name: SSL_CERT_DIR
  value: /etc/plux/trust
{{- end }}
{{- end -}}

{{- /* Volumes and mounts that go with plux.dataEnv. Context: root. */ -}}
{{- define "plux.dataVolumes" -}}
{{- if eq .Values.postgresql.mode "cnpg" }}
- name: postgres-ca
  secret:
    secretName: {{ include "plux.pgName" . }}-ca
    items: [{key: ca.crt, path: ca.crt}]
{{- end }}
{{- if or (include "plux.valkeyTLS" .) .Values.valkey.external.caSecret }}
- name: trust
  projected:
    sources:
      {{- if include "plux.valkeyTLS" . }}
      - configMap:
          name: {{ include "plux.valkeyName" . }}-ca
          items: [{key: ca.crt, path: valkey-ca.crt}]
      {{- end }}
      {{- if .Values.valkey.external.caSecret }}
      - secret:
          name: {{ .Values.valkey.external.caSecret }}
          items: [{key: {{ .Values.valkey.external.caKey }}, path: valkey-external-ca.crt}]
      {{- end }}
{{- end }}
{{- if eq .Values.signing.backend "file" }}
- name: keys
  {{- if .Values.signing.file.persistence.existingClaim }}
  persistentVolumeClaim: {claimName: {{ .Values.signing.file.persistence.existingClaim }}}
  {{- else }}
  persistentVolumeClaim: {claimName: {{ include "plux.fullname" . }}-keys}
  {{- end }}
{{- end }}
{{- end -}}

{{- /* The enterprise licence file, mounted read-only from an existing Secret into the roles that serve (GOV-030). Context: root. */ -}}
{{- define "plux.licenseVolumes" -}}
{{- if .Values.license.existingSecret }}
- name: license
  secret:
    secretName: {{ .Values.license.existingSecret }}
    items: [{key: {{ .Values.license.key }}, path: {{ .Values.license.key }}}]
{{- end }}
{{- end -}}

{{- /* The server reads its licence path from PLUX_LICENSE_FILE; the
configuration file has no licence section. */ -}}
{{- define "plux.licenseEnv" -}}
{{- if .Values.license.existingSecret }}
- name: PLUX_LICENSE_FILE
  value: {{ printf "/etc/plux/license/%s" .Values.license.key | quote }}
{{- end }}
{{- end -}}

{{- define "plux.licenseMounts" -}}
{{- if .Values.license.existingSecret }}
- {name: license, mountPath: /etc/plux/license, readOnly: true}
{{- end }}
{{- end -}}

{{- define "plux.dataMounts" -}}
{{- if eq .Values.postgresql.mode "cnpg" }}
- {name: postgres-ca, mountPath: /etc/plux/postgres, readOnly: true}
{{- end }}
{{- if or (include "plux.valkeyTLS" .) .Values.valkey.external.caSecret }}
- {name: trust, mountPath: /etc/plux/trust, readOnly: true}
{{- end }}
{{- if eq .Values.signing.backend "file" }}
- {name: keys, mountPath: /var/lib/plux/keys}
{{- end }}
{{- end -}}

{{- /*
Pod and container security: Pod Security "restricted". Context: root, role,
and optionally uid (default 65532) and seccomp (default RuntimeDefault, or a
Localhost profile name).
*/ -}}
{{- define "plux.podSecurityContext" -}}
runAsNonRoot: true
runAsUser: {{ default 65532 .uid }}
runAsGroup: {{ default 65532 .uid }}
fsGroup: {{ default 65532 .uid }}
fsGroupChangePolicy: OnRootMismatch
seccompProfile:
  {{- if .seccomp }}
  type: Localhost
  localhostProfile: {{ .seccomp }}
  {{- else }}
  type: RuntimeDefault
  {{- end }}
{{- end -}}

{{- define "plux.containerSecurityContext" -}}
allowPrivilegeEscalation: false
readOnlyRootFilesystem: true
privileged: false
capabilities:
  drop: [ALL]
{{- end -}}

{{- /*
Default scheduling: spread over zones and over nodes (scheduling.*Spread).
A role's own topologySpreadConstraints or affinity replaces the default.
Context: root, role, v (the role's values).
*/ -}}
{{- define "plux.scheduling" -}}
{{- $labels := include "plux.selectorLabels" (dict "root" .root "component" .role) -}}
{{- if .v.topologySpreadConstraints }}
topologySpreadConstraints:
  {{- toYaml .v.topologySpreadConstraints | nindent 2 }}
{{- else }}
topologySpreadConstraints:
  - maxSkew: 1
    topologyKey: topology.kubernetes.io/zone
    whenUnsatisfiable: {{ .root.Values.scheduling.zoneSpread }}
    labelSelector:
      matchLabels:
        {{- $labels | nindent 8 }}
  - maxSkew: 1
    topologyKey: kubernetes.io/hostname
    whenUnsatisfiable: {{ .root.Values.scheduling.nodeSpread }}
    labelSelector:
      matchLabels:
        {{- $labels | nindent 8 }}
{{- end }}
{{- if .v.affinity }}
affinity:
  {{- toYaml .v.affinity | nindent 2 }}
{{- end }}
{{- with .v.nodeSelector }}
nodeSelector:
  {{- toYaml . | nindent 2 }}
{{- end }}
{{- with .v.tolerations }}
tolerations:
  {{- toYaml . | nindent 2 }}
{{- end }}
{{- end -}}

{{- /* Seconds a pod gets to drain: the server's grace plus the preStop sleep. */ -}}
{{- define "plux.terminationGrace" -}}
{{- add 20 (int (trimSuffix "s" .Values.server.shutdownGrace)) -}}
{{- end -}}

{{- /* A PodDisruptionBudget. Context: root, role, v. */ -}}
{{- define "plux.pdb" -}}
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: {{ include "plux.fullname" .root }}-{{ .role }}
  labels:
    {{- include "plux.labels" (dict "root" .root "component" .role) | nindent 4 }}
spec:
  {{- toYaml .v.podDisruptionBudget | nindent 2 }}
  selector:
    matchLabels:
      {{- include "plux.selectorLabels" (dict "root" .root "component" .role) | nindent 6 }}
{{- end -}}

{{- /* A HorizontalPodAutoscaler on CPU. Context: root, role, v. */ -}}
{{- define "plux.hpa" -}}
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: {{ include "plux.fullname" .root }}-{{ .role }}
  labels:
    {{- include "plux.labels" (dict "root" .root "component" .role) | nindent 4 }}
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: {{ include "plux.fullname" .root }}-{{ .role }}
  minReplicas: {{ .v.autoscaling.minReplicas }}
  maxReplicas: {{ .v.autoscaling.maxReplicas }}
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: {{ .v.autoscaling.targetCPUUtilizationPercentage }}
  behavior:
    scaleDown:
      stabilizationWindowSeconds: 300
{{- end -}}

{{- /* The pod template metadata shared by the role deployments. */ -}}
{{- define "plux.podAnnotations" -}}
{{- /* Annotation values are strings in Kubernetes; quote them so a value
set as a number (--set x.podAnnotations.roll=1) does not break an upgrade. */ -}}
{{- $lines := list }}
{{- range $k, $v := .v.podAnnotations }}
{{- $lines = append $lines (printf "%s: %s" $k ($v | toString | quote)) }}
{{- end }}
{{- join "\n" $lines }}
{{- end -}}

{{- /*
Generated certificate authority and identities, kept across upgrades: a Secret
that exists is reused, so the chart's output is stable and a rotation is a
deliberate act (delete the Secret, upgrade).
*/ -}}

{{- /* Context: root, name (the Secret), cn. Prints {"cert": ..., "key": ...}. */ -}}
{{- define "plux.ca" -}}
{{- $existing := lookup "v1" "Secret" .root.Release.Namespace .name -}}
{{- if and $existing (hasKey $existing.data "ca.crt") (hasKey $existing.data "ca.key") -}}
{{- dict "cert" (index $existing.data "ca.crt" | b64dec) "key" (index $existing.data "ca.key" | b64dec) | toJson -}}
{{- else -}}
{{- $ca := genCA .cn 3650 -}}
{{- dict "cert" $ca.Cert "key" $ca.Key | toJson -}}
{{- end -}}
{{- end -}}

{{- /*
Context: root, name (the Secret), cn, sans, optional ips, ca (the dict plux.ca prints).
Prints {"crt": ..., "key": ...}.
*/ -}}
{{- define "plux.identity" -}}
{{- $existing := lookup "v1" "Secret" .root.Release.Namespace .name -}}
{{- if and $existing (hasKey $existing.data "tls.crt") (hasKey $existing.data "tls.key") -}}
{{- dict "crt" (index $existing.data "tls.crt" | b64dec) "key" (index $existing.data "tls.key" | b64dec) | toJson -}}
{{- else -}}
{{- $ca := buildCustomCert (.ca.cert | b64enc) (.ca.key | b64enc) -}}
{{- $cert := genSignedCert .cn (default (list) .ips) .sans (int .root.Values.internalTLS.validityDays) $ca -}}
{{- dict "crt" $cert.Cert "key" $cert.Key | toJson -}}
{{- end -}}
{{- end -}}

{{- /* The DNS names of a Service. Context: root, svc. */ -}}
{{- define "plux.sans" -}}
{{- $ns := .root.Release.Namespace -}}
{{- $d := .root.Values.clusterDomain -}}
{{- toJson (list .svc (printf "%s.%s" .svc $ns) (printf "%s.%s.svc" .svc $ns) (printf "%s.%s.svc.%s" .svc $ns $d)) -}}
{{- end -}}

{{- /* A random value that is kept across upgrades. Context: root, name, key. */ -}}
{{- define "plux.randomValue" -}}
{{- $existing := lookup "v1" "Secret" .root.Release.Namespace .name -}}
{{- if and $existing (hasKey $existing.data .key) -}}
{{- index $existing.data .key | b64dec -}}
{{- else -}}
{{- randAlphaNum 32 -}}
{{- end -}}
{{- end -}}

{{- /*
A duration of whole seconds, minutes or hours ("120s", "2m", "1h") in seconds.
The values schema admits no other form.
*/ -}}
{{- define "plux.durationSeconds" -}}
{{- $unit := regexFind "[smh]$" . -}}
{{- $n := int (trimSuffix $unit .) -}}
{{- if eq $unit "h" -}}{{ mul $n 3600 }}{{- else if eq $unit "m" -}}{{ mul $n 60 }}{{- else -}}{{ $n }}{{- end -}}
{{- end -}}
