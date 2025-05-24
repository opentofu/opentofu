# This is a test fixture for testing output sensitivity changes.

# This output starts non-sensitive, and the test will change it to sensitive
output "sensitive_after" {
  value = "after"
  sensitive = true
}

# This output starts sensitive, and the test will change it to non-sensitive
output "sensitive_before" {
  value = "before"
  sensitive = false
}

# Regular output unchanged, to verify it doesn't show up in the plan
output "unchanged_insensitive" {
  value = "unchanged"
  sensitive = false
}

# This output's value changes, to verify it shows up in the plan
output "changed_insensitive" {
  value = "changed_but_always_insensitive"
  sensitive = false
}

output "changed_sensitive" {
  value = "changed_and_always_sensitive"
  sensitive = true
}

# This output is added, to verify it shows up in the plan
output "added_insensitive" {
  value = "added_but_always_insensitive"
  sensitive = false
}
