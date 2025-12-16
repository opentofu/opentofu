mock_provider "test" {
    override_resource {
        target = test_resource.cats["pet_a"]
        values = {
            id = "Lassie"
        }
    }
}

override_resource {
    target = test_resource.cats
    values = {
        id = "Bartholomew"
    }
}
# Even though this override is less specific, it's done at the root level.
# All root-level test resource overrides are given precedence over any override in the mock provider

run "test_a" {
    assert {
        condition = strcontains(output.pet_a_greeting, "Hi Bartholomew")
        error_message = "Woops thats not what that output should be"
    }
}

run "test_b" {
    assert {
        condition = strcontains(output.pet_b_greeting, "Hello Bartholomew")
        error_message = "Woops thats not what that output should be"
    }
}
