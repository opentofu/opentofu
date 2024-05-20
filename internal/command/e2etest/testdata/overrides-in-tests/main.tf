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

resource "random_integer" "count" {
  count = 2

  min = 1
  max = 10
}

resource "random_integer" "for_each" {
  for_each = {
    "a": {
        "min": 1
        "max": 10
    }
    "b": {
        "min": 20
        "max": 30
    }
  }

  min = each.value.min
  max = each.value.max
}

module "rand_for_each" {
    for_each = {
        "a": 1
        "b": 2
    }

    source = "./rand"
}

module "rand_count" {
    count = 2

    source = "./rand"
}
