## FAQ

### Who decides if a feature will be implemented and how is that decision made?

When you submit an enhancement request, the [maintainers](MAINTAINERS.md) looks at your issue first. Given the size of the code, adding a new feature is always a careful balancing act. The core team takes the following points into consideration:

1. **Is it possible to implement this on a technical level?**<br />Sometimes, even if a feature would be extremely useful, the state of the codebase doesn't let us do it. 
2. **Does the feature cause more technical debt?**<br />A feature request may hide a larger issue under the hood. Sometimes it is more desirable to resolve the underlying issue instead of implementing the feature in isolation.
3. **Is there someone who would do the work?**<br />The maintainers doesn't have the capacity to implement everything, so for many issues community contributions are very welcome. Sometimes companies external to OpenTofu decide to dedicate developers for the development of a specific larger feature, which can also weigh in on the decision-making process.
4. **Is there enough capacity on the maintainers to support a community contributor?**<br />Core engineers dedicate time to review PRs and help community members with questions as we don't expect contributors to implement the entire feature in isolation. We actively participate with planning, reviews and writing documentation as needed, some more than others, which flows into the decision of accepting or rejecting an issue. 
5. **Does this feature enable someone to do something new with OpenTofu they were not able to do before?**<br />We prioritize work based on community input and need. An issue with a large number of reactions is more likely to make it into the accepted phase, but if a viable workaround or tool exists, the feature is less likely to be accepted. If a feature is just the integration of a cool technology, but doesn't solve any problems for a large number of people, it will be rejected.

Depending on the maintainers's review, a feature request can have the following outcomes:

1. **The feature is accepted for development by the maintainers.** This means that the core team will schedule it for an upcoming release and develop it.
2. **The feature is accepted and open for community contributions.** The maintainers adds a `help wanted` label and waits for community volunteers to develop it.
3. **More information is needed.** The maintainers will either add questions in comments, or when there is a deep technical issue to be resolved, call for an [RFC](./rfc/README.md) to detail a possible implementation.
4. **More community input is needed.** When an issue is, on its surface, valuable, but there is no track record of a large portion of the community needing it, the maintainers adds the `needs community input` label. If you are interested in the feature and would like to use it, please add a reaction to the issue and add a description on specifically what problem it would solve for you in a comment.
5. **The feature is rejected.** If based on the criteria above it is not feasible to implement the feature, the maintainers closes the issue with an explanation why it is being closed.
6. **The feature is referred to the Technical Steering Committee**. If the feature requires the commitment of a larger amount of core developer time, has legal implications, or otherwise requires leadership attention, the maintainers adds the feature to the agenda of the Technical Leadership Committee. Once decided, the TSC records the decision in the [TSC notes](TSC).

---

### I've been assigned an issue, now what?

First of all, thank you for volunteering! Before you begin coding, please take a minute to answer the following questions:

