mock_provider "test" {
}

override_resource {
    target = module.dog_houses[*].test_resource.dogs["scoob"]
    values = {
        id = "Scooby Doo"
    }
}

override_resource {
    target = module.dog_houses["c"].test_resource.dogs[*]
    values = {
        id = "Snoopy"
    }
}

override_data {
    target = module.dog_houses[*].data.test_data_source.water_bowl[*]
    values = {
        value = "hose"
    }
}

run "test_a" {
    assert {
        condition = strcontains(output.scoob_a_bowl, "Scooby Doo drinks from a hose")
        error_message = "Woops thats not what that output should be"
    }
}

run "test_b" {
    assert {
        condition = strcontains(output.astro_c_bowl, "Snoopy drinks from a hose")
        error_message = "Woops thats not what that output should be"
    }
}
