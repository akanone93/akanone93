# SPDX-FileCopyrightText: 2026 Plux contributors
# SPDX-License-Identifier: Apache-2.0

# AWS reference infrastructure for a Plux installation (DEP-050). Every
# resource is private: the cluster endpoint, the database, the cache and the
# bucket have no public path, and every store is encrypted with a customer
# managed KMS key. Nothing here holds a credential: the database password is
# managed by Secrets Manager and the cache password comes from a variable.

locals {
  tags             = merge(var.tags, { "plux:module" = "deploy/iac/aws" })
  managed_postgres = var.postgresql_mode == "managed"
}

# ── Networking ───────────────────────────────────────────────────────────────

resource "aws_vpc" "plux" {
  cidr_block           = var.vpc_cidr
  enable_dns_support   = true
  enable_dns_hostnames = true
  tags                 = merge(local.tags, { Name = var.name })
}

# Private subnets only: no route leads to an internet gateway. Images and
# the registry are reached through VPC endpoints or an egress path that the
# operator adds (deploy/iac/README.md, "Networking").
resource "aws_subnet" "private" {
  count                   = length(var.availability_zones)
  vpc_id                  = aws_vpc.plux.id
  cidr_block              = cidrsubnet(var.vpc_cidr, 4, count.index)
  availability_zone       = var.availability_zones[count.index]
  map_public_ip_on_launch = false

  tags = merge(local.tags, {
    Name                              = "${var.name}-private-${count.index}"
    "kubernetes.io/role/internal-elb" = "1"
  })
}

resource "aws_db_subnet_group" "plux" {
  count      = local.managed_postgres ? 1 : 0
  name       = var.name
  subnet_ids = aws_subnet.private[*].id
  tags       = local.tags
}

resource "aws_elasticache_subnet_group" "plux" {
  name       = var.name
  subnet_ids = aws_subnet.private[*].id
}

# A security group created without egress rules has none: Terraform removes
# the default allow-all egress rule of AWS.
resource "aws_security_group" "database" {
  count       = local.managed_postgres ? 1 : 0
  name        = "${var.name}-database"
  description = "PostgreSQL, reachable from the VPC only"
  vpc_id      = aws_vpc.plux.id
  tags        = local.tags
}

resource "aws_vpc_security_group_ingress_rule" "database" {
  count             = local.managed_postgres ? 1 : 0
  security_group_id = aws_security_group.database[0].id
  cidr_ipv4         = var.vpc_cidr
  from_port         = 5432
  to_port           = 5432
  ip_protocol       = "tcp"
}

resource "aws_security_group" "cache" {
  name        = "${var.name}-cache"
  description = "Valkey, reachable from the VPC only"
  vpc_id      = aws_vpc.plux.id
  tags        = local.tags
}

resource "aws_vpc_security_group_ingress_rule" "cache" {
  security_group_id = aws_security_group.cache.id
  cidr_ipv4         = var.vpc_cidr
  from_port         = 6379
  to_port           = 6379
  ip_protocol       = "tcp"
}

# ── Keys ─────────────────────────────────────────────────────────────────────

# The data key encrypts the cluster secrets, the database, the bucket and the
# cache. Rotation is automatic and yearly.
resource "aws_kms_key" "data" {
  description             = "${var.name}: encryption of cluster secrets, database, bucket and cache"
  enable_key_rotation     = true
  deletion_window_in_days = var.kms_deletion_window_days
  tags                    = local.tags
}

resource "aws_kms_alias" "data" {
  name          = "alias/${var.name}-data"
  target_key_id = aws_kms_key.data.key_id
}

# The signing key never leaves KMS: the server asks KMS to sign and holds
# no private key material.
resource "aws_kms_key" "signing" {
  description              = "${var.name}: asymmetric key that signs bundles"
  key_usage                = "SIGN_VERIFY"
  customer_master_key_spec = var.signing_key_spec
  deletion_window_in_days  = var.kms_deletion_window_days
  tags                     = local.tags
}

resource "aws_kms_alias" "signing" {
  name          = "alias/${var.name}-signing"
  target_key_id = aws_kms_key.signing.key_id
}

# ── Kubernetes cluster ───────────────────────────────────────────────────────

resource "aws_iam_role" "cluster" {
  name = "${var.name}-cluster"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "eks.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })
  tags = local.tags
}

resource "aws_iam_role_policy_attachment" "cluster" {
  role       = aws_iam_role.cluster.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonEKSClusterPolicy"
}

# The cluster role uses the data key to encrypt Kubernetes secrets.
resource "aws_iam_role_policy" "cluster_kms" {
  name = "secrets-encryption"
  role = aws_iam_role.cluster.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["kms:CreateGrant", "kms:DescribeKey"]
      Resource = aws_kms_key.data.arn
    }]
  })
}

resource "aws_eks_cluster" "plux" {
  name     = var.name
  version  = var.kubernetes_version
  role_arn = aws_iam_role.cluster.arn

  vpc_config {
    subnet_ids              = aws_subnet.private[*].id
    endpoint_private_access = true
    endpoint_public_access  = false
  }

  access_config {
    authentication_mode = "API"
  }

  encryption_config {
    resources = ["secrets"]
    provider {
      key_arn = aws_kms_key.data.arn
    }
  }

  enabled_cluster_log_types = ["api", "audit", "authenticator"]
  tags                      = local.tags

  depends_on = [
    aws_iam_role_policy_attachment.cluster,
    aws_iam_role_policy.cluster_kms,
  ]
}

