mock_provider "test" {
}

override_resource {
    target = module.concrete_dog_house.test_resource.dogs["dino"]
    values = {
        id = "Lizard"
    }
}

run "test_a" {
    assert {
        condition = strcontains(output.dino_concrete_greeting, "Hello, Lizard")
        error_message = "Woops thats not what that output should be"
    }
}

run "test_b" {
    assert {
        condition = strcontains(output.scoob_concrete_greeting, "Hello, Lizard")
        error_message = "Woops thats not what that output should be"
    }
}
