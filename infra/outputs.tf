output "cluster_name" {
  description = "ECS cluster name."
  value       = aws_ecs_cluster.main.name
}

output "service_name" {
  description = "ECS service name."
  value       = aws_ecs_service.service.name
}

output "public_subnet_ids" {
  description = "Public subnet ids used by the service."
  value       = aws_subnet.public[*].id
}

output "metrics_table_name" {
  description = "DynamoDB table used for metric storage."
  value       = aws_dynamodb_table.metrics.name
}
