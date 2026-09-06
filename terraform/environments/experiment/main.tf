terraform {
  required_version = ">= 1.5.0"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  region = var.aws_region
}

locals {
  environment  = "experiment"
  cluster_name = "${local.environment}-predictive-eks"
  common_tags = {
    Environment = local.environment
    Project     = "Kubernetes-Predictive-Scheduler-Experiment"
    ManagedBy   = "Terraform"
  }
}

module "vpc" {
  source = "../../modules/vpc"

  environment        = local.environment
  cluster_name       = local.cluster_name
  vpc_cidr             = "10.20.0.0/16"
  public_subnet_cidrs  = ["10.20.1.0/24", "10.20.2.0/24"]
  private_subnet_cidrs = ["10.20.10.0/24", "10.20.11.0/24"]
  availability_zones   = ["${var.aws_region}a", "${var.aws_region}b"]
  tags               = local.common_tags
}

module "s3" {
  source = "../../modules/s3"

  environment   = local.environment
  force_destroy = true
  tags          = local.common_tags
}

module "ecr" {
  source = "../../modules/ecr"

  environment = local.environment
  tags        = local.common_tags
}

module "eks" {
  source = "../../modules/eks"

  cluster_name     = local.cluster_name
  subnet_ids       = module.vpc.private_subnet_ids
  instance_types   = ["m5.large"]
  desired_capacity = 6
  min_capacity     = 4
  max_capacity     = 16
  tags             = local.common_tags
}

module "iam" {
  source = "../../modules/iam"

  environment          = local.environment
  oidc_provider_arn    = module.eks.oidc_provider_arn
  oidc_provider_url    = module.eks.oidc_provider_url
  s3_bucket_arn        = module.s3.bucket_arn
  namespace            = "predictive-scheduler"
  service_account_name = "predictive-scheduler-sa"
  tags                 = local.common_tags
}
