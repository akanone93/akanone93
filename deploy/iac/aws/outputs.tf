# SPDX-FileCopyrightText: 2026 Plux contributors
# SPDX-License-Identifier: Apache-2.0

output "cluster_name" {
  description = "Name of the EKS cluster; the kubeconfig is built from it with the AWS CLI."
  value       = aws_eks_cluster.plux.name
}

output "cluster_endpoint" {
  description = "Private API endpoint of the EKS cluster."
  value       = aws_eks_cluster.plux.endpoint
}

output "bucket_name" {
  description = "Name of the object-storage bucket for bundles and deltas."
  value       = aws_s3_bucket.objects.bucket
}

output "database_address" {
  description = "Host name of the managed database; null in cnpg mode."
  value       = one(aws_db_instance.plux[*].address)
}

output "cache_primary_endpoint" {
  description = "Host name of the cache's primary endpoint, for the rediss:// URL the operator stores in a Secret."
  value       = aws_elasticache_replication_group.plux.primary_endpoint_address
}

output "data_key_arn" {
  description = "ARN of the KMS key that encrypts the cluster secrets, the database, the bucket and the cache."
  value       = aws_kms_key.data.arn
}

output "signing_key_arn" {
  description = "ARN of the asymmetric KMS key that signs bundles."
  value       = aws_kms_key.signing.arn
}

output "helm_values" {
  description = "Values for deploy/helm/plux that point the chart at this module's stores. Contains no secret: the operator creates the Secrets the chart references."
  value = {
    postgresql = {
      mode     = local.managed_postgres ? "external" : "cnpg"
      database = "plux"
    }
    valkey = {
      mode = "external"
    }
    objectStorage = {
      mode      = "external"
      bucket    = aws_s3_bucket.objects.bucket
      region    = var.region
      pathStyle = false
    }
  }
}
