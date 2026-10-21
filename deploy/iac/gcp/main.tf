# SPDX-FileCopyrightText: 2026 Plux contributors
# SPDX-License-Identifier: Apache-2.0

# Google Cloud reference infrastructure for a Plux installation (DEP-050). The
# cluster has a private endpoint and private nodes, the database and the cache
# have private IPs only, the bucket refuses public access and every store is
# encrypted with a customer managed Cloud KMS key. No credential is written
# here: the bucket's S3-interoperability keys are created by the operator.

locals {
  labels           = merge(var.labels, { plux_module = "deploy-iac-gcp" })
  managed_postgres = var.postgresql_mode == "managed"
}

# ── Networking ───────────────────────────────────────────────────────────────

resource "google_compute_network" "plux" {
  name                    = var.name
  project                 = var.project_id
  auto_create_subnetworks = false
  routing_mode            = "REGIONAL"
}

# Private Google access lets the nodes reach Google APIs without an external
# address. Flow logs record every connection for the audit trail.
resource "google_compute_subnetwork" "cluster" {
  name                     = "${var.name}-cluster"
  project                  = var.project_id
  region                   = var.region
  network                  = google_compute_network.plux.id
  ip_cidr_range            = var.subnet_cidr
  private_ip_google_access = true

  secondary_ip_range {
    range_name    = "pods"
    ip_cidr_range = var.pods_cidr
  }

  secondary_ip_range {
    range_name    = "services"
    ip_cidr_range = var.services_cidr
  }

  log_config {
    aggregation_interval = "INTERVAL_5_SEC"
    flow_sampling        = 0.5
    metadata             = "INCLUDE_ALL_METADATA"
  }
}

# Private service access: the peering that Cloud SQL and Memorystore use to
# place their private IPs in this network.
resource "google_compute_global_address" "private_services" {
  name          = "${var.name}-private-services"
  project       = var.project_id
  purpose       = "VPC_PEERING"
  address_type  = "INTERNAL"
  prefix_length = 16
  network       = google_compute_network.plux.id
}

resource "google_service_networking_connection" "private_services" {
  network                 = google_compute_network.plux.id
  service                 = "servicenetworking.googleapis.com"
  reserved_peering_ranges = [google_compute_global_address.private_services.name]
}

# ── Keys ─────────────────────────────────────────────────────────────────────

resource "google_kms_key_ring" "plux" {
  name     = var.name
  project  = var.project_id
  location = var.region
}

# The data key encrypts the cluster secrets, the database, the bucket and the
# cache. It rotates every 90 days and is never destroyed by this module.
resource "google_kms_crypto_key" "data" {
  name            = "${var.name}-data"
  key_ring        = google_kms_key_ring.plux.id
  purpose         = "ENCRYPT_DECRYPT"
  rotation_period = "7776000s"

  version_template {
    algorithm        = "GOOGLE_SYMMETRIC_ENCRYPTION"
    protection_level = var.kms_protection_level
  }

  lifecycle {
    prevent_destroy = true
  }
}

# The signing key never leaves Cloud KMS: the server asks KMS to sign and holds
# no private key material.
resource "google_kms_crypto_key" "signing" {
  name     = "${var.name}-signing"
  key_ring = google_kms_key_ring.plux.id
  purpose  = "ASYMMETRIC_SIGN"

  version_template {
    algorithm        = var.signing_algorithm
    protection_level = var.kms_protection_level
  }

  lifecycle {
    prevent_destroy = true
  }
}

# The Cloud Storage service agent must use the data key to encrypt the bucket.
data "google_storage_project_service_account" "gcs" {
  project = var.project_id
}

resource "google_kms_crypto_key_iam_member" "gcs_data_key" {
  crypto_key_id = google_kms_crypto_key.data.id
  role          = "roles/cloudkms.cryptoKeyEncrypterDecrypter"
  member        = "serviceAccount:${data.google_storage_project_service_account.gcs.email_address}"
}

# ── Kubernetes cluster ───────────────────────────────────────────────────────

# The nodes' identity: it can write logs and metrics and read images, and
# nothing else. The default compute service account is not used.
resource "google_service_account" "nodes" {
  account_id   = "${var.name}-nodes"
  project      = var.project_id
  display_name = "Plux GKE nodes"
}

resource "google_project_iam_member" "nodes" {
  for_each = toset([
    "roles/logging.logWriter",
    "roles/monitoring.metricWriter",
    "roles/artifactregistry.reader",
  ])
  project = var.project_id
  role    = each.value
  member  = "serviceAccount:${google_service_account.nodes.email}"
}

