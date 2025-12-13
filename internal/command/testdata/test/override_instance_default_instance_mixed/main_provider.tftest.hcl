
mock_provider "test" {
}

override_resource {
    target = test_resource.cats["pet_a"]
    values = {
        id = "Lassie"
    }
}

override_resource {
    target = test_resource.cats
    values = {
        id = "Baby"
    }
}

override_resource {
    target = test_resource.cats["pet_c"]
    values = {
        id = "Babby"
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
        condition = strcontains(output.pet_b_greeting, "Hello Baby")
        error_message = "Woops thats not what that output should be"
    }
}

run "test_c" {
    assert {
        condition = strcontains(output.pet_c_greeting, "Sup Babby")
        error_message = "Woops thats not what that output should be"
    }
}
