# Network module - simulates VPC and networking setup

variable "environment" {
  description = "Environment name"
  type        = string
  default     = "dev"
}

# Quick network ID generation
resource "random_id" "vpc_id" {
  byte_length = 4
}

resource "random_id" "subnet_1" {
  byte_length = 4
}

resource "random_id" "subnet_2" {
  byte_length = 4
}

resource "random_id" "subnet_3" {
  byte_length = 4
}

# VPC creation (moderate timing)
resource "null_resource" "vpc_create" {
  provisioner "local-exec" {
    command = "sleep 7 && echo 'VPC ${random_id.vpc_id.hex} created'"
  }
  
  triggers = {
    vpc_id = random_id.vpc_id.hex
    env    = var.environment
  }
}

# Subnet creation (parallel but staggered)
resource "null_resource" "subnet_1_create" {
  provisioner "local-exec" {
    command = "sleep 5 && echo 'Subnet 1 created'"
  }
  
  depends_on = [null_resource.vpc_create]
}

resource "null_resource" "subnet_2_create" {
  provisioner "local-exec" {
    command = "sleep 8 && echo 'Subnet 2 created'" 
  }
  
  depends_on = [null_resource.vpc_create]
}

resource "null_resource" "subnet_3_create" {
  provisioner "local-exec" {
    command = "sleep 6 && echo 'Subnet 3 created'"
  }
  
  depends_on = [null_resource.vpc_create]
}

# Internet gateway (depends on VPC)
resource "null_resource" "igw_create" {
  provisioner "local-exec" {
    command = "sleep 4 && echo 'Internet gateway created'"
  }
  
  depends_on = [null_resource.vpc_create]
}

# Route table setup (slow operation)
resource "null_resource" "route_table" {
  provisioner "local-exec" {
    command = "sleep 10 && echo 'Route table configured'"
  }
  
  depends_on = [
    null_resource.subnet_1_create,
    null_resource.subnet_2_create, 
    null_resource.subnet_3_create,
    null_resource.igw_create
  ]
}

# Security group (final step)
resource "null_resource" "security_group" {
  provisioner "local-exec" {
    command = "sleep 3 && echo 'Security group created'"
  }
  
  depends_on = [null_resource.route_table]
}

output "vpc_id" {
  value = "vpc-${random_id.vpc_id.hex}"
}

output "subnet_ids" {
  value = [
    "subnet-${random_id.subnet_1.hex}",
    "subnet-${random_id.subnet_2.hex}",
    "subnet-${random_id.subnet_3.hex}"
  ]
  depends_on = [
    null_resource.subnet_1_create,
    null_resource.subnet_2_create,
    null_resource.subnet_3_create
  ]
}

output "ready" {
  value = true
  depends_on = [null_resource.security_group]
}