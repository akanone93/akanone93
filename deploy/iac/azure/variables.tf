# SPDX-FileCopyrightText: 2026 Plux contributors
# SPDX-License-Identifier: Apache-2.0

variable "name" {
  description = "Name prefix of the installation: 3 to 24 lowercase letters, digits or hyphens, starting with a letter. The cluster, the database and the cache take it as their name."
  type        = string

  validation {
    condition     = can(regex("^[a-z][a-z0-9-]{1,22}[a-z0-9]$", var.name))
    error_message = "name must be 3 to 24 lowercase letters, digits or hyphens, starting with a letter."
  }
}

variable "location" {
  description = "Azure region of every resource."
  type        = string
}

variable "zones" {
  description = "Two or three availability zones of the region, in which the nodes and the cache run."
  type        = list(string)

  validation {
    condition     = length(var.zones) >= 2 && length(var.zones) <= 3
    error_message = "zones must list two or three zones."
  }
}

variable "vnet_address_space" {
  description = "IPv4 address space of the virtual network. The subnets below must lie inside it."
  type        = string
  default     = "10.50.0.0/16"

  validation {
    condition     = can(cidrnetmask(var.vnet_address_space))
    error_message = "vnet_address_space must be an IPv4 CIDR."
  }
}

variable "subnet_cidrs" {
  description = "CIDRs of the four subnets: aks (nodes), postgresql (delegated to the flexible server), cache (Premium Redis, injected) and endpoints (private endpoints)."
  type = object({
    aks        = string
    postgresql = string
    cache      = string
    endpoints  = string
  })
  default = {
    aks        = "10.50.0.0/20"
    postgresql = "10.50.16.0/24"
    cache      = "10.50.17.0/24"
    endpoints  = "10.50.18.0/24"
  }

  validation {
    condition     = alltrue([for c in values(var.subnet_cidrs) : can(cidrnetmask(c))])
    error_message = "every subnet_cidrs entry must be an IPv4 CIDR."
  }
}

variable "kubernetes_version" {
  description = "AKS Kubernetes version. Null takes the version the region supports by default."
  type        = string
  default     = null
}

variable "kubernetes_service_cidr" {
  description = "CIDR of the Kubernetes services. It must not overlap the virtual network or the pod overlay."
  type        = string
  default     = "172.20.0.0/16"
}

variable "node_vm_size" {
  description = "VM size of the system node pool."
  type        = string
  default     = "Standard_D4s_v5"
}

variable "node_min_count" {
  description = "Minimum nodes of the system pool."
  type        = number
  default     = 2

  validation {
    condition     = var.node_min_count >= 2
    error_message = "node_min_count must be at least 2, so that a node loss does not stop the installation."
  }
}

variable "node_max_count" {
  description = "Maximum nodes of the system pool the autoscaler may reach."
  type        = number
  default     = 6

  validation {
    condition     = var.node_max_count >= var.node_min_count && var.node_max_count <= 100
    error_message = "node_max_count must be at least node_min_count and at most 100."
  }
}

variable "postgresql_mode" {
  description = "\"managed\" creates an Azure Database for PostgreSQL flexible server; \"cnpg\" creates no database and the chart's CloudNativePG Cluster is used on the cluster instead."
  type        = string
  default     = "managed"

  validation {
    condition     = contains(["managed", "cnpg"], var.postgresql_mode)
    error_message = "postgresql_mode must be \"managed\" or \"cnpg\"."
  }
}

variable "postgresql_version" {
  description = "Major version of the PostgreSQL flexible server (managed mode only)."
  type        = string
  default     = "16"
}

variable "postgresql_sku_name" {
  description = "SKU of the flexible server, for example GP_Standard_D2ds_v5 (managed mode only)."
  type        = string
  default     = "GP_Standard_D2ds_v5"
}

variable "postgresql_storage_mb" {
  description = "Storage of the flexible server in MiB (managed mode only)."
  type        = number
  default     = 131072

  validation {
    condition     = var.postgresql_storage_mb >= 32768
    error_message = "postgresql_storage_mb must be at least 32768 (32 GiB)."
  }
}

variable "postgresql_backup_retention_days" {
  description = "Days of backups kept for point-in-time recovery (managed mode only)."
  type        = number
  default     = 14

  validation {
    condition     = var.postgresql_backup_retention_days >= 7 && var.postgresql_backup_retention_days <= 35
    error_message = "postgresql_backup_retention_days must be between 7 and 35."
  }
}

variable "cache_capacity" {
  description = "Capacity of the Premium Azure Cache for Redis: 1 is 6 GB, each step doubles it."
  type        = number
  default     = 1

  validation {
    condition     = contains([1, 2, 3, 4, 5], var.cache_capacity)
    error_message = "cache_capacity must be between 1 and 5."
  }
}

variable "storage_account_name" {
  description = "Name of the storage account: 3 to 24 lowercase letters and digits, unique across Azure."
  type        = string

  validation {
    condition     = can(regex("^[a-z0-9]{3,24}$", var.storage_account_name))
    error_message = "storage_account_name must be 3 to 24 lowercase letters and digits."
  }
}

variable "container_name" {
  description = "Name of the blob container that holds bundles and deltas."
  type        = string
  default     = "objects"

  validation {
    condition     = can(regex("^[a-z0-9](?:[a-z0-9]|-(?:[a-z0-9])){2,62}$", var.container_name))
    error_message = "container_name must be a valid blob container name."
  }
}

variable "key_vault_name" {
  description = "Name of the Key Vault that holds the data key and the signing key: 3 to 24 letters, digits or hyphens."
  type        = string

  validation {
    condition     = can(regex("^[a-zA-Z][a-zA-Z0-9-]{1,22}[a-zA-Z0-9]$", var.key_vault_name))
    error_message = "key_vault_name must be 3 to 24 letters, digits or hyphens, starting with a letter."
  }
}

variable "signing_key_curve" {
  description = "Elliptic curve of the EC key that signs bundles. It must match the signing algorithm of the Plux server; see README."
  type        = string
  default     = "P-256"

  validation {
    condition     = contains(["P-256", "P-384", "P-521"], var.signing_key_curve)
    error_message = "signing_key_curve must be P-256, P-384 or P-521."
  }
}

variable "tags" {
  description = "Tags added to every resource."
  type        = map(string)
  default     = {}
}
