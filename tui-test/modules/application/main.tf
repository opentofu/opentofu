# Application module - simulates application deployment

variable "database_url" {
  description = "Database connection URL"
  type        = string
  sensitive   = true
}

variable "vpc_id" {
  description = "VPC ID for deployment"
  type        = string
}

# Application identifiers
resource "random_id" "app_id" {
  byte_length = 6
}

resource "random_string" "app_version" {
  length  = 8
  special = false
  upper   = false
}

# Application build (long process)
resource "null_resource" "app_build" {
  provisioner "local-exec" {
    command = "sleep 18 && echo 'Application build completed'"
  }
  
  triggers = {
    app_id      = random_id.app_id.hex
    app_version = random_string.app_version.result
  }
}

# Container registry push
resource "null_resource" "container_push" {
  provisioner "local-exec" {
    command = "sleep 9 && echo 'Container pushed to registry'"
  }
  
  depends_on = [null_resource.app_build]
}

# Call the compute submodule
module "compute" {
  source = "./compute"
  
  app_id      = random_id.app_id.hex
  app_version = random_string.app_version.result
  instance_count = 3
  
  depends_on = [null_resource.container_push]
}

# Load balancer setup
resource "null_resource" "load_balancer" {
  provisioner "local-exec" {
    command = "sleep 7 && echo 'Load balancer configured'"
  }
  
  triggers = {
    vpc_id = var.vpc_id
    cluster_endpoint = module.compute.cluster_endpoint
  }
}

# Application deployment (multiple instances)
resource "null_resource" "app_deploy_1" {
  provisioner "local-exec" {
    command = "sleep 12 && echo 'App instance 1 deployed on ${module.compute.cluster_endpoint}'"
  }
  
  depends_on = [
    module.compute,
    null_resource.load_balancer
  ]
}

resource "null_resource" "app_deploy_2" {
  provisioner "local-exec" {
    command = "sleep 14 && echo 'App instance 2 deployed on ${module.compute.cluster_endpoint}'"
  }
  
  depends_on = [
    module.compute,
    null_resource.load_balancer
  ]
}

resource "null_resource" "app_deploy_3" {
  provisioner "local-exec" {
    command = "sleep 11 && echo 'App instance 3 deployed on ${module.compute.cluster_endpoint}'"
  }
  
  depends_on = [
    module.compute,
    null_resource.load_balancer
  ]
}

# Health check setup
resource "null_resource" "health_check" {
  provisioner "local-exec" {
    command = "sleep 5 && echo 'Health checks configured'"
  }
  
  depends_on = [
    null_resource.app_deploy_1,
    null_resource.app_deploy_2,
    null_resource.app_deploy_3
  ]
}

# Monitoring setup (final step)
resource "null_resource" "monitoring" {
  provisioner "local-exec" {
    command = "sleep 8 && echo 'Monitoring and alerts configured'"
  }
  
  depends_on = [null_resource.health_check]
}

output "endpoint" {
  value = "https://app-${random_id.app_id.hex}.example.com"
  depends_on = [null_resource.monitoring]
}

output "version" {
  value = "v${random_string.app_version.result}"
}

output "instances" {
  value = {
    instance_1 = "running"
    instance_2 = "running" 
    instance_3 = "running"
  }
  depends_on = [null_resource.monitoring]
}

output "compute_cluster_id" {
  value = module.compute.cluster_id
  description = "ID of the compute cluster from submodule"
}

output "compute_instance_ids" {
  value = module.compute.instance_ids
  description = "List of compute instance IDs from submodule"
}