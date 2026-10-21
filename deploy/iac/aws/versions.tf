# SPDX-FileCopyrightText: 2026 Plux contributors
# SPDX-License-Identifier: Apache-2.0

# Provider requirements of the AWS reference module (DEP-050). The module
# configures no provider: the root module that calls it does, so that no
# credential or region is written into this code.
terraform {
  required_version = ">= 1.8.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.80"
    }
  }
}
