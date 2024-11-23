
module "test" {
  source  = sensitive("hostname/namespace/name/system") # ERROR: Sensitive value not allowed
  version = sensitive("1.0.0") # ERROR: Invalid version constraint
}
