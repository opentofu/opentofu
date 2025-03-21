variable "foo" {
  type       = string
  default    = "foo default value"
  deprecated = "foo deprecated note"
}

module "bar-call" {
  source = "../modbar"
  bar    = "bar given value"
}