# SPDX-FileCopyrightText: 2026 Plux contributors
# SPDX-License-Identifier: Apache-2.0

output "cluster_name" {
  description = "Name of the AKS cluster; its credentials are fetched with az aks get-credentials."
  value       = azurerm_kubernetes_cluster.plux.name
}

output "cluster_fqdn" {
  description = "Private fully qualified domain name of the AKS API server."
  value       = azurerm_kubernetes_cluster.plux.private_fqdn
}

output "blob_endpoint" {
  description = "Primary blob endpoint of the storage account. Azure Blob Storage has no S3 API, so the chart cannot use it as objectStorage (deploy/iac/README.md, \"Open questions\")."
  value       = azurerm_storage_account.objects.primary_blob_endpoint
}

output "database_fqdn" {
  description = "Private host name of the flexible server; null in cnpg mode."
  value       = one(azurerm_postgresql_flexible_server.plux[*].fqdn)
}

output "cache_hostname" {
  description = "Host name of the cache, for the rediss:// URL the operator stores in a Secret."
  value       = azurerm_redis_cache.plux.hostname
}

output "key_vault_uri" {
  description = "URI of the Key Vault that holds the data key and the signing key."
  value       = azurerm_key_vault.plux.vault_uri
}

output "signing_key_id" {
  description = "Versionless ID of the EC key that signs bundles."
  value       = azurerm_key_vault_key.signing.versionless_id
}

output "helm_values" {
  description = "Values for deploy/helm/plux that point the chart at this module's database and cache. The object storage is not included: the chart needs an S3 API and Azure Blob Storage has none. Contains no secret."
  value = {
    postgresql = {
      mode     = local.managed_postgres ? "external" : "cnpg"
      database = "plux"
    }
    valkey = {
      mode = "external"
    }
  }
}
