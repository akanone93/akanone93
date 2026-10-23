# SPDX-FileCopyrightText: 2026 Plux contributors
# SPDX-License-Identifier: Apache-2.0

# Azure reference infrastructure for a Plux installation (DEP-050). The
# cluster is private, the database, the cache, the storage account and the
# Key Vault are reachable from the virtual network only, and the stores are
# encrypted at rest. Nothing here holds a credential: the database has no
# password (Microsoft Entra ID authentication only), and the storage account
# has no shared key.

locals {
  tags             = merge(var.tags, { "plux:module" = "deploy/iac/azure" })
  managed_postgres = var.postgresql_mode == "managed"
}

data "azurerm_client_config" "current" {}

# ── Networking ───────────────────────────────────────────────────────────────

resource "azurerm_resource_group" "plux" {
  name     = "${var.name}-rg"
  location = var.location
  tags     = local.tags
}

resource "azurerm_virtual_network" "plux" {
  name                = var.name
  location            = azurerm_resource_group.plux.location
  resource_group_name = azurerm_resource_group.plux.name
  address_space       = [var.vnet_address_space]
  tags                = local.tags
}

resource "azurerm_subnet" "aks" {
  name                 = "aks"
  resource_group_name  = azurerm_resource_group.plux.name
  virtual_network_name = azurerm_virtual_network.plux.name
  address_prefixes     = [var.subnet_cidrs.aks]
}

# The flexible server takes its private address from this subnet, which only
# the server may use.
resource "azurerm_subnet" "postgresql" {
  count                = local.managed_postgres ? 1 : 0
  name                 = "postgresql"
  resource_group_name  = azurerm_resource_group.plux.name
  virtual_network_name = azurerm_virtual_network.plux.name
  address_prefixes     = [var.subnet_cidrs.postgresql]

  delegation {
    name = "postgresql"
    service_delegation {
      name    = "Microsoft.DBforPostgreSQL/flexibleServers"
      actions = ["Microsoft.Network/virtualNetworks/subnets/join/action"]
    }
  }
}

# The Premium cache is injected into this subnet, which holds nothing else.
resource "azurerm_subnet" "cache" {
  name                 = "cache"
  resource_group_name  = azurerm_resource_group.plux.name
  virtual_network_name = azurerm_virtual_network.plux.name
  address_prefixes     = [var.subnet_cidrs.cache]
}

resource "azurerm_subnet" "endpoints" {
  name                 = "endpoints"
  resource_group_name  = azurerm_resource_group.plux.name
  virtual_network_name = azurerm_virtual_network.plux.name
  address_prefixes     = [var.subnet_cidrs.endpoints]
}

# ── Keys ─────────────────────────────────────────────────────────────────────

# Public network access is off: the keys are created from inside the virtual
# network (deploy/iac/README.md, "Running the apply").
resource "azurerm_key_vault" "plux" {
  name                          = var.key_vault_name
  location                      = azurerm_resource_group.plux.location
  resource_group_name           = azurerm_resource_group.plux.name
  tenant_id                     = data.azurerm_client_config.current.tenant_id
  sku_name                      = "standard"
  rbac_authorization_enabled    = true
  purge_protection_enabled      = true
  soft_delete_retention_days    = 90
  public_network_access_enabled = false
  tags                          = local.tags

  network_acls {
    default_action = "Deny"
    bypass         = "AzureServices"
  }
}

# The data key wraps the storage account's encryption key. It is never exported.
resource "azurerm_key_vault_key" "data" {
  name         = "${var.name}-data"
  key_vault_id = azurerm_key_vault.plux.id
  key_type     = "RSA"
  key_size     = 3072
  key_opts     = ["wrapKey", "unwrapKey"]
}

# The signing key signs and verifies and never leaves Key Vault: the server
# asks Key Vault to sign and holds no private key material.
resource "azurerm_key_vault_key" "signing" {
  name         = "${var.name}-signing"
  key_vault_id = azurerm_key_vault.plux.id
  key_type     = "EC"
  curve        = var.signing_key_curve
  key_opts     = ["sign", "verify"]
}

resource "azurerm_private_dns_zone" "vault" {
  name                = "privatelink.vaultcore.azure.net"
  resource_group_name = azurerm_resource_group.plux.name
  tags                = local.tags
}