# The chart's NetworkPolicies are enforced only when the VPC CNI enforces
# them, so the add-on is set to enforce them.
resource "aws_eks_addon" "vpc_cni" {
  cluster_name                = aws_eks_cluster.plux.name
  addon_name                  = "vpc-cni"
  configuration_values        = jsonencode({ enableNetworkPolicy = "true" })
  resolve_conflicts_on_create = "OVERWRITE"
  resolve_conflicts_on_update = "OVERWRITE"
  tags                        = local.tags
}

resource "aws_iam_role" "node" {
  name = "${var.name}-node"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "ec2.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })
  tags = local.tags
}

resource "aws_iam_role_policy_attachment" "node" {
  for_each = toset([
    "arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy",
    "arn:aws:iam::aws:policy/AmazonEKS_CNI_Policy",
    "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly",
  ])
  role       = aws_iam_role.node.name
  policy_arn = each.value
}

# Encrypted root volumes with IMDSv2 required: the launch template is the only
# place EKS managed node groups take these settings from.
resource "aws_launch_template" "node" {
  name_prefix   = "${var.name}-node-"
  instance_type = var.node_instance_type
  tags          = local.tags

  metadata_options {
    http_endpoint               = "enabled"
    http_tokens                 = "required"
    http_put_response_hop_limit = 2
  }

  block_device_mappings {
    device_name = "/dev/xvda"
    ebs {
      volume_size = 50
      volume_type = "gp3"
      encrypted   = true
      kms_key_id  = aws_kms_key.data.arn
    }
  }
}

resource "aws_eks_node_group" "plux" {
  cluster_name    = aws_eks_cluster.plux.name
  node_group_name = var.name
  node_role_arn   = aws_iam_role.node.arn
  subnet_ids      = aws_subnet.private[*].id

  launch_template {
    id      = aws_launch_template.node.id
    version = aws_launch_template.node.latest_version
  }

  scaling_config {
    min_size     = var.node_min_size
    max_size     = var.node_max_size
    desired_size = var.node_desired_size
  }

  update_config {
    max_unavailable = 1
  }

  tags = local.tags

  depends_on = [
    aws_iam_role_policy_attachment.node,
    aws_eks_addon.vpc_cni,
  ]
}

# ── Database (managed mode only) ─────────────────────────────────────────────

resource "aws_db_instance" "plux" {
  count = local.managed_postgres ? 1 : 0

  identifier                          = var.name
  engine                              = "postgres"
  engine_version                      = var.postgresql_engine_version
  instance_class                      = var.postgresql_instance_class
  allocated_storage                   = var.postgresql_storage_gb
  storage_type                        = "gp3"
  storage_encrypted                   = true
  kms_key_id                          = aws_kms_key.data.arn
  db_name                             = "plux"
  username                            = "plux"
  manage_master_user_password         = true
  db_subnet_group_name                = aws_db_subnet_group.plux[0].name
  vpc_security_group_ids              = [aws_security_group.database[0].id]
  publicly_accessible                 = false
  multi_az                            = true
  backup_retention_period             = var.postgresql_backup_retention_days
  copy_tags_to_snapshot               = true
  auto_minor_version_upgrade          = true
  iam_database_authentication_enabled = true
  deletion_protection                 = true
  skip_final_snapshot                 = false
  final_snapshot_identifier           = "${var.name}-final"
  tags                                = local.tags
}

# ── Object storage ───────────────────────────────────────────────────────────

resource "aws_s3_bucket" "objects" {
  bucket        = var.bucket_name
  force_destroy = false
  tags          = local.tags
}

resource "aws_s3_bucket_versioning" "objects" {
  bucket = aws_s3_bucket.objects.id

  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_ownership_controls" "objects" {
  bucket = aws_s3_bucket.objects.id

  rule {
    object_ownership = "BucketOwnerEnforced"
  }
}

resource "aws_s3_bucket_public_access_block" "objects" {
  bucket                  = aws_s3_bucket.objects.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_server_side_encryption_configuration" "objects" {
  bucket = aws_s3_bucket.objects.id

  rule {
    bucket_key_enabled = true
    apply_server_side_encryption_by_default {
      sse_algorithm     = "aws:kms"
      kms_master_key_id = aws_kms_key.data.arn
    }
  }
}

# Refuses every request that does not use TLS.
resource "aws_s3_bucket_policy" "objects" {
  bucket = aws_s3_bucket.objects.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Sid       = "DenyInsecureTransport"
      Effect    = "Deny"
      Principal = "*"
      Action    = "s3:*"
      Resource  = [aws_s3_bucket.objects.arn, "${aws_s3_bucket.objects.arn}/*"]
      Condition = {
        Bool = { "aws:SecureTransport" = "false" }
      }
    }]
  })

  depends_on = [aws_s3_bucket_public_access_block.objects]
}

# ── Cache ────────────────────────────────────────────────────────────────────

# Encrypted at rest and in transit, with an automatic failover across zones.
resource "aws_elasticache_replication_group" "plux" {
  replication_group_id       = var.name
  description                = "${var.name}: sessions, rate limits and delivery queues"
  engine                     = "valkey"
  engine_version             = var.valkey_engine_version
  node_type                  = var.valkey_node_type
  num_cache_clusters         = var.valkey_replicas
  port                       = 6379
  automatic_failover_enabled = true
  multi_az_enabled           = true
  subnet_group_name          = aws_elasticache_subnet_group.plux.name
  security_group_ids         = [aws_security_group.cache.id]
  at_rest_encryption_enabled = true
  kms_key_id                 = aws_kms_key.data.arn
  transit_encryption_enabled = true
  auth_token                 = var.valkey_auth_token
  snapshot_retention_limit   = 7
  apply_immediately          = false
  tags                       = local.tags
}
