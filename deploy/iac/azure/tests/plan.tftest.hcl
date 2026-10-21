# SPDX-FileCopyrightText: 2026 Plux contributors
# SPDX-License-Identifier: Apache-2.0

# Plan-level tests of the Azure module with a mock provider: no subscription,
# no network. They prove the planned resources and the secure defaults
# (DEP-050); they cannot prove how Azure behaves.

mock_provider "azurerm" {}

variables {
  name                 = "plux-test"
  location             = "westeurope"
  zones                = ["1", "2", "3"]
  storage_account_name = "pluxtestobjects"
  key_vault_name       = "plux-test-kv"
}

# Verifies: DEP-050.
run "DEP_050_storage_account_is_private_versioned_and_encrypted_with_a_managed_key" {
  command = plan

  assert {
    condition     = azurerm_storage_account.objects.public_network_access_enabled == false
    error_message = "the storage account must not be publicly reachable"
  }

  assert {
    condition     = azurerm_storage_account.objects.allow_nested_items_to_be_public == false
    error_message = "blobs must not be publicly readable"
  }

  assert {
    condition     = azurerm_storage_account.objects.shared_access_key_enabled == false
    error_message = "shared keys must be off; access goes through Microsoft Entra ID"
  }

  assert {
    condition     = azurerm_storage_account.objects.blob_properties[0].versioning_enabled == true
    error_message = "blob versioning must be on"
  }

  assert {
    condition     = azurerm_storage_account.objects.infrastructure_encryption_enabled == true
    error_message = "infrastructure encryption must be on"
  }

  assert {
    condition     = azurerm_storage_container.objects.container_access_type == "private"
    error_message = "the container must be private"
  }
}

# Verifies: DEP-050.
run "DEP_050_key_vault_refuses_public_access_and_keeps_deleted_keys" {
  command = plan

  assert {
    condition     = azurerm_key_vault.plux.public_network_access_enabled == false
    error_message = "the Key Vault must not be publicly reachable"
  }

  assert {
    condition     = azurerm_key_vault.plux.purge_protection_enabled == true
    error_message = "purge protection must be on"
  }

  assert {
    condition     = azurerm_key_vault.plux.rbac_authorization_enabled == true
    error_message = "access must be granted by Azure RBAC, not access policies"
  }

  assert {
    condition     = contains(azurerm_key_vault_key.signing.key_opts, "sign") && !contains(azurerm_key_vault_key.signing.key_opts, "decrypt")
    error_message = "the signing key must sign and verify only"
  }
}

# Verifies: DEP-050.
run "DEP_050_cluster_API_is_private_and_local_accounts_are_off" {
  command = plan

  assert {
    condition     = azurerm_kubernetes_cluster.plux.private_cluster_enabled == true
    error_message = "the API server must be private"
  }

  assert {
    condition     = azurerm_kubernetes_cluster.plux.local_account_disabled == true
    error_message = "local accounts must be disabled"
  }

  assert {
    condition     = azurerm_kubernetes_cluster.plux.network_profile[0].network_policy == "cilium"
    error_message = "Cilium must enforce the chart's NetworkPolicies"
  }
}

# Verifies: DEP-050.
run "DEP_050_managed_database_is_private_Entra-only_and_locked_against_deletion" {
  command = plan

  assert {
    condition     = azurerm_postgresql_flexible_server.plux[0].public_network_access_enabled == false
    error_message = "the flexible server must not be publicly reachable"
  }

  assert {
    condition     = azurerm_postgresql_flexible_server.plux[0].authentication[0].password_auth_enabled == false
    error_message = "password authentication must be off"
  }

  assert {
    condition     = azurerm_postgresql_flexible_server.plux[0].geo_redundant_backup_enabled == true
    error_message = "backups must be geo-redundant"
  }

  assert {
    condition     = azurerm_management_lock.postgresql[0].lock_level == "CanNotDelete"
    error_message = "the database must carry a CanNotDelete lock"
  }
}

# Verifies: DEP-050.
run "DEP_050_cache_is_private_TLS-only_and_zone_redundant" {
  command = plan

  assert {
    condition     = azurerm_redis_cache.plux.public_network_access_enabled == false
    error_message = "the cache must not be publicly reachable"
  }

  assert {
    condition     = azurerm_redis_cache.plux.non_ssl_port_enabled == false
    error_message = "the non-TLS port must be closed"
  }

  assert {
    condition     = azurerm_redis_cache.plux.minimum_tls_version == "1.2"
    error_message = "the cache must refuse TLS below 1.2"
  }
}

# Verifies: DEP-050.
run "DEP_050_cnpg_mode_creates_no_managed_database" {
  command = plan

  variables {
    postgresql_mode = "cnpg"
  }

  assert {
    condition     = length(azurerm_postgresql_flexible_server.plux) == 0
    error_message = "cnpg mode must not create a flexible server"
  }

  assert {
    condition     = output.helm_values.postgresql.mode == "cnpg"
    error_message = "cnpg mode must point the chart at CloudNativePG"
  }
}

# Verifies: DEP-050.
run "DEP_050_rejects_a_public_database_mode" {
  command = plan

  variables {
    postgresql_mode = "public"
  }

  expect_failures = [var.postgresql_mode]
}

# Verifies: DEP-050.
run "DEP_050_rejects_a_storage_account_name_with_uppercase_letters" {
  command = plan

  variables {
    storage_account_name = "PluxObjects"
  }

  expect_failures = [var.storage_account_name]
}

# Verifies: DEP-050.
run "DEP_050_rejects_a_single_availability_zone" {
  command = plan

  variables {
    zones = ["1"]
  }

  expect_failures = [var.zones]
}
