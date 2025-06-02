# Compute submodule - handles application compute resources

variable "app_id" {
  description = "Application ID from parent module"
  type        = string
}

variable "app_version" {
  description = "Application version from parent module"
  type        = string
}

variable "instance_count" {
  description = "Number of compute instances"
  type        = number
  default     = 3
}

# Compute cluster configuration
resource "random_id" "cluster_id" {
  byte_length = 4
}

resource "null_resource" "compute_cluster" {
  provisioner "local-exec" {
    command = "sleep 6 && echo 'Compute cluster ${random_id.cluster_id.hex} created'"
  }
  
  triggers = {
    app_id      = var.app_id
    app_version = var.app_version
  }
}

# Individual compute instances
resource "null_resource" "compute_instance" {
  count = var.instance_count
  
  provisioner "local-exec" {
    command = "sleep 4 && echo 'Compute instance ${count.index + 1} started'"
  }
  
  depends_on = [null_resource.compute_cluster]
  
  triggers = {
    cluster_id = random_id.cluster_id.hex
    instance_index = count.index
  }
}

# Instance scaling configuration
resource "null_resource" "auto_scaling" {
  provisioner "local-exec" {
    command = "sleep 3 && echo 'Auto-scaling configured for ${var.instance_count} instances'"
  }
  
  depends_on = [null_resource.compute_instance]
}

output "cluster_id" {
  value = random_id.cluster_id.hex
  description = "Unique identifier for the compute cluster"
}

output "instance_ids" {
  value = [for i in range(var.instance_count) : "instance-${i+1}-${random_id.cluster_id.hex}"]
  description = "List of compute instance identifiers"
}

output "cluster_endpoint" {
  value = "compute-${random_id.cluster_id.hex}.internal"
  description = "Internal endpoint for the compute cluster"
  depends_on = [null_resource.auto_scaling]
}