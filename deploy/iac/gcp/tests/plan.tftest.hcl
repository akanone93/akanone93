# SPDX-FileCopyrightText: 2026 Plux contributors
# SPDX-License-Identifier: Apache-2.0

# Plan-level tests of the Google Cloud module with a mock provider: no project,
# no network. They prove the planned resources and the secure defaults
# (DEP-050); they cannot prove how Google Cloud behaves.

mock_provider "google" {}

variables {
  name        = "plux-test"
  project_id  = "plux-test-project"
  region      = "europe-west1"
  zones       = ["europe-west1-b", "europe-west1-c"]
  bucket_name = "plux-test-objects"
}

# Verifies: DEP-050.
run "DEP_050_bucket_is_versioned_encrypted_and_refuses_public_access" {
  command = plan

  assert {
    condition     = google_storage_bucket.objects.public_access_prevention == "enforced"
    error_message = "public access prevention must be enforced"
  }

  assert {
    condition     = google_storage_bucket.objects.uniform_bucket_level_access == true
    error_message = "uniform bucket-level access must be on"
  }

  assert {
    condition     = google_storage_bucket.objects.versioning[0].enabled == true
    error_message = "the bucket must be versioned"
  }

  assert {
    condition     = google_storage_bucket.objects.force_destroy == false
    error_message = "the bucket must not be destroyed with its objects"
  }
}

# Verifies: DEP-050.
run "DEP_050_cluster_has_a_private_endpoint_private_nodes_and_enforced_policies" {
  command = plan

  assert {
    condition     = google_container_cluster.plux.private_cluster_config[0].enable_private_endpoint == true
    error_message = "the control plane must have a private endpoint"
  }

  assert {
    condition     = google_container_cluster.plux.private_cluster_config[0].enable_private_nodes == true
    error_message = "the nodes must have no external address"
  }

  assert {
    condition     = google_container_cluster.plux.datapath_provider == "ADVANCED_DATAPATH"
    error_message = "Dataplane V2 must enforce NetworkPolicies"
  }

  assert {
    condition     = google_container_cluster.plux.database_encryption[0].state == "ENCRYPTED"
    error_message = "Kubernetes secrets must be encrypted with Cloud KMS"
  }

  assert {
    condition     = google_container_cluster.plux.deletion_protection == true
    error_message = "the cluster must be protected from deletion"
  }
}

# Verifies: DEP-050.
run "DEP_050_nodes_use_a_dedicated_identity_not_the_default_one" {
  command = plan

  assert {
    condition     = google_container_node_pool.system.node_config[0].service_account == google_service_account.nodes.email
    error_message = "the node pool must run as the dedicated service account"
  }

  assert {
    condition     = google_container_node_pool.system.node_config[0].workload_metadata_config[0].mode == "GKE_METADATA"
    error_message = "workload identity metadata must be on"
  }
}

# Verifies: DEP-050.
run "DEP_050_managed_database_is_private_encrypted_and_protected" {
  command = plan

  assert {
    condition     = google_sql_database_instance.plux[0].settings[0].ip_configuration[0].ipv4_enabled == false
    error_message = "the database must have no public IP"
  }

  assert {
    condition     = google_sql_database_instance.plux[0].settings[0].ip_configuration[0].ssl_mode == "ENCRYPTED_ONLY"
    error_message = "the database must accept TLS only"
  }

  assert {
    condition     = google_sql_database_instance.plux[0].deletion_protection == true
    error_message = "the database must be protected from deletion"
  }

  assert {
    condition     = google_sql_database_instance.plux[0].encryption_key_name != null
    error_message = "the database must be encrypted with a customer managed key"
  }
}

# Verifies: DEP-050.
run "DEP_050_cache_is_private_authenticated_and_encrypted_in_transit" {
  command = plan

  assert {
    condition     = google_redis_instance.plux.connect_mode == "PRIVATE_SERVICE_ACCESS"
    error_message = "the cache must have a private IP only"
  }

  assert {
    condition     = google_redis_instance.plux.auth_enabled == true
    error_message = "the cache must require AUTH"
  }

  assert {
    condition     = google_redis_instance.plux.transit_encryption_mode == "SERVER_AUTHENTICATION"
    error_message = "the cache must use TLS"
  }
}

# Verifies: DEP-050.
run "DEP_050_signing_key_is_an_asymmetric_signing_key" {
  command = plan

  assert {
    condition     = google_kms_crypto_key.signing.purpose == "ASYMMETRIC_SIGN"
    error_message = "the signing key must be an asymmetric signing key"
  }

  assert {
    condition     = google_kms_crypto_key.data.rotation_period == "7776000s"
    error_message = "the data key must rotate every 90 days"
  }
}

# Verifies: DEP-050.
run "DEP_050_cnpg_mode_creates_no_managed_database" {
  command = plan

  variables {
    postgresql_mode = "cnpg"
  }

  assert {
    condition     = length(google_sql_database_instance.plux) == 0
    error_message = "cnpg mode must not create a Cloud SQL instance"
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
run "DEP_050_rejects_a_control_plane_range_that_is_not_a_/28" {
  command = plan

  variables {
    master_cidr = "172.16.0.0/24"
  }

  expect_failures = [var.master_cidr]
}

# Verifies: DEP-050.
run "DEP_050_rejects_a_single_zone" {
  command = plan

  variables {
    zones = ["europe-west1-b"]
  }

  expect_failures = [var.zones]
}
