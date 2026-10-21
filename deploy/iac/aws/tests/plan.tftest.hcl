# SPDX-FileCopyrightText: 2026 Plux contributors
# SPDX-License-Identifier: Apache-2.0

# Plan-level tests of the AWS module with a mock provider: no account, no
# network. They prove the planned resources and the secure defaults (DEP-050);
# they cannot prove how AWS behaves.

mock_provider "aws" {}

variables {
  name               = "plux-test"
  region             = "eu-west-1"
  vpc_cidr           = "10.20.0.0/16"
  availability_zones = ["eu-west-1a", "eu-west-1b", "eu-west-1c"]
  kubernetes_version = "1.34"
  bucket_name        = "plux-test-objects"
  valkey_auth_token  = "test-only-token-0123456789"
}

# Verifies: DEP-050.
run "DEP-050 buckets are versioned, encrypted and private" {
  command = plan

  assert {
    condition     = aws_s3_bucket_versioning.objects.versioning_configuration[0].status == "Enabled"
    error_message = "the bucket must be versioned"
  }

  assert {
    condition = alltrue([
      aws_s3_bucket_public_access_block.objects.block_public_acls,
      aws_s3_bucket_public_access_block.objects.block_public_policy,
      aws_s3_bucket_public_access_block.objects.ignore_public_acls,
      aws_s3_bucket_public_access_block.objects.restrict_public_buckets,
    ])
    error_message = "every public access block must be on"
  }

  assert {
    condition     = aws_s3_bucket_server_side_encryption_configuration.objects.rule[0].apply_server_side_encryption_by_default[0].sse_algorithm == "aws:kms"
    error_message = "the bucket must be encrypted with KMS"
  }

  assert {
    condition     = aws_s3_bucket_ownership_controls.objects.rule[0].object_ownership == "BucketOwnerEnforced"
    error_message = "ACLs must be disabled"
  }
}

# Verifies: DEP-050.
run "DEP-050 cluster endpoint is private and secrets are encrypted" {
  command = plan

  assert {
    condition     = aws_eks_cluster.plux.vpc_config[0].endpoint_public_access == false
    error_message = "the API endpoint must not be public"
  }

  assert {
    condition     = aws_eks_cluster.plux.vpc_config[0].endpoint_private_access == true
    error_message = "the API endpoint must be reachable from the VPC"
  }

  assert {
    condition     = contains(aws_eks_cluster.plux.encryption_config[0].resources, "secrets")
    error_message = "Kubernetes secrets must be encrypted with KMS"
  }

  assert {
    condition     = aws_eks_addon.vpc_cni.configuration_values == jsonencode({ enableNetworkPolicy = "true" })
    error_message = "the VPC CNI must enforce NetworkPolicies"
  }

  assert {
    condition     = aws_launch_template.node.metadata_options[0].http_tokens == "required"
    error_message = "nodes must require IMDSv2"
  }

  assert {
    condition     = aws_launch_template.node.block_device_mappings[0].ebs[0].encrypted == true
    error_message = "node volumes must be encrypted"
  }
}

# Verifies: DEP-050.
run "DEP-050 managed database is private, encrypted and protected" {
  command = plan

  assert {
    condition     = aws_db_instance.plux[0].publicly_accessible == false
    error_message = "the database must not be publicly accessible"
  }

  assert {
    condition     = aws_db_instance.plux[0].storage_encrypted == true
    error_message = "the database storage must be encrypted"
  }

  assert {
    condition     = aws_db_instance.plux[0].deletion_protection == true
    error_message = "the database must be protected from deletion"
  }

  assert {
    condition     = aws_db_instance.plux[0].manage_master_user_password == true
    error_message = "the master password must be managed by Secrets Manager, not written in code"
  }
}

# Verifies: DEP-050.
run "DEP-050 cache is encrypted in transit and at rest" {
  command = plan

  assert {
    condition     = aws_elasticache_replication_group.plux.transit_encryption_enabled == true
    error_message = "the cache must use TLS"
  }

  assert {
    condition     = aws_elasticache_replication_group.plux.at_rest_encryption_enabled == true
    error_message = "the cache must be encrypted at rest"
  }

  assert {
    condition     = aws_elasticache_replication_group.plux.engine == "valkey"
    error_message = "the cache engine must be Valkey"
  }
}

# Verifies: DEP-050.
run "DEP-050 signing key is an asymmetric key that only signs and verifies" {
  command = plan

  assert {
    condition     = aws_kms_key.signing.key_usage == "SIGN_VERIFY"
    error_message = "the signing key must be a SIGN_VERIFY key"
  }

  assert {
    condition     = aws_kms_key.data.enable_key_rotation == true
    error_message = "the data key must rotate automatically"
  }
}

# Verifies: DEP-050.
run "DEP-050 cnpg mode creates no managed database" {
  command = plan

  variables {
    postgresql_mode = "cnpg"
  }

  assert {
    condition     = length(aws_db_instance.plux) == 0
    error_message = "cnpg mode must not create an RDS instance"
  }

  assert {
    condition     = output.helm_values.postgresql.mode == "cnpg"
    error_message = "cnpg mode must point the chart at CloudNativePG"
  }
}

# Verifies: DEP-050.
run "DEP-050 rejects a public database mode" {
  command = plan

  variables {
    postgresql_mode = "public"
  }

  expect_failures = [var.postgresql_mode]
}

# Verifies: DEP-050.
run "DEP-050 rejects a cache password that is too short" {
  command = plan

  variables {
    valkey_auth_token = "short"
  }

  expect_failures = [var.valkey_auth_token]
}

# Verifies: DEP-050.
run "DEP-050 rejects a single availability zone" {
  command = plan

  variables {
    availability_zones = ["eu-west-1a"]
  }

  expect_failures = [var.availability_zones]
}

# Verifies: DEP-050.
run "DEP-050 rejects a bucket name that is not a valid S3 name" {
  command = plan

  variables {
    bucket_name = "Plux_Objects"
  }

  expect_failures = [var.bucket_name]
}

# Verifies: DEP-050.
run "DEP-050 helm values carry no secret" {
  command = plan

  assert {
    condition     = output.helm_values.objectStorage.mode == "external" && output.helm_values.objectStorage.bucket == "plux-test-objects"
    error_message = "the chart must use the bucket of this module"
  }

  assert {
    condition     = !contains(keys(output.helm_values.postgresql), "password")
    error_message = "the Helm values must not carry a password"
  }
}
