typedef "info" {
  type = object({
    division   = string
    department = string
    team       = string
    project    = string
  })
}

typedef "transformer" {
  type = object({
    id = string
  })
}

function "validate_info" {
  parameter "info" {
    type = symbols::types(info)
    # validation
  }
  return = param.info
}

function "identity" {
  parameter "info" {
    type = symbols::types(info)
    # validation
  }
  parameter "target" {
    type = string
  }
  locals {
    info = symbols::validate_info(param.info)
  }
  return = "${local.info.division}-${local.info.department}-${local.info.team}-${local.info.project}-${param.target}"
}


function "org" {
  parameter "info" {
    type = symbols::types(info)
  }
  locals {
    info = symbols::validate_info(param.info)
  }
  return = "${local.info.division}_${local.info.department}"
}
function "owner" {
  parameter "info" {
    type = symbols::types(info)
  }
  locals {
    info = symbols::validate_info(param.info)
  }
  return = "${local.info.team}_${local.info.project}"
}
