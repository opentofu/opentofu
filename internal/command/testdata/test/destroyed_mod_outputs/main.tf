variable "numbers" {
    type = set(string)
}

module "mod" {
    source = "./mod"
    for_each = var.numbers
    val = each.key
}