1. **Is the issue clear enough? Do you know what the expected outcome is?** Sometimes, issues are not as clear as they should be. If the issue is unclear, please let us know in the `#dev-general` channel on the [OpenTofu Slack](https://opentofu.org/slack/) so a maintainers member can clarify it.
2. **What's your timeline for implementing the issue?** Please communicate how much time you estimate you'll need. We ask for this to avoid issues staying assigned to an inactive community member and blocking other contributors. However, if you find out you'll need more time, please feel free to comment on the issue and we'll leave the issue assigned to you.
3. **Do you have questions about parts of the OpenTofu codebase?** Please post in `#dev-general` on the [OpenTofu Slack](https://opentofu.org/slack/) for a quick answer. (Please avoid DMs as the person you are DM'ing may not be available or know the answer.)
4. **Do you have experience writing tests?** All of our code-related PR's need adequate tests. Please [familiarize yourself with testing in Go](https://go.dev/doc/tutorial/add-a-test).
5. **Have you read this document?** If not, please give it a quick skim, especially the sections about copyright and signoffs. 

---

### What do the labels mean?

- [`pending-decision`](https://github.com/opentofu/opentofu/labels/pending-decision): there is no decision if this issue will be implemented yet. You can show your support for this issue by commenting on it and describing what implementing this issue would solve for you.
- [`pending-steering-committee-decision`](https://github.com/opentofu/opentofu/labels/pending-steering-committee-decision): the maintainers has referred this issue to the Technical Steering Committee for a decision.
- [`accepted`](https://github.com/opentofu/opentofu/labels/accepted): the issue is accepted for development by either the maintainers or a community contributor. Please check if it's assigned to someone and comment on the issue if you want to work on it.
- [`help wanted`](https://github.com/opentofu/opentofu/labels/help%20wanted): this issue is open for community contributions. Please check if it's assigned to someone and comment on the issue if you want to work on it.
- [`good first issue`](https://github.com/opentofu/opentofu/labels/good%20first%20issue): this issue is relatively simple. If you are looking for a first contribution, this may be for you. Please check if it's assigned to someone and comment on the issue if you want to work on it.
- [`bug`](https://github.com/opentofu/opentofu/labels/bug): it's broken and needs to be fixed.
- [`enhancement`](https://github.com/opentofu/opentofu/labels/enhancement): it's a short-form feature request. If the implementation path is unclear, an `rfc` may be needed in addition.
- [`documentation`](https://github.com/opentofu/opentofu/labels/documentation): something that needs a description on the OpenTofu website.
- [`rfc`](https://github.com/opentofu/opentofu/labels/rfc): a long-form discussion on building a feature or solving bug, see [RFC](./rfc/README.md).
- [`question`](https://github.com/opentofu/opentofu/labels/question): the maintainers needs more information on the issue to decide.
- [`needs-community-input`](https://github.com/opentofu/opentofu/labels/needs-community-input): the maintainers needs to see how many people are affected by this issue. You can provide feedback by using reactions on the issue and adding your use case in the comments. (Please describe what problem it would solve for you specifically.)
- [`needs-rfc`](https://github.com/opentofu/opentofu/labels/needs-rfc): this issue needs a detailed technical description on how it would be implemented in the form of an [RFC](./rfc/README.md).

---

### My issue / PR / comment is not getting any responses!

Please accept our apologies, sometimes issues and comments fall through the cracks. Please post in `#dev-general` on the [OpenTofu Slack](https://opentofu.org/slack/) to alert the maintainers members to the lack of an answer.

---

### When I run `tofu version`, it contains a `-dev` suffix. How do I get rid of it?

You can get rid of this suffix by changing the `version.dev` ldflag:

```
go build -ldflags "-w -s -X 'github.com/opentofu/opentofu/version.dev=no'" -o tofu ./cmd/tofu
```

---

### How do I enable experimental features?

You can build `tofu` with the experimental features enabled using the `main.experimentsAllowed` ldflag set to `yes`:

```
go build -ldflags "-w -s -X 'main.experimentsAllowed=yes'" -o tofu ./cmd/tofu
```

---

### Can you implement X in the language?

It depends. The OpenTofu language is based on the [HCL language](https://github.com/hashicorp/hcl) and the [cty typing system](https://github.com/zclconf/go-cty). Since both are available under an open source license, and we prefer to keep compatibility as much as possible, we currently don't maintain a fork of these libraries. Language features may need to be partially or fully implemented in HCL or cty and if that is the case, we can't implement them in OpenTofu without changes to the respective libraries beforehand.

---

### I don't like HCL, can you replace it?

HCL is baked into every corner of the OpenTofu codebase, so is here to stay. However, you can use the [JSON configuration syntax](https://opentofu.org/docs/language/syntax/json/) to write your code in JSON instead of HCL.

---

### Can you fix a bug in or add a feature to the Hashicorp providers, such as AWS, Azure, etc.?

We currently only maintain a read-only mirror of these providers for the purposes of building binaries for the OpenTofu registry which would otherwise not be available. They are not true downstream versions that we can add patches to and OpenTofu users expect them to work the same as the upstream versions. In short: no, we cannot fix bugs or add features to these providers.

---

### Can you fork project X?

Currently, we are at capacity for development and do not have additional capacity to take on additional projects unless necessary for the continued work on OpenTofu. While the final determination lies with the [Technical Steering Committee](#who-is-the-technical-steering-committee), the answer is likely no in almost all cases.


