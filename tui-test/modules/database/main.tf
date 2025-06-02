# Database module - simulates database setup with varying timing

variable "db_name" {
  description = "Database name"
  type        = string
}

# Quick setup resources
resource "random_id" "db_instance_id" {
  byte_length = 8
}

resource "random_password" "db_password" {
  length  = 16
  special = true
}

# Long running database initialization
resource "null_resource" "db_create" {
  provisioner "local-exec" {
    command = "sleep 12 && echo 'Database ${var.db_name} created'"
  }
  
  triggers = {
    db_name = var.db_name
    db_id   = random_id.db_instance_id.hex
  }
}

# Database migration (depends on creation)
resource "null_resource" "db_migrate" {
  provisioner "local-exec" {
    command = "sleep 6 && echo 'Database migration completed'"
  }
  
  depends_on = [null_resource.db_create]
}

# Database backup setup
resource "null_resource" "db_backup_setup" {
  provisioner "local-exec" {
    command = "sleep 4 && echo 'Backup configured'"
  }
  
  depends_on = [null_resource.db_migrate]
}

# Very slow replication setup
resource "null_resource" "db_replication" {
  provisioner "local-exec" {
    command = "sleep 15 && echo 'Replication configured'"
  }
  
  depends_on = [null_resource.db_backup_setup]
}

output "connection_string" {
  value = "postgresql://user:${random_password.db_password.result}@db-${random_id.db_instance_id.hex}.example.com:5432/${var.db_name}"
  sensitive = true
}

output "status" {
  value = "ready"
  depends_on = [null_resource.db_replication]
}