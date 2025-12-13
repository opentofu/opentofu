locals {
    my_pets = toset(["pet_a", "pet_b", "pet_c"])
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
