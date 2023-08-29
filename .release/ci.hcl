# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

schema = "1"

project "opentf" {
  // the team key is not used by CRT currently
  team = "opentffoundation"
  slack {
    notification_channel = "C05NDN8RB3J"
  }
  github {
    organization = "opentffoundation"
    repository = "opentf"

    release_branches = [
      "main",
      "release/**",
      "v**.**",
    ]
  }
}

event "build" {
  depends = ["merge"]
  action "build" {
    organization = "opentffoundation"
    repository = "opentf"
    workflow = "build"
  }
}

// Read more about what the `prepare` workflow does here:
// https://hashicorp.atlassian.net/wiki/spaces/RELENG/pages/2489712686/Dec+7th+2022+-+Introducing+the+new+Prepare+workflow
event "prepare" {
  depends = ["build"]

  action "prepare" {
    organization = "opentffoundation"
    repository   = "crt-workflows-common"
    workflow     = "prepare"
    depends      = ["build"]
  }

  notification {
    on = "fail"
  }
}

## These are promotion and post-publish events
## they should be added to the end of the file after the verify event stanza.

event "trigger-staging" {
// This event is dispatched by the bob trigger-promotion command
// and is required - do not delete.
}

event "promote-staging" {
  depends = ["trigger-staging"]
  action "promote-staging" {
    organization = "opentffoundation"
    repository = "crt-workflows-common"
    workflow = "promote-staging"
    config = "release-metadata.hcl"
  }

  notification {
    on = "always"
  }
}

event "promote-staging-docker" {
  depends = ["promote-staging"]
  action "promote-staging-docker" {
    organization = "opentffoundation"
    repository = "crt-workflows-common"
    workflow = "promote-staging-docker"
  }

  notification {
    on = "always"
  }
}

event "promote-staging-packaging" {
  depends = ["promote-staging-docker"]
  action "promote-staging-packaging" {
    organization = "opentffoundation"
    repository = "crt-workflows-common"
    workflow = "promote-staging-packaging"
  }

  notification {
    on = "always"
  }
}

event "trigger-production" {
// This event is dispatched by the bob trigger-promotion command
// and is required - do not delete.
}

event "promote-production" {
  depends = ["trigger-production"]
  action "promote-production" {
    organization = "opentffoundation"
    repository = "crt-workflows-common"
    workflow = "promote-production"
  }

  notification {
    on = "always"
  }
}

event "promote-production-docker" {
  depends = ["promote-production"]
  action "promote-production-docker" {
    organization = "opentffoundation"
    repository = "crt-workflows-common"
    workflow = "promote-production-docker"
  }

  notification {
    on = "always"
  }
}

event "promote-production-packaging" {
  depends = ["promote-production-docker"]
  action "promote-production-packaging" {
    organization = "opentffoundation"
    repository = "crt-workflows-common"
    workflow = "promote-production-packaging"
  }

  notification {
    on = "always"
  }
}

event "crt-hook-tfc-upload" {
  depends = ["promote-production-packaging"]
  action "crt-hook-tfc-upload" {
    organization = "opentffoundation"
    repository = "opentf-releases"
    workflow = "crt-hook-tfc-upload"
  }

  notification {
    on = "always"
  }
}
