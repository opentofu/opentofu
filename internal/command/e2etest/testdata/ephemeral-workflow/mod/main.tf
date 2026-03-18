variable "in" {
  type        = string
  description = "Variable that is marked as ephemeral and doesn't matter what value is given in, ephemeral or not, the value evaluated for this variable will be marked as ephemeral"
  ephemeral   = true
}

variable "from_resource" {
  type = string
}
// NOTE: During the reworking of the ephemeral resources, realised that this test does not cover the situation
// where an ephemeral resource is deferred and it is referenced in other constructs to see that deferral
// has indeed side effects.
ephemeral "simple_resource" "deferred_ephemeral" {
  value = var.from_resource // This will create a dependency on the managed resource that will defer the opening
}

data "simple_resource" "deferred_data" {
  // NOTE: "hardcoded" value here because we want this data source to be deferred for the apply phase but through
  // indirect dependencies: data -(precondition)-> ephemeral -> managed resource
  value    = "hardcoded"
  lifecycle {
    precondition {
      condition     = !tofu.applying || ephemeral.simple_resource.deferred_ephemeral.value != null
      error_message = "test message"
    }
  }
}

output "out1" {
  value     = var.in
  // NOTE: because this output gets its value from referencing an ephemeral variable,
  // it needs to be configured as ephemeral too.
  ephemeral = true
}
