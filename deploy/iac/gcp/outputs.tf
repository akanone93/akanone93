# SPDX-FileCopyrightText: 2026 Plux contributors
# SPDX-License-Identifier: Apache-2.0

output "cluster_name" {
  description = "Name of the GKE cluster; its credentials are fetched with gcloud."
  value       = google_container_cluster.plux.name
}

output "cluster_endpoint" {
  description = "Private endpoint of the GKE control plane."
  value       = google_container_cluster.plux.private_cluster_config[0].private_endpoint
}

output "bucket_name" {
  description = "Name of the Cloud Storage bucket for bundles and deltas."
  value       = google_storage_bucket.objects.name
}

output "database_address" {
  description = "Private IP of the Cloud SQL instance; null in cnpg mode."
  value       = one(google_sql_database_instance.plux[*].private_ip_address)
}

output "cache_host" {
  description = "Private host of the Memorystore instance, for the rediss:// URL the operator stores in a Secret."
  value       = google_redis_instance.plux.host
}

output "data_key_id" {
  description = "Full ID of the Cloud KMS key that encrypts the cluster secrets, the database, the bucket and the cache."
  value       = google_kms_crypto_key.data.id
}

output "signing_key_id" {
  description = "Full ID of the asymmetric Cloud KMS key that signs bundles."
  value       = google_kms_crypto_key.signing.id
}

output "helm_values" {
  description = "Values for deploy/helm/plux that point the chart at this module's stores. Contains no secret: the operator creates the Secrets the chart references (including the HMAC keys of the bucket's S3 interoperability)."
  value = {
    postgresql = {
      mode     = local.managed_postgres ? "external" : "cnpg"
      database = "plux"
    }
    valkey = {
      mode = "external"
    }
    objectStorage = {
      mode   = "external"
      bucket = google_storage_bucket.objects.name
      region = var.region
      external = {
        endpoint = "https://storage.googleapis.com"
      }
    }
  }
}
