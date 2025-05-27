output "fully_overridden" {
  value = "a_override"
  description = "a_override description"
  deprecated = "a_override deprecated"
  ephemeral = true
}

output "partially_overridden" {
  value = "a_override partial"
}