resource "azurerm_private_dns_zone_virtual_network_link" "vault" {
  name                  = "${var.name}-vault"
  resource_group_name   = azurerm_resource_group.plux.name
  private_dns_zone_name = azurerm_private_dns_zone.vault.name
  virtual_network_id    = azurerm_virtual_network.plux.id
  registration_enabled  = false
  tags                  = local.tags
}

resource "azurerm_private_endpoint" "vault" {
  name                = "${var.name}-vault"
  location            = azurerm_resource_group.plux.location
  resource_group_name = azurerm_resource_group.plux.name
  subnet_id           = azurerm_subnet.endpoints.id
  tags                = local.tags

  private_service_connection {
    name                           = "${var.name}-vault"
    private_connection_resource_id = azurerm_key_vault.plux.id
    subresource_names              = ["vault"]
    is_manual_connection           = false
  }

  private_dns_zone_group {
    name                 = "vault"
    private_dns_zone_ids = [azurerm_private_dns_zone.vault.id]
  }
}

# ── Kubernetes cluster ───────────────────────────────────────────────────────

# A private API server, local accounts off (only Microsoft Entra ID can
# authenticate), Azure RBAC for Kubernetes, and Cilium, which enforces the
# chart's NetworkPolicies.
resource "azurerm_kubernetes_cluster" "plux" {
  name                              = var.name
  location                          = azurerm_resource_group.plux.location
  resource_group_name               = azurerm_resource_group.plux.name
  dns_prefix                        = var.name
  kubernetes_version                = var.kubernetes_version
  private_cluster_enabled           = true
  local_account_disabled            = true
  automatic_upgrade_channel         = "patch"
  oidc_issuer_enabled               = true
  workload_identity_enabled         = true
  role_based_access_control_enabled = true
  tags                              = local.tags

  azure_active_directory_role_based_access_control {
    azure_rbac_enabled = true
    tenant_id          = data.azurerm_client_config.current.tenant_id
  }

  default_node_pool {
    name                 = "system"
    vm_size              = var.node_vm_size
    vnet_subnet_id       = azurerm_subnet.aks.id
    zones                = var.zones
    auto_scaling_enabled = true
    min_count            = var.node_min_count
    max_count            = var.node_max_count
  }

  identity {
    type = "SystemAssigned"
  }

  network_profile {
    network_plugin      = "azure"
    network_plugin_mode = "overlay"
    network_data_plane  = "cilium"
    network_policy      = "cilium"
    service_cidr        = var.kubernetes_service_cidr
    dns_service_ip      = cidrhost(var.kubernetes_service_cidr, 10)
  }
}

# ── Database (managed mode only) ─────────────────────────────────────────────

# The server's name resolves through a private zone linked to the network only.
resource "azurerm_private_dns_zone" "postgresql" {
  count               = local.managed_postgres ? 1 : 0
  name                = "${var.name}.postgres.database.azure.com"
  resource_group_name = azurerm_resource_group.plux.name
  tags                = local.tags
}

resource "azurerm_private_dns_zone_virtual_network_link" "postgresql" {
  count                 = local.managed_postgres ? 1 : 0
  name                  = "${var.name}-postgresql"
  resource_group_name   = azurerm_resource_group.plux.name
  private_dns_zone_name = azurerm_private_dns_zone.postgresql[0].name
  virtual_network_id    = azurerm_virtual_network.plux.id
  registration_enabled  = false
  tags                  = local.tags
}

# Entra ID authentication only: no password exists to store or rotate.
# Geo-redundant backups and point-in-time recovery are on.
resource "azurerm_postgresql_flexible_server" "plux" {
  count = local.managed_postgres ? 1 : 0

  name                          = var.name
  resource_group_name           = azurerm_resource_group.plux.name
  location                      = azurerm_resource_group.plux.location
  version                       = var.postgresql_version
  delegated_subnet_id           = azurerm_subnet.postgresql[0].id
  private_dns_zone_id           = azurerm_private_dns_zone.postgresql[0].id
  public_network_access_enabled = false
  sku_name                      = var.postgresql_sku_name
  storage_mb                    = var.postgresql_storage_mb
  auto_grow_enabled             = true
  backup_retention_days         = var.postgresql_backup_retention_days
  geo_redundant_backup_enabled  = true
  tags                          = local.tags

  authentication {
    active_directory_auth_enabled = true
    password_auth_enabled         = false
    tenant_id                     = data.azurerm_client_config.current.tenant_id
  }

  depends_on = [azurerm_private_dns_zone_virtual_network_link.postgresql]
}