resource "google_container_cluster" "plux" {
  name     = var.name
  project  = var.project_id
  location = var.region

  network                  = google_compute_network.plux.id
  subnetwork               = google_compute_subnetwork.cluster.id
  remove_default_node_pool = true
  initial_node_count       = 1
  deletion_protection      = true
  resource_labels          = local.labels

  ip_allocation_policy {
    cluster_secondary_range_name  = "pods"
    services_secondary_range_name = "services"
  }

  # The control plane has a private endpoint only. Reaching it from outside
  # the subnet needs an authorized CIDR.
  private_cluster_config {
    enable_private_nodes    = true
    enable_private_endpoint = true
    master_ipv4_cidr_block  = var.master_cidr
  }

  master_authorized_networks_config {
    dynamic "cidr_blocks" {
      for_each = var.authorized_cidrs
      content {
        cidr_block = cidr_blocks.value
      }
    }
  }

  # Dataplane V2 enforces the chart's NetworkPolicies.
  datapath_provider = "ADVANCED_DATAPATH"

  database_encryption {
    state    = "ENCRYPTED"
    key_name = google_kms_crypto_key.data.id
  }

  workload_identity_config {
    workload_pool = "${var.project_id}.svc.id.goog"
  }

  release_channel {
    channel = var.release_channel
  }
}

resource "google_container_node_pool" "system" {
  name           = "system"
  project        = var.project_id
  location       = var.region
  cluster        = google_container_cluster.plux.name
  node_locations = var.zones

  autoscaling {
    min_node_count = var.node_min_count
    max_node_count = var.node_max_count
  }

  management {
    auto_repair  = true
    auto_upgrade = true
  }

  upgrade_settings {
    max_surge       = 1
    max_unavailable = 0
  }

  node_config {
    machine_type    = var.node_machine_type
    service_account = google_service_account.nodes.email
    oauth_scopes    = ["https://www.googleapis.com/auth/cloud-platform"]
    image_type      = "COS_CONTAINERD"
    metadata        = { "disable-legacy-endpoints" = "true" }

    workload_metadata_config {
      mode = "GKE_METADATA"
    }

    shielded_instance_config {
      enable_secure_boot          = true
      enable_integrity_monitoring = true
    }
  }
}

# ── Database (managed mode only) ─────────────────────────────────────────────

# Private IP only, TLS required, IAM database authentication on, and a
# deletion lock. Its password is not set here: the operator creates the
# database users.
resource "google_sql_database_instance" "plux" {
  count = local.managed_postgres ? 1 : 0

  name                = var.name
  project             = var.project_id
  region              = var.region
  database_version    = var.postgresql_version
  deletion_protection = true
  encryption_key_name = google_kms_crypto_key.data.id

  settings {
    tier              = var.postgresql_tier
    availability_type = "REGIONAL"
    disk_type         = "PD_SSD"
    disk_size         = var.postgresql_storage_gb
    disk_autoresize   = true
    user_labels       = local.labels

    ip_configuration {
      ipv4_enabled    = false
      private_network = google_compute_network.plux.id
      ssl_mode        = "ENCRYPTED_ONLY"
    }

    backup_configuration {
      enabled                        = true
      point_in_time_recovery_enabled = true
      transaction_log_retention_days = 7

      backup_retention_settings {
        retained_backups = var.postgresql_backup_retention_days
      }
    }

    database_flags {
      name  = "cloudsql.iam_authentication"
      value = "on"
    }
  }

  depends_on = [google_service_networking_connection.private_services]
}

# ── Object storage ───────────────────────────────────────────────────────────

resource "google_storage_bucket" "objects" {
  name                        = var.bucket_name
  project                     = var.project_id
  location                    = var.region
  storage_class               = "STANDARD"
  uniform_bucket_level_access = true
  public_access_prevention    = "enforced"
  force_destroy               = false
  labels                      = local.labels

  versioning {
    enabled = true
  }

  encryption {
    default_kms_key_name = google_kms_crypto_key.data.id
  }

  depends_on = [google_kms_crypto_key_iam_member.gcs_data_key]
}

# ── Cache ────────────────────────────────────────────────────────────────────

# Redis-compatible Memorystore: a standard high-availability tier across two
# zones, TLS with the server certificate, AUTH on, and a private IP only.
resource "google_redis_instance" "plux" {
  name                    = var.name
  project                 = var.project_id
  region                  = var.region
  location_id             = var.zones[0]
  alternative_location_id = var.zones[1]
  tier                    = "STANDARD_HA"
  memory_size_gb          = var.cache_memory_gb
  redis_version           = "REDIS_7_2"
  authorized_network      = google_compute_network.plux.id
  connect_mode            = "PRIVATE_SERVICE_ACCESS"
  auth_enabled            = true
  transit_encryption_mode = "SERVER_AUTHENTICATION"
  customer_managed_key    = google_kms_crypto_key.data.id
  labels                  = local.labels

  depends_on = [google_service_networking_connection.private_services]
}
