mock_provider "test" {
}

override_resource {
    target = test_resource.cats["pet_a"]
    values = {
        id = "Lassie"
    }
}

run "test_a" {
    assert {
        condition = strcontains(output.pet_a_greeting, "Hi Lassie")
        error_message = "Woops thats not what that output should be"
    }
}

run "test_b" {
    assert {
        condition = strcontains(output.pet_b_greeting, "Hello Lassie")
        error_message = "Woops thats not what that output should be"
    }
}
