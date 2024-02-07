/**
 * Copyright (c) The OpenTofu Authors
 * SPDX-License-Identifier: MPL-2.0
 * Copyright (c) 2023 HashiCorp, Inc.
 * SPDX-License-Identifier: MPL-2.0
 */

module.exports = {
    "extends": ["plugin:mdx/recommended"],
    // optional, if you want to lint code blocks at the same time
    "settings": {
        "mdx/code-blocks": true,
        // optional, if you want to disable language mapper, set it to `false`
        // if you want to override the default language mapper inside, you can provide your own
        "mdx/language-mapper": {},
        "mdx/remark": {}
    }
}
