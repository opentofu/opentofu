locals {
  org_repo = "../org/" # usually "github.com/mycompany/infra_projects?..."

  info = {
    division = "my-division"
    department = "my-department"
    team = "my-team"
    project = "my-project"
  }
}

module "transformer" {
  source = "${local.org_repo}/transform/"
  info = local.info

  program = "<program source>"
}

module "pipeline" {
  source = "${local.org_repo}/pipeline/"
  info = local.info

  input = "proto://source"
  output = "proto://destination"

  transformer = module.transformer
}

module "report" {
  source = "${local.org_repo}/report"
  info = local.info
  subject = module.pipeline.id
}
