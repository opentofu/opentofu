# See internal/initwd/testdata/registry-modules/root.tf for more information on the module required

variable "modver" {
	type = string
}

module "acctest_root" {
  source  = "hashicorp/module-installer-acctest/aws"
  version = nonsensitive(var.modver)
}
