# OCI survey results

---

This document is part of the [OCI registries RFC](../20241206-oci-registries.md).

| [« Previous](1-oci-primer.md) | [Up](../20241206-oci-registries.md) | [Next »](3-design-considerations.md) |

---

We have run a survey on the OCI feature among the OpenTofu community, which attracted 103 responses. This document discusses this survey and the conclusions we drew from it.

## 82% want to use private provider registries

96 out of 103 respondents (93%) indicated that they are interested in using OCI for provider distribution. Out of these 85 (82%) indicated that they would be interested in using OCI either to mirror an existing public provider (66 responses, 64%) or publish a private provider (63 responses, 61%). 8 respondents (8%) indicated that they are purely interested in a public provider ecosystem based on OCI and do not have a use case for a private OCI registry. However, 13 respondents (13%) indicated that they are authoring a public provider and would like to publish their provider in an OCI registry.

One respondant noted that a generic OCI registry would not automatically generate and publish provider documentation as the main OpenTofu registry does. (Thank you!) Therefore if it becomes popular in future to publish providers or modules _exclusively_ in OCI registries then we will need to somehow address that.

We also asked what solutions respondants were already using for provider distribution today. The answers with more than one response were:

- **Using the default OpenTofu public registry:** 56 responses
- **Using a private provider registry:** 27 responses
- **Using an alternative public registry:** 18 responses
- **Using a filesystem mirror directory:** 22 responses
- **Manually deploying providers:** 14 responses
- **Not using OpenTofu today:** 13 responses
- **Using a network mirror server:** 11 responses

## 90% want to use private module registries

93 out of 103 respondents (90%) indicated that they would be interested in using OCI for module distribution. Out of these, 87 (84%) indicated that they would be interested in using a private registry to either publish modules in a private registry (82 respondents, 79%) or mirror public modules (61 respondents, 59%). Only 4 respondents (4%) indicated that they would be purely interested in using a public OCI module ecosystem. 17 respondents (17%) indicated that they are creating publicly available modules and would be interested in publishing them to an OCI registry.

We also asked what solutions respondants were already using for module distribution today. The answers with more than one response were:

- **Using a private Git repository:** 72 responses
- **Using the default OpenTofu public Registry:** 37 responses
- **Using a public Git repository:** 37 responses
- **Using a private OpenTofu/Terraform registry:** 32 responses
- **Using an alternative public registry:** 19 responses
- **Using a network filesystem:** 9 responses
- **Not using modules with OpenTofu:** 7 responses

## 85% want to use a security scanner

In our survey, we asked our community which security scanning tools they would like to use. 88 out of 103 respondents have answered this question and indicated the following tools would be of help (responses with more than one vote):

- **Trivy**: 47 responses
- **TFLint:** 34 responses
- **Snyk:** 26 responses
- **Unsure which one:** 24 responses
- **Open Policy Agent:** 21 responses
- **Terrascan:** 19 responses
- **JFrog Xray:** 12 responses
- **Anchore (including Grype):** 9 responses
- **Clair:** 8 responses
- **Sonatype:** 7 responses
- **Qualys:** 7 responses

## 33% use an airgapped setup

What we found most surprising was that 35 respondents (33%) indicated that they have a some level of air-gapped infrastucture. We use "air-gapped" to mean any situation where OpenTofu is running in an environment where it is unable to access (or forbidden from accessing by policy) public registry services, and so this requires creating a local mirror of all needed dependencies.

## A wide range of registry implementations

We have also asked our community about which OCI registries they might like to use with OpenTofu. The following are responses with more than one vote:

- **GitHub (ghcr.io):** 57 responses
- **AWS ECR:** 54 responses
- **Azure Container Registry:** 31 responses
- **Self-hosted Harbor:** 27 responses
- **Self-hosted "registry":** 27 responses
- **Google Container Registry:** 25 responses
- **Docker Hub:** 22 responses
- **Self-hosted GitLab:** 22 responses
- **Self-hosted JFrog:** 19 responses
- **GitLab.com:** 11 responses
- **Quay.io:** 9 responses
- **Self-hosted Quay:** 8 responses
- **I don't know yet:** 8 responses
- **Jfrog (cloud):** 5 responses
- **Sonatype (cloud):** 4 responses

## Tooling

We asked which tools our community would like to use for generating and publishing OCI artifacts for OpenTofu. 27 respondents indicated that they would like to use Podman/Docker/Containerd workflows to push artifacts. In contrast, 24 respondents indicated that they would like OpenTofu to have its own built-in tooling. Although not present in the survey answers, 3 respondents indicated that they would like to use ORAS or built-in tooling to publish OCI artifacts. 24 respondents indicated that they have no strong preference as long as the tooling works in GitHub Actions.

When it comes to credential storage, a majority of respondents to that question (36 responses) indicated that they are using some sort of cloud integration for their OCI credentials. 19 responses indicated that their users are using the Docker/Podman credential helpers. 16 respondents are using local cleartext storage for their OCI credentials today. 2 respondents responded that they are using Kubernetes secrets to store OCI credentials. As far as preferences go, 55 respondents indicated that they would like OpenTofu to reuse existing Container ecosystem credentials, whereas 26 respondents indicated that OpenTofu should use its own credentials. Out of these, 12 respondents indicated that they are happy with both solutions.

## A note of gratitude

In total, we have received a larger number of responses than we expected, equally distributed across GitHub, Slack, LinkedIn and Reddit as sources. We owe our community a debt of gratitude as it gave us a clearer picture what use cases to focus on and how to design our OCI implementation. 

In the motivation for OCI many responses indicated that it would solve a real pain-point when it comes to running private distribution servers or CI systems with a large number of jobs requiring bandwidth for downloads. According to the answers, OCI is widely deployed and makes in-house distribution, caching and mirroring much easier.

Thank you again to all who participated in the survey. While our first iteration of these features as described in this RFC will not satisfy all of the responses, we have made an intentional effort to reserve opportunities to extend the design to meet additional use-cases in future releases.

---

| [« Previous](1-oci-primer.md) | [Up](../20241206-oci-registries.md) | [Next »](3-design-considerations.md) |

---