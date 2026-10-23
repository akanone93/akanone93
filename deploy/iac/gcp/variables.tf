# SPDX-FileCopyrightText: 2026 Plux contributors
# SPDX-License-Identifier: Apache-2.0

variable "name" {
  description = "Name prefix of every resource: 3 to 20 lowercase letters, digits or hyphens, starting with a letter."
  type        = string

  validation {
    condition     = can(regex("^[a-z][a-z0-9-]{1,18}[a-z0-9]$", var.name))
    error_message = "name must be 3 to 20 lowercase letters, digits or hyphens, starting with a letter."
  }
}

variable "project_id" {
  description = "Project that holds the installation."
  type        = string
}

variable "region" {
  description = "Region of the cluster, the database, the cache and the bucket."
  type        = string
}

variable "zones" {
  description = "Two or three zones of the region, in which the nodes, the cache and the database run."
  type        = list(string)

  validation {
    condition     = length(var.zones) >= 2 && length(var.zones) <= 3
    error_message = "zones must list two or three zones."
  }
}

variable "subnet_cidr" {
  description = "Primary IPv4 range of the cluster subnet, for the nodes."
  type        = string
  default     = "10.30.0.0/20"
}

variable "pods_cidr" {
  description = "Secondary range of the subnet for the pods."
  type        = string
  default     = "10.40.0.0/14"
}

variable "services_cidr" {
  description = "Secondary range of the subnet for the Kubernetes services."
  type        = string
  default     = "10.60.0.0/20"
}

variable "master_cidr" {
  description = "/28 range of the control plane, which must not overlap the others."
  type        = string
  default     = "172.16.0.0/28"

  validation {
    condition     = can(regex("/28$", var.master_cidr))
    error_message = "master_cidr must be a /28 range."
  }
}

variable "authorized_cidrs" {
  description = "CIDRs allowed to reach the private control plane, for example the CI runners' egress range. Empty means nothing outside the subnet can reach it."
  type        = list(string)
  default     = []
}

variable "release_channel" {
  description = "GKE release channel of the cluster."
  type        = string
  default     = "REGULAR"

  validation {
    condition     = contains(["RAPID", "REGULAR", "STABLE"], var.release_channel)
    error_message = "release_channel must be RAPID, REGULAR or STABLE."
  }
}

variable "node_machine_type" {
  description = "Machine type of the nodes."
  type        = string
  default     = "e2-standard-4"
}

variable "node_min_count" {
  description = "Minimum nodes per zone."
  type        = number
  default     = 1

  validation {
    condition     = var.node_min_count >= 1
    error_message = "node_min_count must be at least 1."
  }
}

variable "node_max_count" {
  description = "Maximum nodes per zone the autoscaler may reach."
  type        = number
  default     = 2

  validation {
    condition     = var.node_max_count >= var.node_min_count && var.node_max_count <= 50
    error_message = "node_max_count must be at least node_min_count and at most 50."
  }
}

variable "postgresql_mode" {
  description = "\"managed\" creates a Cloud SQL for PostgreSQL instance; \"cnpg\" creates no database and the chart's CloudNativePG Cluster is used on the cluster instead."
  type        = string
  default     = "managed"

  validation {
    condition     = contains(["managed", "cnpg"], var.postgresql_mode)
    error_message = "postgresql_mode must be \"managed\" or \"cnpg\"."
  }
}

variable "postgresql_version" {
  description = "Cloud SQL database version (managed mode only)."
  type        = string
  default     = "POSTGRES_16"
}

variable "postgresql_tier" {
  description = "Cloud SQL machine tier of the database (managed mode only)."
  type        = string
  default     = "db-custom-2-7680"
}

variable "postgresql_storage_gb" {
  description = "Disk size of the database in GiB (managed mode only)."
  type        = number
  default     = 100

  validation {
    condition     = var.postgresql_storage_gb >= 10
    error_message = "postgresql_storage_gb must be at least 10."
  }
}

variable "postgresql_backup_retention_days" {
  description = "Automated backups kept, in days, for point-in-time recovery (managed mode only)."
  type        = number
  default     = 14

  validation {
    condition     = var.postgresql_backup_retention_days >= 7 && var.postgresql_backup_retention_days <= 365
    error_message = "postgresql_backup_retention_days must be between 7 and 365."
  }
}

variable "cache_memory_gb" {
  description = "Memory of the Memorystore for Redis instance in GiB. It is highly available across two zones."
  type        = number
  default     = 1

  validation {
    condition     = var.cache_memory_gb >= 1 && var.cache_memory_gb <= 300
    error_message = "cache_memory_gb must be between 1 and 300."
  }
}

variable "bucket_name" {
  description = "Name of the Cloud Storage bucket that holds bundles and deltas. Bucket names are global."
  type        = string

  validation {
    condition     = can(regex("^[a-z0-9][a-z0-9_.-]{1,61}[a-z0-9]$", var.bucket_name))
    error_message = "bucket_name must be a valid Cloud Storage bucket name."
  }
}

variable "kms_protection_level" {
  description = "Protection level of the encryption keys: HSM keeps them in a hardware security module."
  type        = string
  default     = "HSM"

  validation {
    condition     = contains(["SOFTWARE", "HSM"], var.kms_protection_level)
    error_message = "kms_protection_level must be SOFTWARE or HSM."
  }
}

variable "signing_algorithm" {
  description = "Cloud KMS algorithm of the asymmetric key that signs bundles. It must match the signing algorithm of the Plux server; see README."
  type        = string
  default     = "EC_SIGN_P256_SHA256"

  validation {
    condition     = contains(["EC_SIGN_P256_SHA256", "EC_SIGN_P384_SHA384", "EC_SIGN_ED25519", "RSA_SIGN_PSS_3072_SHA256"], var.signing_algorithm)
    error_message = "signing_algorithm must be an asymmetric signing algorithm of Cloud KMS."
  }
}

variable "labels" {
  description = "Labels added to every resource. Keys and values must be lowercase."
  type        = map(string)
  default     = {}
}
