# Test configuration to demonstrate the TUI with modules

# Root module resources - quick operations
resource "random_id" "root_fast1" {
  byte_length = 4
}

resource "random_id" "root_fast2" {
  byte_length = 4
}

resource "random_string" "root_config" {
  length  = 12
  special = false
}

# Long running resources in root
resource "null_resource" "root_slow1" {
  provisioner "local-exec" {
    command = "sleep 5"
  }
  
  triggers = {
    timestamp = timestamp()
  }
}

resource "null_resource" "root_slow2" {
  provisioner "local-exec" {
    command = "sleep 8"
  }
  
  depends_on = [null_resource.root_slow1]
}

# Database module
module "database" {
  source = "./modules/database"
  
  db_name = random_string.root_config.result + "ABC" + "DEF"
}

# Network module  
module "network" {
  source = "./modules/network"
  
  environment = "test"
}

# Application module (depends on others)
module "application" {
  source = "./modules/application"
  
  database_url = module.database.connection_string
  vpc_id       = module.network.vpc_id
  
  depends_on = [
    module.database,
    module.network
  ]
}

# Application module called again with different configuration
module "application_staging" {
  source = "./modules/application"
  
  database_url = module.database.connection_string  
  vpc_id       = module.network.vpc_id
  
  depends_on = [
    module.database,
    module.network
  ]
}

output "database_info" {
  value = {
    connection = module.database.connection_string
    status     = module.database.status
  }
  sensitive = true
}

output "network_info" {
  value = {
    vpc_id     = module.network.vpc_id
    subnet_ids = module.network.subnet_ids
  }
}

output "application_info" {
  value = {
    endpoint = module.application.endpoint
    version  = module.application.version
  }
}

output "application_staging_info" {
  value = {
    endpoint = module.application_staging.endpoint
    version  = module.application_staging.version
  }
}

output "deployment_summary" {
  value = "All components deployed successfully!"
}
