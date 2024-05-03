module "first" {
  source = "./first"
}

module "second" {
  source = "./second"
}

resource "local_file" "dont_create_me" {
    filename = "${path.module}/dont_create_me.txt"
    content = "101"
}

resource "local_file" "create_me" {
    filename = "${path.module}/create_me.txt"
    content = "101"
}

data "local_file" "second_mod_file" {
    filename = module.first.create_me_filename
}
