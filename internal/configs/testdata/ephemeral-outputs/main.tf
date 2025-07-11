output "test" {
  value = "explicit"
  ephemeral = true // This needs to fail when loaded as root module
}