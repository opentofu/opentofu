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
