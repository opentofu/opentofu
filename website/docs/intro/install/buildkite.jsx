import React from 'react';
import BuildKiteSVG from './buildkite.svg'

export default ({children, color}) => (
    <p style={{textAlign: "center", padding: "1.5rem"}}><a href={"https://buildkite.com"} style={{display: "block", color: "#fff", textDecoration: "none"}}>
        Thank you to <BuildKiteSVG style={{maxWidth: "50%", marginLeft: "auto", marginRight: "auto"}}/> for sponsoring the OpenTofu package hosting.
    </a></p>
);
