module "submodule" {
  source = "./submodule"
}

output "changed" {
  value = "after"
}

output "sensitive_before" {
  value = "after"
  # no sensitive = true here, but the prior state is marked as sensitive in the test code
}

output "sensitive_after" {
  value = "after"

  # This one is _not_ sensitive in the prior state, but is transitioning to
  # being sensitive in our new plan.
  sensitive = true
}

output "added" { // not present in the prior state
  value = "after"
}

output "sensitive_only_before" {
  value = "before"
  # no sensitive = true here, but the prior state is marked as sensitive in the test code.
  # The value is unchanged — only the sensitivity transitions from true to false.
}

output "sensitive_only_after" {
  value = "before"

  # This one is _not_ sensitive in the prior state, but is transitioning to
  # being sensitive in our new plan. The value is unchanged.
  sensitive = true
}

output "unchanged" {
  value = "before"
}
