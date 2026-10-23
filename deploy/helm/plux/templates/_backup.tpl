{{- /*
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0

Helpers for backups and restores (DEP-010): where the CloudNativePG cluster
archives its WAL and base backups, and which credential it uses. The same
store is the source of a restore, so backup and restore read one set of values
(postgresql.cnpg.backup.*).
*/ -}}

{{- /* The S3 endpoint of the backup store: its own, else the object storage's. */ -}}
{{- define "plux.backup.endpoint" -}}
{{- $b := .Values.postgresql.cnpg.backup -}}
{{- if $b.endpoint -}}
{{- $b.endpoint -}}
{{- else if eq .Values.objectStorage.mode "seaweedfs" -}}
{{- printf "http://%s:8333" (include "plux.seaweedName" .) -}}
{{- else -}}
{{- required "postgresql.cnpg.backup.endpoint (or objectStorage.external.endpoint) is required to back up PostgreSQL" .Values.objectStorage.external.endpoint -}}
{{- end -}}
{{- end -}}

{{- /* The bucket of the backup store. */ -}}
{{- define "plux.backup.bucket" -}}
{{- default .Values.objectStorage.bucket .Values.postgresql.cnpg.backup.bucket -}}
{{- end -}}

{{- /* The Secret that holds the backup store's credential; empty means the pod's ambient credential. */ -}}
{{- define "plux.backup.secret" -}}
{{- default (include "plux.s3SecretName" .) .Values.postgresql.cnpg.backup.existingSecret -}}
{{- end -}}

{{- /* The key names in that Secret. Prints {"id": ..., "secret": ...}. */ -}}
{{- define "plux.backup.secretKeys" -}}
{{- $b := .Values.postgresql.cnpg.backup -}}
{{- if $b.existingSecret -}}
{{- dict "id" $b.accessKeyIDKey "secret" $b.secretAccessKeyKey | toJson -}}
{{- else if eq .Values.objectStorage.mode "external" -}}
{{- dict "id" .Values.objectStorage.external.accessKeyIDKey "secret" .Values.objectStorage.external.secretAccessKeyKey | toJson -}}
{{- else -}}
{{- dict "id" "accessKeyID" "secret" "secretAccessKey" | toJson -}}
{{- end -}}
{{- end -}}

{{- /* The region the backup store signs for. */ -}}
{{- define "plux.backup.region" -}}
{{- default (default "us-east-1" .Values.objectStorage.region) .Values.postgresql.cnpg.backup.region -}}
{{- end -}}

{{- /* The cluster's folder in the store: the server name Barman keeps WAL and backups under. */ -}}
{{- define "plux.backup.serverName" -}}
{{- default (include "plux.pgName" .) .Values.postgresql.cnpg.backup.serverName -}}
{{- end -}}

{{- /*
The barmanObjectStore of the Cluster spec as JSON. Context: root, serverName
(the folder this object store entry reads or writes), archive (whether it also
carries the WAL and data options of an archiving cluster).
*/ -}}
{{- define "plux.backup.store" -}}
{{- $root := .root -}}
{{- $b := $root.Values.postgresql.cnpg.backup -}}
{{- $secret := include "plux.backup.secret" $root -}}
{{- $creds := dict -}}
{{- if $secret -}}
{{- $keys := include "plux.backup.secretKeys" $root | fromJson -}}
{{- $creds = dict "accessKeyId" (dict "name" $secret "key" $keys.id) "secretAccessKey" (dict "name" $secret "key" $keys.secret) -}}
{{- else -}}
{{- $creds = dict "inheritFromIAMRole" true -}}
{{- end -}}
{{- $store := dict
  "destinationPath" (printf "s3://%s/%s" (include "plux.backup.bucket" $root) (trimAll "/" $b.path))
  "endpointURL" (include "plux.backup.endpoint" $root)
  "serverName" .serverName
  "s3Credentials" $creds
-}}
{{- if $b.caSecret -}}
{{- $_ := set $store "endpointCA" (dict "name" $b.caSecret "key" $b.caKey) -}}
{{- end -}}
{{- if .archive -}}
{{- $_ := set $store "wal" (dict "compression" $b.compression "maxParallel" (int $b.walMaxParallel)) -}}
{{- $_ := set $store "data" (dict "compression" $b.compression "immediateCheckpoint" $b.immediateCheckpoint) -}}
{{- end -}}
{{- $store | toJson -}}
{{- end -}}
