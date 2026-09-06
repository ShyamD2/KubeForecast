terraform {
  required_version = ">= 1.5.0"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

data "aws_caller_identity" "current" {}

# IRSA Assume Role Policy based on EKS OIDC Provider
data "aws_iam_policy_document" "irsa_assume" {
  statement {
    actions = ["sts:AssumeRoleWithWebIdentity"]
    effect  = "Allow"

    principals {
      type        = "Federated"
      identifiers = [var.oidc_provider_arn]
    }

    condition {
      test     = "StringEquals"
      variable = "${replace(var.oidc_provider_url, "https://", "")}:sub"
      values   = ["system:serviceaccount:${var.namespace}:${var.service_account_name}"]
    }

    condition {
      test     = "StringEquals"
      variable = "${replace(var.oidc_provider_url, "https://", "")}:aud"
      values   = ["sts.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "irsa" {
  name               = "${var.environment}-predictive-scheduler-irsa"
  assume_role_policy = data.aws_iam_policy_document.irsa_assume.json

  tags = var.tags
}

# Least-privilege S3 snapshot policy
resource "aws_iam_policy" "s3_access" {
  name        = "${var.environment}-predictive-scheduler-s3-policy"
  description = "Least privilege S3 access for cluster snapshots and waterline prediction history"

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid    = "ListBucketSnapshots"
        Effect = "Allow"
        Action = [
          "s3:ListBucket"
        ]
        Resource = [var.s3_bucket_arn]
        Condition = {
          StringLike = {
            "s3:prefix" = ["kubeforecast/*", "snapshots/*", "reports/*"]
          }
        }
      },
      {
        Sid    = "ReadWriteSnapshots"
        Effect = "Allow"
        Action = [
          "s3:GetObject",
          "s3:PutObject"
        ]
        Resource = ["${var.s3_bucket_arn}/*"]
      }
    ]
  })
}

resource "aws_iam_role_policy_attachment" "irsa_attach" {
  role       = aws_iam_role.irsa.name
  policy_arn = aws_iam_policy.s3_access.arn
}
