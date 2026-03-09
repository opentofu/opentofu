locals {
    my_pets = toset(["pet_a", "pet_b", "pet_c"])
    universes = toset(["alpha", "67", "413"])
}

resource "test_resource" "cats" {
    for_each = local.my_pets
}

output "pet_a_greeting" {
    value = "Hi ${test_resource.cats["pet_a"].id}"
}
output "pet_b_greeting" {
    value = "Hello ${test_resource.cats["pet_b"].id}"
}
output "pet_c_greeting" {
    value = "Sup ${test_resource.cats["pet_c"].id}"
}

module dog_houses {
    source = "./dog_house"
    for_each = local.universes
}

output "dino_a_greeting" {
    value = "Hey there, ${module.dog_houses["alpha"].flintstones}"
}

output "dino_b_greeting" {
    value = "Ooga Booga, ${module.dog_houses["67"].flintstones}"
}

output "dino_c_greeting" {
    value = "*random grunts*, ${module.dog_houses["413"].flintstones}"
}

output "astro_a_greeting" {
    value = "Greetings, ${module.dog_houses["alpha"].jetsons}"
}

output "astro_b_greeting" {
    value = "Avast, ${module.dog_houses["67"].jetsons}"
}

output "astro_c_greeting" {
    value = "*random beeps*, ${module.dog_houses["413"].jetsons}"
}

output "scoob_a_greeting" {
    value = "Zoinks, ${module.dog_houses["alpha"].shaggy}"
}

output "scoob_b_greeting" {
    value = "Jeepers, ${module.dog_houses["67"].shaggy}"
}

output "scoob_c_greeting" {
    value = "Jinkies, ${module.dog_houses["413"].shaggy}"
}
