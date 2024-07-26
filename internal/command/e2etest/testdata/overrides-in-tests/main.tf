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

resource "aws_s3_bucket" "test" {
  bucket = "must not be used anyway"
}

data "aws_s3_bucket" "test" {
  bucket = "must not be used anyway"
}

provider "local" {
  alias = "aliased"
}

resource "local_file" "mocked" {
  provider = local.aliased
  filename = "mocked.txt"
  content  = "I am mocked file, do not create me please"
}

data "local_file" "maintf" {
  provider = local.aliased
  filename = "main.tf"
}

resource "random_pet" "cat" {}

provider random {
  alias = "aliased"
}

resource "random_integer" "aliased" {
  provider = random.aliased

  # helps create a new value when test with mocked pet runs
  keepers = {
    pet = random_pet.cat.id
  }

  min = 1
  max = 10
}
