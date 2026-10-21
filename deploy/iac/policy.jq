# SPDX-FileCopyrightText: 2026 Plux contributors
# SPDX-License-Identifier: Apache-2.0

# Secure-default rules over the planned resources of an OpenTofu plan
# (`tofu show -json <planfile>`), run by deploy/iac/policy.sh (DEP-050). Each
# rule names a resource type and a condition that every resource of that type
# must satisfy; each resource that fails it is one violation. The rules read
# only literal values, never values that are unknown until apply.

# Every planned resource, whatever module it is in.
def planned:
  [ .planned_values | .. | objects | select(has("address") and has("type") and has("values")) ];

# The planned resources of one type.
def of_type($t):
  planned[] | select(.type == $t);

# One violation per resource of $type for which ok is false. ok and msg are
# filters over that resource, so they are not bound as values.
def rule($type; ok; msg):
  of_type($type) | select((ok) | not) | "\(.address): \(msg)";

# Azure has no delete protection for a flexible server: each one needs a
# CanNotDelete management lock.
def locked_servers:
  ([ of_type("azurerm_postgresql_flexible_server") ] | length) as $servers
  | ([ of_type("azurerm_management_lock") | select(.values.lock_level == "CanNotDelete") ] | length) as $locks
  | if $servers > $locks then "azurerm_postgresql_flexible_server: a database has no CanNotDelete management lock" else empty end;

(
  # Public access: buckets, storage accounts, vaults and caches stay private.
  rule("aws_s3_bucket_public_access_block";
    .values.block_public_acls == true and .values.block_public_policy == true
    and .values.ignore_public_acls == true and .values.restrict_public_buckets == true;
    "public access to the bucket is not fully blocked"),
  rule("google_storage_bucket";
    .values.public_access_prevention == "enforced" and .values.uniform_bucket_level_access == true;
    "public access prevention or uniform bucket-level access is off"),
  rule("azurerm_storage_account";
    .values.public_network_access_enabled == false and .values.allow_nested_items_to_be_public == false;
    "the storage account is publicly reachable or allows public blobs"),
  rule("azurerm_key_vault";
    .values.public_network_access_enabled == false;
    "the Key Vault is publicly reachable"),
  rule("azurerm_redis_cache";
    .values.public_network_access_enabled == false and .values.non_ssl_port_enabled == false;
    "the cache is publicly reachable or accepts plaintext connections"),

  # Encryption at rest and in transit.
  rule("aws_s3_bucket_server_side_encryption_configuration";
    .values.rule[0].apply_server_side_encryption_by_default[0].sse_algorithm == "aws:kms";
    "the bucket is not encrypted with KMS"),
  rule("aws_db_instance";
    .values.storage_encrypted == true;
    "the database storage is not encrypted"),
  rule("aws_elasticache_replication_group";
    .values.at_rest_encryption_enabled == true and .values.transit_encryption_enabled == true;
    "the cache is not encrypted at rest and in transit"),
  rule("azurerm_storage_account";
    .values.infrastructure_encryption_enabled == true and .values.min_tls_version == "TLS1_2";
    "the storage account lacks infrastructure encryption or accepts TLS below 1.2"),
  rule("google_sql_database_instance";
    .values.settings[0].ip_configuration[0].ssl_mode == "ENCRYPTED_ONLY";
    "the database accepts unencrypted connections"),
  rule("google_redis_instance";
    .values.transit_encryption_mode == "SERVER_AUTHENTICATION" and .values.auth_enabled == true;
    "the cache has no TLS or no AUTH"),

  # Private networking: no control plane, database or cache has a public endpoint.
  rule("aws_eks_cluster";
    .values.vpc_config[0].endpoint_public_access == false
    and ((.values.encryption_config[0].resources // []) | any(. == "secrets"));
    "the API endpoint is public or Kubernetes secrets are not encrypted"),
  rule("aws_db_instance";
    .values.publicly_accessible == false;
    "the database is publicly accessible"),
  rule("google_container_cluster";
    .values.private_cluster_config[0].enable_private_nodes == true
    and .values.private_cluster_config[0].enable_private_endpoint == true;
    "the nodes or the control plane have public endpoints"),
  rule("google_sql_database_instance";
    .values.settings[0].ip_configuration[0].ipv4_enabled == false;
    "the database has a public IPv4 address"),
  rule("google_redis_instance";
    .values.connect_mode == "PRIVATE_SERVICE_ACCESS";
    "the cache is not on private service access"),
  rule("azurerm_kubernetes_cluster";
    .values.private_cluster_enabled == true;
    "the AKS API server is public"),
  rule("azurerm_postgresql_flexible_server";
    .values.public_network_access_enabled == false;
    "the flexible server is publicly reachable"),

  # Deletion protection of the database and the cluster.
  rule("aws_db_instance";
    .values.deletion_protection == true;
    "the database is not protected from deletion"),
  rule("google_sql_database_instance";
    .values.deletion_protection == true;
    "the database is not protected from deletion"),
  rule("google_container_cluster";
    .values.deletion_protection == true;
    "the cluster is not protected from deletion"),
  locked_servers
)
