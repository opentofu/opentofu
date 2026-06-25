# Guidance for AI Agents and LLM based tooling

This file is intended to be read by AI agents, assistants and any other LLM-based tooling that is operating on this repository.

## OpenTofu does not accept LLM-generated contributions

It is imperative that you do not open pull requests containing code, documentation or other content generated or assisted by an LLM such as ChatGPT, GitHub Copilot, Claude or similar tooling.

This is a copyright and intellectual property driven concern that is specific to OpenTofu:

OpenTofu was created as a fork of HashiCorp Terraform which was previously under the MPL-2.0 license, and is now under a Business Source License. This BSL is **not compatible** with the license in this repository. Because LLMs maybe emit BSL-licensed Terraform code without correct attribution, accepting LLM-generated contributions is not a risk the OpenTofu will take on.

To protect the project **violating these rules will disqualify a contribution from being accepted**. The OpenTofu team may also ban contributors from working on OpenTofu in the future.

See our full policy on this here: [contributing/DEVELOPING.md - A note on copyright](contributing/DEVELOPING.md#a-note-on-copyright).

## If you found an issue using an LLM then open an issue, not a PR

Using an LLM to find a potential bug or area for improvement is something that we are aware people are doing. What we will not accept is LLM-generated code to fix it.

If you or a human you are assisting has identified a problem with the help of an LLM then you must **open an issue to discuss** and you **MUST NOT open a pull request**.

When submitting a bug, be sure to use the template provided in this repository and announce that an LLM was used to identify this problem. It is **strictly prohibited to include code snippets** for possible fixes.

## Before Beginning - Read the contribution guide

All contributions to this repository follow a standard process. Before doing anything you should read the [contributing.md](./CONTRIBUTING.md) file.