# Azure has no delete protection on a flexible server: a CanNotDelete lock
# stands in for it until an operator removes the lock on purpose.
resource "azurerm_management_lock" "postgresql" {
  count      = local.managed_postgres ? 1 : 0
  name       = "${var.name}-postgresql-nodelete"
  scope      = azurerm_postgresql_flexible_server.plux[0].id
  lock_level = "CanNotDelete"
  notes      = "Deletion protection of the Plux database (DEP-050)."
}

# ── Object storage ───────────────────────────────────────────────────────────

# Private blobs only, TLS 1.2 or later, no shared keys, versioning and soft
# delete on, and encryption with the Key Vault data key.
resource "azurerm_storage_account" "objects" {
  name                              = var.storage_account_name
  resource_group_name               = azurerm_resource_group.plux.name
  location                          = azurerm_resource_group.plux.location
  account_tier                      = "Standard"
  account_replication_type          = "ZRS"
  account_kind                      = "StorageV2"
  min_tls_version                   = "TLS1_2"
  https_traffic_only_enabled        = true
  public_network_access_enabled     = false
  allow_nested_items_to_be_public   = false
  shared_access_key_enabled         = false
  default_to_oauth_authentication   = true
  infrastructure_encryption_enabled = true
  tags                              = local.tags

  identity {
    type = "SystemAssigned"
  }

  blob_properties {
    versioning_enabled = true

    delete_retention_policy {
      days = 14
    }

    container_delete_retention_policy {
      days = 14
    }
  }

  network_rules {
    default_action = "Deny"
    bypass         = ["AzureServices"]
  }
}

resource "azurerm_storage_container" "objects" {
  name                  = var.container_name
  storage_account_id    = azurerm_storage_account.objects.id
  container_access_type = "private"
}

# The storage account reads its key from Key Vault with its own identity.
resource "azurerm_role_assignment" "storage_key" {
  scope                = azurerm_key_vault.plux.id
  role_definition_name = "Key Vault Crypto Service Encryption User"
  principal_id         = azurerm_storage_account.objects.identity[0].principal_id
}

resource "azurerm_storage_account_customer_managed_key" "objects" {
  storage_account_id = azurerm_storage_account.objects.id
  # The versionless key ID: Azure Storage follows the key's rotations.
  key_vault_key_id = azurerm_key_vault_key.data.versionless_id

  depends_on = [azurerm_role_assignment.storage_key]
}

resource "azurerm_private_dns_zone" "blob" {
  name                = "privatelink.blob.core.windows.net"
  resource_group_name = azurerm_resource_group.plux.name
  tags                = local.tags
}

resource "azurerm_private_dns_zone_virtual_network_link" "blob" {
  name                  = "${var.name}-blob"
  resource_group_name   = azurerm_resource_group.plux.name
  private_dns_zone_name = azurerm_private_dns_zone.blob.name
  virtual_network_id    = azurerm_virtual_network.plux.id
  registration_enabled  = false
  tags                  = local.tags
}

resource "azurerm_private_endpoint" "blob" {
  name                = "${var.name}-blob"
  location            = azurerm_resource_group.plux.location
  resource_group_name = azurerm_resource_group.plux.name
  subnet_id           = azurerm_subnet.endpoints.id
  tags                = local.tags

  private_service_connection {
    name                           = "${var.name}-blob"
    private_connection_resource_id = azurerm_storage_account.objects.id
    subresource_names              = ["blob"]
    is_manual_connection           = false
  }

  private_dns_zone_group {
    name                 = "blob"
    private_dns_zone_ids = [azurerm_private_dns_zone.blob.id]
  }
}

# ── Cache ────────────────────────────────────────────────────────────────────

# Premium Azure Cache for Redis, injected into its own subnet, with TLS 1.2
# only, the non-TLS port closed, no public access and zone redundancy. It is
# Redis-compatible, not Valkey: Azure has no managed Valkey service.
resource "azurerm_redis_cache" "plux" {
  name                          = var.name
  location                      = azurerm_resource_group.plux.location
  resource_group_name           = azurerm_resource_group.plux.name
  capacity                      = var.cache_capacity
  family                        = "P"
  sku_name                      = "Premium"
  non_ssl_port_enabled          = false
  minimum_tls_version           = "1.2"
  public_network_access_enabled = false
  subnet_id                     = azurerm_subnet.cache.id
  zones                         = var.zones
  replicas_per_primary          = 1
  tags                          = local.tags

  redis_configuration {
    authentication_enabled = true
  }
}
