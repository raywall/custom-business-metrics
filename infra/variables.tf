variable "aws_region" {
  description = "AWS region used by the MVP infrastructure."
  type        = string
}

variable "project_name" {
  description = "Name used to prefix low-cost resources."
  type        = string
}

variable "environment" {
  description = "Deployment environment name."
  type        = string
}

variable "container_port" {
  description = "HTTP port exposed by service and webview containers."
  type        = number
  default     = 8080
}

variable "service_cpu" {
  description = "Fargate CPU units for the service task."
  type        = number
  default     = 256
}

variable "service_memory" {
  description = "Fargate memory in MiB for the service task."
  type        = number
  default     = 512
}

variable "desired_count" {
  description = "Desired number of service tasks."
  type        = number
  default     = 1
}

variable "service_image" {
  description = "Container image URI for the service."
  type        = string
}
