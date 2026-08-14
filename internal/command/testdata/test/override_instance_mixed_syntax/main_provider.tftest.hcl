mock_provider "test" {
}

override_resource {
    target = module.dog_houses.test_resource.dogs[*]
    values = {
        id = "Woofer"
    }
}

run "test_a" {
    assert {
        condition = strcontains(output.scoob_b_greeting, "Jeepers, Woofer")
        error_message = "Woops thats not what that output should be"
    }
}
