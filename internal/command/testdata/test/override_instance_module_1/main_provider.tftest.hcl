mock_provider "test" {
}

override_resource {
    target = module.dog_houses.test_resource.dogs
    values = {
        id = "Puppy"
    }
}

run "test_a" {
    assert {
        condition = strcontains(output.dino_a_greeting, "Hey there, Puppy")
        error_message = "Woops thats not what that output should be"
    }
}

run "test_b" {
    assert {
        condition = strcontains(output.astro_b_greeting, "Avast, Puppy")
        error_message = "Woops thats not what that output should be"
    }
}

run "test_c" {
    assert {
        condition = strcontains(output.scoob_c_greeting, "Jinkies, Puppy")
        error_message = "Woops thats not what that output should be"
    }
}
