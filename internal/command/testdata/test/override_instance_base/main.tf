locals {
    my_pets = toset(["pet_a", "pet_b", "pet_c"])
    universes = toset(["a", "b", "c"])
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

module concrete_dog_house {
    source = "./dog_house"
}

output "dino_a_greeting" {
    value = "Hey there, ${module.dog_houses["a"].flintstones}"
}

output "dino_b_greeting" {
    value = "Ooga Booga, ${module.dog_houses["b"].flintstones}"
}

output "dino_c_greeting" {
    value = "*random grunts*, ${module.dog_houses["c"].flintstones}"
}

output "astro_a_greeting" {
    value = "Greetings, ${module.dog_houses["a"].jetsons}"
}

output "astro_b_greeting" {
    value = "Avast, ${module.dog_houses["b"].jetsons}"
}

output "astro_c_greeting" {
    value = "*random beeps*, ${module.dog_houses["c"].jetsons}"
}

output "scoob_a_greeting" {
    value = "Zoinks, ${module.dog_houses["a"].shaggy}"
}

output "scoob_b_greeting" {
    value = "Jeepers, ${module.dog_houses["b"].shaggy}"
}

output "scoob_c_greeting" {
    value = "Jinkies, ${module.dog_houses["c"].shaggy}"
}

output "dino_concrete_greeting" {
    value = "Hello, ${module.concrete_dog_house.flintstones}"
}

output "astro_concrete_greeting" {
    value = "Hello, ${module.concrete_dog_house.jetsons}"
}

output "scoob_concrete_greeting" {
    value = "Hello, ${module.concrete_dog_house.shaggy}"
}

output "scoob_a_bowl" {
    value = module.dog_houses["a"].spooky_bowl
}

output "astro_c_bowl" {
    value = module.dog_houses["c"].space_bowl
}

