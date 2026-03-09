locals {
    my_pets = toset(["dino", "astro", "scoob"])
}

resource "test_resource" "dogs" {
    for_each = local.my_pets
}

output "flintstones" {
    value = test_resource.dogs["dino"].id 
}

output "jetsons" {
    value = test_resource.dogs["astro"].id 
}

output "shaggy" {
    value = test_resource.dogs["scoob"].id 
}
