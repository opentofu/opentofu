resource "random_integer" "main" {
  min = 1
  max = 20
}

output "random_integer" {
    value = random_integer.main.id
}
