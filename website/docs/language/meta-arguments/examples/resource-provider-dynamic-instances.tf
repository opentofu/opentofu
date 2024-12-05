variable "aws_regions" {
  type = map(object({
    vpc_cidr_block = string
  }))
}

provider "aws" {
  alias    = "by_region"
  for_each = var.aws_regions

  region = each.key
}

resource "aws_vpc" "private" {
  # This expression filters var.aws_regions to include only
  # the elements whose value is not null. Refer to the
  # warning in the text below for more information.
  for_each = {
    for region, config in var.aws_regions : region => config
    if config != null
  }
  provider = aws.by_region[each.key]

  cidr_block = each.value.vpc_cidr_block
}
