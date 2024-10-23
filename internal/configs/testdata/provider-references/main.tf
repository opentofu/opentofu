provider "null" {
}

provider "null" {
  alias = "alias"
}

provider "null" {
  alias = "a-0"
}

provider "null" {
  alias = "a-1"
}

locals {
  alias = "alias"
  aliases = {
    "alias": "alias",
    "a-0": "a-0",
    "a-1": "a-1"
  }
}
