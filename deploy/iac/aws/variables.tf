# SPDX-FileCopyrightText: 2026 Plux contributors
# SPDX-License-Identifier: Apache-2.0

variable "name" {
  description = "Name prefix of every resource: 3 to 24 lowercase letters, digits or hyphens, starting with a letter."
  type        = string

  validation {
    condition     = can(regex("^[a-z][a-z0-9-]{1,22}[a-z0-9]$", var.name))
    error_message = "name must be 3 to 24 lowercase letters, digits or hyphens, starting with a letter."
  }
}

variable "region" {
  description = "AWS region the root module configures. Used only for the Helm values output; the module does not configure a provider."
  type        = string
}

variable "vpc_cidr" {
  description = "IPv4 CIDR of the VPC. Each private subnet takes a /20 slice of it, so it must be a /16 or larger block."
  type        = string

  validation {
    condition     = can(cidrnetmask(var.vpc_cidr)) && tonumber(split("/", var.vpc_cidr)[1]) <= 16
    error_message = "vpc_cidr must be an IPv4 CIDR of /16 or larger."
  }
}

variable "availability_zones" {
  description = "Two or three availability zones of the region. The cluster, the database and the cache span all of them."
  type        = list(string)

  validation {
    condition     = length(var.availability_zones) >= 2 && length(var.availability_zones) <= 3
    error_message = "availability_zones must list two or three zones."
  }
}

variable "kubernetes_version" {
  description = "Amazon EKS Kubernetes version. Must be a version still supported by EKS on the day of the apply."
  type        = string
}

variable "node_instance_type" {
  description = "EC2 instance type of the worker nodes."
  type        = string
  default     = "m6i.large"
}

variable "node_min_size" {
  description = "Minimum number of worker nodes."
  type        = number
  default     = 3

  validation {
    condition     = var.node_min_size >= 2
    error_message = "node_min_size must be at least 2, so that a node loss does not stop the installation."
  }
}

variable "node_max_size" {
  description = "Maximum number of worker nodes the autoscaler may reach."
  type        = number
  default     = 6

  validation {
    condition     = var.node_max_size >= var.node_min_size && var.node_max_size <= 100
    error_message = "node_max_size must be at least node_min_size and at most 100."
  }
}

variable "node_desired_size" {
  description = "Desired number of worker nodes at creation; the autoscaler then owns the count."
  type        = number
  default     = 3

  validation {
    condition     = var.node_desired_size >= var.node_min_size && var.node_desired_size <= var.node_max_size
    error_message = "node_desired_size must lie between node_min_size and node_max_size."
  }
}

variable "postgresql_mode" {
  description = "\"managed\" creates an RDS for PostgreSQL instance; \"cnpg\" creates no database and the chart's CloudNativePG Cluster is used on the cluster instead."
  type        = string
  default     = "managed"

  validation {
    condition     = contains(["managed", "cnpg"], var.postgresql_mode)
    error_message = "postgresql_mode must be \"managed\" or \"cnpg\"."
  }
}

variable "postgresql_engine_version" {
  description = "Major version of the RDS for PostgreSQL engine (managed mode only)."
  type        = string
  default     = "16"
}

variable "postgresql_instance_class" {
  description = "RDS instance class of the database (managed mode only)."
  type        = string
  default     = "db.m6i.large"
}

variable "postgresql_storage_gb" {
  description = "Allocated storage of the database in GiB (managed mode only)."
  type        = number
  default     = 100

  validation {
    condition     = var.postgresql_storage_gb >= 20
    error_message = "postgresql_storage_gb must be at least 20."
  }
}

variable "postgresql_backup_retention_days" {
  description = "Days of automated backups kept for point-in-time recovery (managed mode only)."
  type        = number
  default     = 14

  validation {
    condition     = var.postgresql_backup_retention_days >= 7 && var.postgresql_backup_retention_days <= 35
    error_message = "postgresql_backup_retention_days must be between 7 and 35."
  }
}

variable "valkey_engine_version" {
  description = "ElastiCache for Valkey engine version."
  type        = string
  default     = "8.0"
}

variable "valkey_node_type" {
  description = "ElastiCache node type of every cache node."
  type        = string
  default     = "cache.t4g.medium"
}

variable "valkey_replicas" {
  description = "Number of cache nodes, a primary and its replicas. At least two, so that automatic failover across zones works."
  type        = number
  default     = 2

  validation {
    condition     = var.valkey_replicas >= 2 && var.valkey_replicas <= 6
    error_message = "valkey_replicas must be between 2 and 6."
  }
}

variable "valkey_auth_token" {
  description = "Password of the cache, required by the transit encryption. Supply it from a secret store through TF_VAR_valkey_auth_token; it is never defaulted."
  type        = string
  sensitive   = true

  validation {
    condition     = length(var.valkey_auth_token) >= 16 && length(var.valkey_auth_token) <= 128 && can(regex("^[\\x21-\\x7e]+$", var.valkey_auth_token))
    error_message = "valkey_auth_token must be 16 to 128 printable characters without spaces."
  }
}

variable "bucket_name" {
  description = "Name of the object-storage bucket that holds bundles and deltas. Bucket names are global to AWS."
  type        = string

  validation {
    condition     = can(regex("^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$", var.bucket_name))
    error_message = "bucket_name must be a valid S3 bucket name (3 to 63 lowercase characters)."
  }
}

variable "kms_deletion_window_days" {
  description = "Days a scheduled KMS key deletion waits before the key is removed."
  type        = number
  default     = 30

  validation {
    condition     = var.kms_deletion_window_days >= 7 && var.kms_deletion_window_days <= 30
    error_message = "kms_deletion_window_days must be between 7 and 30."
  }
}

variable "signing_key_spec" {
  description = "KMS key spec of the asymmetric key that signs bundles. It must match the signing algorithm of the Plux server; see README."
  type        = string
  default     = "ECC_NIST_P256"

  validation {
    condition     = contains(["ECC_NIST_P256", "ECC_NIST_P384", "ECC_NIST_EDWARDS25519", "RSA_3072", "RSA_4096"], var.signing_key_spec)
    error_message = "signing_key_spec must be one of the asymmetric SIGN_VERIFY key specs of AWS KMS."
  }
}

variable "tags" {
  description = "Tags added to every resource."
  type        = map(string)
  default     = {}
}
