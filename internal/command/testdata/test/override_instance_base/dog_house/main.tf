locals {
    my_pets = toset(["dino", "astro", "scoob"])
}

resource "test_resource" "dogs" {
    for_each = local.my_pets
}

data "test_data_source" "water_bowl" {
    for_each = local.my_pets
    id = "whatever"
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

output "stone_bowl" {
    value = "${test_resource.dogs["dino"].id} drinks from a ${data.test_data_source.water_bowl["dino"].value}"
}

output "space_bowl" {
    value = "${test_resource.dogs["astro"].id} drinks from a ${data.test_data_source.water_bowl["astro"].value}"
}

output "spooky_bowl" {
    value = "${test_resource.dogs["scoob"].id} drinks from a ${data.test_data_source.water_bowl["scoob"].value}"
}
