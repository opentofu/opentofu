/**
 * Copyright (c) The OpenTofu Authors
 * SPDX-License-Identifier: MPL-2.0
 * Copyright (c) 2023 HashiCorp, Inc.
 * SPDX-License-Identifier: MPL-2.0
 */

import React from 'react';
import BuildKiteSVG from './buildkite.svg'

const BuildKite = () => {
    return (
      <p style={{ textAlign: "center", padding: "1.5rem" }}>
        <a
          href={"https://buildkite.com"}
          style={{ display: "block", color: "#fff", textDecoration: "none" }}
        >
          Thank you to{" "}
          <BuildKiteSVG
            style={{ maxWidth: "50%", marginLeft: "auto", marginRight: "auto" }}
          />{" "}
          for sponsoring the OpenTofu package hosting.
        </a>
      </p>
    );
};

export default BuildKite;
