aws_region    = "us-east-1"
project_name  = "custom-business-metrics"
environment   = "dev"
service_image = "public.ecr.aws/nginx/nginx:stable-alpine"

container_port = 8080
service_cpu    = 256
service_memory = 512
desired_count  = 1
