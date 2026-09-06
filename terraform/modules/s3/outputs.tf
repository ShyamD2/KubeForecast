output "bucket_id" {
  value       = aws_s3_bucket.snapshots.id
  description = "The ID/name of the S3 bucket"
}

output "bucket_arn" {
  value       = aws_s3_bucket.snapshots.arn
  description = "The ARN of the S3 bucket"
}
