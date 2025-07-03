output "fully_overridden" {
  value = "b_override"
  description = "b_override description"
  deprecated = "b_override deprecated"
  ephemeral = false
}

output "partially_overridden" {
  value = "b_override partial"
  deprecated = "b_override deprecated"
}
