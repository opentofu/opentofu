override_module {
    target = module.second
}

override_resource {
    target = local_file.dont_create_me
}

override_resource {
    target = module.first.local_file.dont_create_me
}

run "check_root_overridden_res" {
    assert {
        condition = !fileexists("${path.module}/dont_create_me.txt")
        error_message = "File 'dont_create_me.txt' must not be created in the root module"
    }
}

run "check_root_overridden_res_twice" {
    override_resource {
        target = local_file.dont_create_me
        values = {
            file_permission = "0333"
        }
    }

    assert {
        condition = !fileexists("${path.module}/dont_create_me.txt") && local_file.dont_create_me.file_permission == "0333"
        error_message = "File 'dont_create_me.txt' must not be created in the root module and its file_permission must be overridden"
    }
}

run "check_root_data" {
    assert {
        condition = data.local_file.second_mod_file.content == file("main.tf")
        error_message = "Content from the local_file data in the root module must be from real file"
    }
}

run "check_root_overridden_data" {
    override_data {
        target = data.local_file.second_mod_file
        values = {
            content = "101"
        }
    }

    assert {
        condition = data.local_file.second_mod_file.content == "101"
        error_message = "Content from the local_file data in the root module must be overridden"
    }
}

run "check_overridden_module_output" {
    override_module {
        target = module.first
        outputs = {
            create_me_filename = "main.tftest.hcl"
        }
    }

    assert {
        condition = data.local_file.second_mod_file.content == file("main.tftest.hcl")
        error_message = "Overridden module output is not used in the depending data resource"
    }
}

run "check_first_module" {
    assert {
        condition = fileexists("${path.module}/first/create_me.txt")
        error_message = "File 'create_me.txt' must be created in the first module"
    }
}

run "check_first_module_overridden_res" {
    assert {
        condition = !fileexists("${path.module}/first/dont_create_me.txt")
        error_message = "File 'dont_create_me.txt' must not be created in the first module"
    }
}

run "check_second_module" {
    assert {
        condition = !fileexists("${path.module}/second/dont_create_me.txt")
        error_message = "File 'dont_create_me.txt' must not be created in the second module"
    }
}

run "check_third_module" {
    assert {
        condition = !fileexists("${path.module}/second/third/dont_create_me.txt")
        error_message = "File 'dont_create_me.txt' must not be created in the third module"
    }
}

override_resource {
    target = random_integer.count
}

override_resource {
    target = random_integer.for_each
}

override_module {
    target = module.rand_count
}

override_module {
    target = module.rand_for_each
}

run "check_for_each_n_count_mocked" {
    assert {
        condition = random_integer.count[0].result == 0
        error_message = "Mocked random integer should be 0"
    }

    assert {
        condition = random_integer.count[1].result == 0
        error_message = "Mocked random integer should be 0"
    }

    assert {
        condition = random_integer.for_each["a"].result == 0
        error_message = "Mocked random integer should be 0"
    }

    assert {
        condition = random_integer.for_each["b"].result == 0
        error_message = "Mocked random integer should be 0"
    }

    assert {
        condition = module.rand_count[0].random_integer == null
        error_message = "Mocked random integer should be null"
    }

    assert {
        condition = module.rand_count[1].random_integer == null
        error_message = "Mocked random integer should be null"
    }

    assert {
        condition = module.rand_for_each["a"].random_integer == null
        error_message = "Mocked random integer should be null"
    }

    assert {
        condition = module.rand_for_each["b"].random_integer == null
        error_message = "Mocked random integer should be null"
    }
}

run "check_for_each_n_count_overridden" {
    override_resource {
        target = random_integer.count
        values = {
            result = 101
        }
    }

    assert {
        condition = random_integer.count[0].result == 101
        error_message = "Overridden random integer should be 101"
    }

    assert {
        condition = random_integer.count[1].result == 101
        error_message = "Overridden random integer should be 101"
    }

    override_resource {
        target = random_integer.for_each
        values = {
            result = 101
        }
    }

    assert {
        condition = random_integer.for_each["a"].result == 101
        error_message = "Overridden random integer should be 101"
    }

    assert {
        condition = random_integer.for_each["b"].result == 101
        error_message = "Overridden random integer should be 101"
    }

    override_module {
        target = module.rand_count
        outputs = {
            random_integer = 101
        }
    }

    assert {
        condition = module.rand_count[0].random_integer == 101
        error_message = "Mocked random integer should be 101"
    }

    assert {
        condition = module.rand_count[1].random_integer == 101
        error_message = "Mocked random integer should be 101"
    }

    override_module {
        target = module.rand_for_each
        outputs = {
            random_integer = 101
        }
    }

    assert {
        condition = module.rand_for_each["a"].random_integer == 101
        error_message = "Mocked random integer should be 101"
    }

    assert {
        condition = module.rand_for_each["b"].random_integer == 101
        error_message = "Mocked random integer should be 101"
    }
}

# ensures non-aliased provider is mocked by default
mock_provider "aws" {
  mock_resource "aws_s3_bucket" {
    defaults = {
      arn = "arn:aws:s3:::mocked"
    }
  }

  mock_data "aws_s3_bucket" {
    defaults = {
      bucket_domain_name = "mocked.com"
    }
  }
}

# ensures non-aliased provider works as intended
# and aliased one is mocked
mock_provider "local" {
  alias = "aliased"
}

# ensures we can use this provider in run's providers block
# to use mocked one only for a specific test
mock_provider "random" {
  alias = "for_pets"

  mock_resource "random_pet" {
    defaults = {
      id = "my lovely cat"
    }
  }
}

mock_provider "random" {
  alias = "aliased"

  mock_resource "random_integer" {
    defaults = {
      id = "11"
    }
  }
}

run "check_mock_providers" {
  assert {
    condition     = resource.aws_s3_bucket.test.arn == "arn:aws:s3:::mocked"
    error_message = "aws s3 bucket resource doesn't have mocked values"
  }

  assert {
    condition     = data.aws_s3_bucket.test.bucket_domain_name == "mocked.com"
    error_message = "aws s3 bucket data doesn't have mocked values"
  }

  assert {
    condition     = !fileexists(local_file.mocked.filename)
    error_message = "file should not be created due to provider being mocked"
  }

  assert {
    condition     = data.local_file.maintf.content != file("main.tf")
    error_message = "file should not be read due to provider being mocked"
  }

  assert {
    condition     = resource.random_integer.aliased.id == "11"
    error_message = "random integer should be 11 due to provider being mocked"
  }
}

run "check_providers_block" {
  providers = {
    aws           = aws
    local.aliased = local.aliased
    random        = random.for_pets
  }

  assert {
    condition     = resource.random_pet.cat.id == "my lovely cat"
    error_message = "providers block in run should allow replacing real providers by mocked"
  }

  assert {
    condition     = resource.random_integer.aliased.id != "11"
    error_message = "random integer should not be mocked if providers block present"
  }
}
