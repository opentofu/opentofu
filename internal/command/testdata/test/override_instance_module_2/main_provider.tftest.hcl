mock_provider "test" {
}

override_resource {
    target = module.dog_houses[*].test_resource.dogs["scoob"]
    values = {
        id = "Scooby Doo"
    }
}

override_resource {
    target = module.dog_houses["c"].test_resource.dogs["astro"]
    values = {
        id = "Krypto"
    }
}

override_resource {
    target = module.dog_houses["b"].test_resource.dogs[*]
    values = {
        id = "Snoopy"
    }
}

# Prefer overrides due to earlier module instance key over resource instance keys across general modules 
run "test_a" {
    assert {
        condition = strcontains(output.scoob_b_greeting, "Jeepers, Snoopy")
        error_message = "Woops thats not what that output should be"
    }
}

run "test_b" {
    assert {
        condition = strcontains(output.astro_b_greeting, "Avast, Snoopy")
        error_message = "Woops thats not what that output should be"
    }
}

run "test_c" {
    assert {
        condition = strcontains(output.scoob_c_greeting, "Jinkies, Scooby Doo")
        error_message = "Woops thats not what that output should be"
    }
}
