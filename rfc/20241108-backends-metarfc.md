# Meta-RFC: The future of Backends

Related feature requests:
- [Backends as plugins](https://github.com/opentofu/opentofu/issues/382)
- [Add workspaces support to the HTTP backend](https://github.com/opentofu/opentofu/issues/317)
- [Document how someone could implement their own cloud backend](https://github.com/opentofu/opentofu/issues/960)
- [Conditionally load tfvars/tf file based on Workspace](https://github.com/opentofu/opentofu/issues/1053)
- Various requests for specific state storage implementations:
  - [Add git as backend](https://github.com/opentofu/opentofu/issues/1746)
  - [Add OpenBao as a remote state store](https://github.com/opentofu/opentofu/issues/907)
  - [OpenTofu Backend with OCI Object Storage](https://github.com/opentofu/opentofu/issues/1011)
  - [Add Huawei Cloud OBS remote storage backend support](https://github.com/opentofu/opentofu/issues/1343)
  - [Bring back swift backend for tfstate](https://github.com/opentofu/opentofu/issues/549)
  - (and numerous potential additions to already-implemented state storage backends)

OpenTofu's concept of "Backends" has recieved a lot of feature requests and other feedback, and is also a part of the system that hasn't seen significant design investment for quite some time. There are lots of different changes we could potentially make, but it's unlikely that we can make all of the changes we'd want to make in a single increment. Decisions made to solve one problem are likely to constrain how we can solve other problems though, so a _purely_ incremental approach risks "painting ourselves into a corner".

This document is a "Meta-RFC" that aims to describe holistically what we hope to achieve in the general area of "backends" -- which includes state storage but also other interesting features like remote operations -- so that we can more easily consider the implications of specific technical proposals we will make later in this area.

This document therefore doesn't propose any specific technical details itself, but instead proposes the creation of other RFCs covering the details of different parts of the overall problem. This RFC is "done" once those RFCs are written and accepted, and then those other RFCs will represent the actual software implementation work.

## Background

The set of features that we now consider to belong to the "backend" concept evolved considerably during the life of OpenTofu's predecessor project, Terraform. This incremental development is likely responsible, at least in part, for some of the design details as currently implemented, and so the following sections describe some of that background in the hope of helping us answer questions such as:

- Which complexity is necessary vs. accidental?
- Which behaviors are important to modern OpenTofu, vs. vestigial from obsolete earlier versions?
- How much freedom might we have to change these historical design decisions as we try to meet newer goals?

### In the beginning: Local State Only

In the very earliest versions, state was literally just a file written to local disk. The `-state` command line option specified where it should be read from and written to, though there was a default path `./terraform.tfstate` which would likely seem familiar to anyone who uses OpenTofu's current "local" backend.

During this period the typical practice was to commit that file to version control along with the associated configuration files. This meant a rather awkward and unconventional workflow though, because it encouraged applying configuration changes before merging those changes into a shared VCS branch and then merging both the configuration changes and the state updates together. That undermines typical code review practices because by the time the configuration changes are being reviewed it's already in some sense "too late": the infrastructure changes implied by the configuration changes have already been applied anyway.

### Initial Remote State Support

Later versions introduced the idea of "remote state", where the system would read and write state snapshots from some separate network service, such as Amazon S3.

The details of exactly how these features were used evolved slightly over several versions, but what all of the variants of this incarnation had in common is that remote state was configured exclusively with CLI commands, with no settings in the root module's `.tf` files.

The final incarnation involved subcommands of the `remote` command, with `remote config` as the main entry point:

```shellsession
$ terraform remote config -backend=s3 -backend-config=bucket=example -backend-config=path=example
```

Under this iteration of the design, remote state was _in addition to_ local state: the system would still write state snapshots to disk, but would _also_ write them to the configured remote location. The default on-disk location when remote state was enabled switched to `.terraform/terraform.tfstate`, which modern users might recognize as what is now just OpenTofu's record of the currently-initialized backend, without any actual state snapshot information.

Due to the configuration settings being exclusively on the command line, this particular design was highly error-prone. A typo in a `-backend-config` argument could cause future operations to start from an effectively-empty state. Worse still, if someone ran the `remote config` subcommand with remote state already enabled it would automatically overwrite the new location with latest snapshot from the previously-configured storage, and so it was a common mistake to overwrite the production environment's state snapshot with the staging environment's state snapshot when performing deployment gradually through a series of environments.

This era is therefore where the concept of "state lineage" originated: it was primarily a way to catch that mistake of accidentally overwriting one environment's snapshot with the snapshot from another environment, by having the system first check whether the new snapshot seems like it was intended to be a successor to the one previously stored.

What was called a "backend" in this era was _just_ for remote state storage, with no other capabilities. Selecting a backend internally meant selecting what in today's OpenTofu we call a "state manager", represented today as [the `statemgr.Storage` interface](https://pkg.go.dev/github.com/opentofu/opentofu@v1.8.5/internal/states/statemgr#Storage).

### Modern "Backends"

Terraform v0.9 significantly evolved the remote state mechanism, introducing the configuration-driven approach that OpenTofu uses, mostly-unchanged, today:

```hcl
terraform {
  backend "s3" {
    bucket = "example"
    path   = "example"
  }
}
```

The shift to using configuration blocks in the root module instead of command line arguments was the most significant user-facing change. This also ended the practice of storing state snapshots in both the remote location _and_ on local disk, replacing `.terraform/terraform.tfstate`'s former role with what is now just a confusing name for the file where OpenTofu "remembers" which backend configuration it's supposed to use in a specific working directory.

This also came with a huge overhaul in the code of what exactly the term "backend" means and how they were architected. This new architecture was designed so that _in theory_ choosing a backend could select more than just state storage, including the possibility of "remote operations". However, in practice it would be quite some years before anything other than local operations was supported and so the initial design hadn't actually been proven adequate to support that; it took some further iteration to reach a design suitable for supporting remote operations too.

Today's OpenTofu contains the result of the various later small iterations on this initial design.

State storage is still handled using a largely-unchanged "state manager" API, although it later gained support for state locking and so is most often implemented as [`statemgr.Full`](https://pkg.go.dev/github.com/opentofu/opentofu@v1.8.5/internal/states/statemgr#Full) which extends `statemgr.Storage` with the extra locking capabilities.

The current "backend" is an extra indirection, based on implementations of [`backend.Backend`](https://pkg.go.dev/github.com/opentofu/opentofu@v1.8.5/internal/backend#Backend). The backend is responsible for deciding which workspaces exist and for providing the "state manager" for any workspace it reports as existing.

[`backend.Enhanced`](https://pkg.go.dev/github.com/opentofu/opentofu@v1.8.5/internal/backend#Enhanced) is an optional extension interface that is implemented by backends that know how to perform "operations", which is the backend API terminology for each of the individual main workflow CLI commands: `tofu plan`, `tofu apply`, etc. In practice there are only a small number of backends that actually implement this interface: `local`, `remote`, and `cloud`. If the configuration selects any other backend then what they are really doing is selecting the `local` backend but configured in a special way to delegate all of the workspace-related methods to some other `backend.Backend` implementation.

```mermaid
flowchart LR
    CLI[OpenTofu CLI] -->|operations| L[local backend] -->|workspaces and state| S[s3 backend]
```

When selecting one of the `remote` or `cloud` backends (which both do essentially the same thing), the situation gets more complicated. These backends both implement `backend.Enhanced` and so the CLI layer delegates to them directly to perform "operations". Both of these backends then use information from a remote API to choose between running the requested operation locally or remotely. If the decision is to run remotely then the backend essentially just asks a remote API to begin the operation and polls for it to complete. If the decision is to run _locally_ then these backends both delegate back to the "local" backend, but configured to use the original remote/cloud backend for workspaces and state, leading to the following situation:

flowchart LR
    CLI[OpenTofu CLI] -->|operations| L[remote backend] -->|"operations (delegated)"| D[local backend] -->|workspaces and state| S[remote backend]

The remote backend here is, in some sense, talking to itself indirectly through the local backend. This particular situation is often surprising for newcomers to this codebase, because the internal modelling of these concepts is so far divorced from how they are presented to end-users.

Most of the feature requests we currently aim to address for backends relate to using the local backend with various different kinds of state storage. The remote operations features currently support only a single vendor's proprietary API, and so are perhaps not used as much as they might be if we had a vendor-agnostic, documented API as requested in [Document how someone could implement their own cloud backend](https://github.com/opentofu/opentofu/issues/960).

## Proposal

The following sections summarize some specific changes we propose to make, likely over the course of several releases, to gradually meet the various feature requests that motivated this proposal. We also hope to improve the overall technical design of "backends" gradually along the way.

Each of the following sections is expected to lead to at leasst one detailed feature RFC of its own, but we hope that summarizing them all here in one place will help in gradually building toward a better system without earlier changes significantly impeding later changes.

### State Storage as Plugins

We propose to introduce a similar plugin-based model for state storage, with backend plugin installation offering _at least_ all of the same installation method flexibility and package authentication mechanisms we currently offer for providers.

Supporting the automatic installation of third-party plugins as _providers_ has allowed the core team to focus primarily on the general language runtime, execution engine, and CLI workflow while delegating the integrations with specific remote APIs to other teams that have specific expertise in those systems.

State storage backends, on the other hand, are currently compiled in to OpenTofu CLI directly and therefore maintained primarily by the core team. This presents a number of challenges, including the following (in no particular order):

- Time spent working on a specific backend benefits only those who use that backend, whereas other core work tends to have broader benefit and is therefore more likely to be prioritized with our limited resources.

    We are therefore quite conservative in what changes we might accept to the backends, and particularly conservative about adding new ones for systems where we don't have direct expertise.

- Many backends can only be properly tested using a real account with a specific remote system, or by installing specific software.

    A team that is focused on developing a specific provider or backend can typically justify the setup cost and ongoing costs for such an environment, particularly if that environment is in relatively constant use, but having them all together in one codebase means that anyone who might contribute to that codebase might potentially need to set up several different accounts or software installations that they only use occasionally and that tend to atrophy between occasional uses.

- The OpenTofu codebase has to carry quite a heavy set of dependencies that are used either mostly or entirely for specific backends.

    Each dependency we include creates additional maintenence overheads such as responding to security advisories, and bloats the size of the OpenTofu CLI executable.

A later RFC for this change will include answers to at least the following questions:

- Is "state storage" a new kind of plugin entirely, or is it just a provider plugin using some additional protocol features?

    So far we have been leaning towards extending the concept of "provider plugin" to include this new capabililty, so that we can more easily reuse all of the existing distribution infrastructure and practices we already enjoy for provider plugins.
    
    However, state storage does present some unique additional challenges we need to consider; most notably, OpenTofu uses the latest state snapshot as part of its decision about which provider plugins it needs to install, and a state storage plugin would need to be available before we could retrieve that snapshot. We also need to consider that `tofu init` today guarantees never to immediately execute any plugins it has just installed, so that operators always have a chance to review any new additions to the dependency lock file before executing them.

    We also need to consider how state storage plugins might interact with the existing `terraform_remote_state` data source. That data source is also compiled into OpenTofu as part of the `terraform.io/builtin/terraform` provider, and so it currently relies on the ability to just reach across into the CLI portion of the codebase to directly instantiate a backend. If this is to support plugins too, we'll presumably need to extend it with some way to specify which plugin is needed and then also install that plugin as part of `tofu init`. A `data` block in the configuration causing an additional plugin dependency (aside from the provider that implements it) is completely unprecedented, so this might be a good justification for introducing "fetch outputs from a state snapshot stored elsewhere" as a first-class language feature, rather than as a (built-in) plugin feature.

- How does plugin-based state storage impact the configuration syntax for specifying which state storage to use?

    Today we have just a flat namespace containing names like `local`, `s3`, `consul`, etc, which relies on the fact that these are all distributed as part of the OpenTofu CLI executable. Supporting an open ecosystem seems likely to require specifying provider-plugin-like source addresses, and somehow specifying those as part of the root module's configuration, which likely requires some new syntax.

- Do we want to change any design details about the "state storage" API before we fix it as an external compatibility constraint?

    So far we've enjoyed the freedom to make arbitrary changes to the internal API between OpenTofu and the state storage implementations because we can change both callers and callees simultaneously in a single git commit. For example, the addition of client-side state encryption extended the state storage part of the backend API to pass the configured encryption implementation to the state storage implementation.

    Once we document an external-facing protocol for third parties to implement, it will become considerably harder to evolve that protocol, and thus to introduce new features or new design ideas over time.

    Some potential questions we've discovered so far, but not yet answered, include:
    - Should encryption implementations be plugin-based too? If so, is that something we'd include as part of the _state storage_ plugin API, or as a separate orthogonal API?
    - Is it valid to assume that all state storage implementations can just store arbitrary, opaque byte arrays? Relatedly, is "one big blob" the model we want to force on _all_ state storage implementations?

        Some potential storage backends prefer to store many small objects rather than one large object. For example, the existing `consul` backend implements somewhat-arbitrary chunking to work around the fact that Consul's key/value store was designed for small strings rather than large blobs. We could potentially make OpenTofu itself aware of this need for splitting the artifact into smaller parts, and thus be able to split it in a smarter way than just fixed-sized subslices of a byte array. Should we try to design the protocol to support that now, or can we wait to extend later once we understand those use-cases better?

        A related concern is that repeatedly overwriting a single large object creates conflicting pressures: we want to write as often as possible during apply to make sure that a local crash or remote failure loses the smallest set of state changes possible, but writing state snapshots can be quite expensive (both in time and in money) and so it's currently assumed to be undesirable to write new snapshots _constantly_ after every small change. A model where the core runtime emits a log of _individual changes_ to the state, rather than constantly emitting copies of the entire state, would give state storage plugin implementers additional options when making these robustness vs. performance tradeoffs.
    - Should state locking always be provided by the same plugin that implements the state storage?
    
        Blob storage systems don't always offer locking primitives, and systems that support locking primitives aren't always good at storing big blobs, so it could potentially be useful to treat locking as a separate concern from storage to allow users to more flexibly mix solutions. However, _some_ locking strategies can only really work if they are implemented by the same component that implements the storage, such as directly locking a local state snapshot file using the operating system's own locking mechanisms.

        A review of both the existing options for storage and locking, along with other options that have been requested or seem plausible to be requested in future, could help to answer this question.
    - Should this API allow using "state storage" plugins also for storing _saved plans_ somehow?

        That capability is currently reserved only for "enhanced backends", since they get to oversee the whole execution of the plan and apply phases. It's most commonly used in conjunction with a remote collaboration system (so-called "TACOS") which is then able to provide added value based on the saved plan, such as policy enforcement. Can we identify any use-cases for saving state snapshots in a store that _isn't_ specifically designed for OpenTofu, like Amazon S3?
    - Is the "state storage" plugin responsible for deciding which workspaces exist?

        In today's architecture it's the "backend" which decides on the set of workspaces, and then each state manager only deals with the state for one workspace at a time. However, in practice the "local" backend delegates to whichever other backend has been bound to it for state storage to answer that question, and so it's a little unclear architecturally which component actually owns this responsibility today.

        The main argument for letting the state storage plugins decide this is that it's the closest design to the current behavior: if you configure `backend "s3"` then it's the content of your S3 bucket that decides which workspaces exist, and if it's a state storage plugin interacting with the S3 bucket then that plugin is presumably the only component that could actually answer that question.

        However, an argument against is the occasional reasonable request for us to support using different state storage implementations for different workspaces. For example, to use the local filesystem for a temporary development workspace used only by one developer but to use real remote state storage for a shared workspace that is shared across a team. There are probably other ways we could meet that use-case, such as by adding "temporary development workspace" as a first-class feature that's independent of state storage plugins and just _always_ uses the local filesystem, but we should consider this at least enough to decide whether the answer impacts our plugin protocol design.
    - Can a "state storage" plugin contribute additional input variable values to a plan?

        Again this is a capability currently reserved only for "enhanced backends", used to allow input variable values to be configured in a central location where they can be maintained as live data rather than as code. [Conditionally load tfvars/tf file based on Workspace](https://github.com/opentofu/opentofu/issues/1053) could be considered a request for this, though that request is scoped only to _local files_ and so we could reasonably decide that choosing the input variables is still an "operations"-level concern rather than a "state storage" concern, and so implement that feature as part of what we currently call the "local" backend (regardless of which state storage plugin is selected).

    (There are probably other similar questions we've not discussed yet which will come to light as we research for writing this RFC.)

### A vendor-agnostic API for tighter integrations

The "remote state" mechanism is widely used, and is designed primarily for situations where someone wishes to use a generic data store -- one not designed with OpenTofu in mind -- as a place to store state snapshots (and possibly to coordinate access to them using locks).

Some vendors wish to go further and offer services that are designed specifically _for_ OpenTofu (and possibly other similar software). These systems typically still include some form of state storage, but also attempt to augment the OpenTofu workflow with additional features to aid in team collaboration, enforcing compliance requirements, coordinating changes between multiple different OpenTofu configurations (and other software), and so on.

Currently in OpenTofu this requirement is met using either [the `remote` backend](https://opentofu.org/docs/language/settings/backends/remote/) or [the special `cloud` block](https://opentofu.org/docs/language/settings/tf-cloud/), which (for historical reasons not known to us) are different implementations of essentially the same behavior.

In both cases, the user-facing interface takes a single hostname and uses [our service discovery protocol](https://opentofu.org/docs/internals/remote-service-discovery/) to abstract away the details of exactly which protocols and base URLs are to be used:

```hcl
  cloud {
    hostname     = "opentofu.example.com"
    organization = "example"

    workspaces {
      # (various ways of selecting a set of workspaces that are in scope
      # for this configuration.)
    }
  }
```

However, the current implementation only supports the proprietary API for a single vendor, which was not designed for implementation by other vendors. The schema expected in the `workspaces` block only supports concepts employed by that specific vendor, and so even with reverse-engineering it's challenging to map this implementation onto other remote system designs. We'd ideally like to open this integration point to a variety of different vendors that each offers a different assortment of services, which is represented by the request [Document how someone could implement their own cloud backend](https://github.com/opentofu/opentofu/issues/960).

Whereas the previous section proposed introducing plugin-based state storage, the integration points for this tighter sort of integration are more tightly coupled to OpenTofu and tend to require a remote API that is already at least somewhat tailored to OpenTofu's concepts.

Plugins are a useful mechanism for integrating OpenTofu with external systems that are _not_ directly made for OpenTofu because they allow the plugin to potentially be maintained by a group that's independent of the system they integrate with, but if a particular vendor is already building an OpenTofu-specific service accessible over the network then it makes for an overall simpler system design to have all of the vendor-specific logic live inside their server, and have OpenTofu act as a direct client to that server without any plugin intermediating.

A future RFC for this should define a new network protocol, designed under similar principles to our existing [`modules.v1`](https://opentofu.org/docs/internals/module-registry-protocol/) and [`providers.v1`](https://opentofu.org/docs/internals/provider-registry-protocol/) protocols, and define how support for that protocol can be announced via [the service discovery protocol](https://opentofu.org/docs/internals/remote-service-discovery/). In particular, the new protocol should ideally use the same model for authentication credentials that's used for the two registry protocols, so that vendors that offer both OpenTofu collabration services _and_ registry services can offer them all on the same hostname and thus present a unified user experience that makes them all appear as if a single service.

The protocol should allow different vendors to offer different subsets of the available functionality, so that each vendor can prioritize which subset best fits their overall product vision and so that enterprises can develop their own in-house implementations, if desired, which focus only on the subset of functionality needed for their own workflow.

The RFC should also describe how the internal implementation will use the service discovery result to choose between using either the new protocol or the legacy vendor-specific protocol. From an end-user perspective this should ideally be automatic, requiring no special configuration beyond what's already required. In particular, it should be possible for a vendor that's currently offering an implementation of the vendor-specific API to transition to using the new API (which would ideally offer additional features that are not in the legacy protocol over time) without requiring any changes to end-user configuration files.

Finally, the RFC should describe some way for an implementation of the protocol to choose which arguments are available in the `workspaces` block when their implementation's hostname is specified, so that each implementation can expose workspace selection arguments that closely follow their own product's concepts. The remote API is then responsible for deciding which workspace names to present to return from commands like `tofu workspace list` based on the values assigned to their chosen arguments.

Several existing vendors seem to have already partially-implemented the existing vendor-specific API by reverse-engineering, and so if those vendors are willing we should consult with them to learn which parts of the API they are currently making use of, whether there are any significant "gaps" in the functionality that they'd like to see filled, and overall to make sure we're designing a protocol that they could potentially migrate to non-disruptively in future.

The surface area of "cloud integration" functionality is quite broad, and it seems that existing vendors making use of this abstraction are each using different subsets of it already. If that is true, it would also be helpful for the RFC to propose a phased implementation approach which focuses on meeting the most important use-cases first while leaving room for more complex additions later. For example, we expect that enumerating workspaces and storing/retrieving state snapshots is fundamental and therefore baseline functionality, but that remote operations are less widely implemented due to the significantly higher complexity of doing so. If that assumption turns out to be true then it would be helpful to omit remote operations support from the initial implementation, but to make sure we have at least one possible API extension point to introduce it later without breaking changes to the API.

#### The future of the `http` backend

It's somewhat common today to use the `http` backend as a "last resort" state storage implementation in situations where an organization wants a customized experience more like the `cloud` block (or `remote` backend) but does not wish to reverse-engineer and reimplement the relevant subset of the vendor-specific API that's currently supported.

A key advantage of the `http` backend is that it allows a user to configure arbitrary URLs and HTTP methods to use to perform each needed operation, and therefore it can be coupled with a custom-made server implementation to achieve a customized integration.

A prominent example of this strategy is [GitLab-managed Terraform State](https://docs.gitlab.com/ee/user/infrastructure/iac/terraform_state.html) (also compatible with OpenTofu), which is implemented as a special API endpoint designed to work with a specific configuration of the `http` backend. This is a pragmatic solution with OpenTofu's current featureset, but it pushes various uninteresting configuration details into the `backend "http"` block and would be considerably more ergonomic as an implementation of the protocol discussed in the previous section which can self-configure using service discovery. (`gitlab.com` already hosts a service discovery document for [its module registry feature](https://docs.gitlab.com/ee/user/packages/terraform_module_registry/), which also already uses our cross-service model for authentication credentials.)

The `http` backend is difficult to expand with new capabilities because each new behavior requires _even more_ configuration burden to specify all of the relevant URLs and HTTP methods. [Add workspaces support to the HTTP backend](https://github.com/opentofu/opentofu/issues/317) is a popular existing request that is very reasonable to ask for but challenging to produce an ergonomic design for.

Either the same RFC defining the new cloud integration protocol, or a separate RFC referring to it, should propose how the `http` backend fits in to our vision for state storage and other "backend" behaviors in OpenTofu. In particular, we should consider whether the new cloud integration protocol is sufficient to subsume all of the `http` backend's known use-cases, and whether we could therefore deprecate the `http` backend over time and freeze it with its current featureset rather than continuing to extend it.

### Simplifying the user experience and reconciling it with the internal design

This final part is a little different since it is a cross-cutting design question rather than a specific end-user feature.

We discussed in the introduction that the user-facing model for backends doesn't actually tesselate very well with how the relevant subsystems are designed internally. While such a difference is not necessarily _wrong_ -- it's expected for an abstraction to hide implementation details, after all -- several quirks of the existing design have "leaked" into the user experience and made it perhaps more confusing than we'd like:

- The `remote` backend and the `cloud` block both cover essentially the same functionality, but in different ways.
- The `remote` backend is the only case configurable using a `backend` block that offers advanced features like remote operations. In all other cases, a `backend` block is only for configuring state storage for local operations.
- The `remote` backend sometimes internally delegates to the `local` backend, but sometimes it doesn't. This distinction seems like it was _intended_ to be just an implementation detail, but nonetheless users are often confused about whether a particular operation is happening locally or remotely, which input variable values and environment variable values are in scope, whether these decisions are being made locally or by some remote system, and how/whether they can override that decision when they need to.

The internal design is also quite confusing for code contributors, making the system more challenging to maintain and extend. There is a single concept of "backend", but overloaded with at least three mostly-independent responsibilities:

1. Which workspaces exist and how is the state stored for each one? (aka "State Storage")
2. Should an operation be run locally or in a remote system via a network API? (aka "Operations")
3. Which implementation should be used for each of 1 and 2? (aka "Backend")

Today's syntax for choosing a backend -- the label of a `backend` block -- specifies the answers to all three of those questions at once:

| `backend` label | Backend | Operations | State Storage |
| -- | -- | -- | -- |
| `"local"` | [`local.Local`](https://pkg.go.dev/github.com/opentofu/opentofu@v1.8.5/internal/backend/local#Local) | [`local.Local`](https://pkg.go.dev/github.com/opentofu/opentofu@v1.8.5/internal/backend/local#Local) | [`local.Local`](https://pkg.go.dev/github.com/opentofu/opentofu@v1.8.5/internal/backend/local#Local) |
| `"s3"` | [`local.Local`](https://pkg.go.dev/github.com/opentofu/opentofu@v1.8.5/internal/backend/local#Local) | [`local.Local`](https://pkg.go.dev/github.com/opentofu/opentofu@v1.8.5/internal/backend/local#Local) | [`s3.Backend`](https://pkg.go.dev/github.com/opentofu/opentofu@v1.8.5/internal/backend/remote-state/s3#Backend) |
| `"remote"` | [`remote.Backend`](https://pkg.go.dev/github.com/opentofu/opentofu@v1.8.5/internal/backend/remote#Remote) | [`local.Local`](https://pkg.go.dev/github.com/opentofu/opentofu@v1.8.5/internal/backend/local#Local)/[`remote.Backend`](https://pkg.go.dev/github.com/opentofu/opentofu@v1.8.5/internal/backend/remote#Remote)<br>(decided dynamically) | [`remote.Backend`](https://pkg.go.dev/github.com/opentofu/opentofu@v1.8.5/internal/backend/remote#Remote)<br>(only relevant if "Operations" is local) |

Putting aside the specific concrete implementations of `backend.Backend`, the two decisions this configuration construct (and the `cloud` block) are currently representing are:

1. Are we delegating control of the workflow to a "TACOS"-like remote API, or are we configuring everything locally?
2. _Only if we're configuring everything locally_, where are we storing the list of available workspaces and their state snapshots?

    (If we're delegating control to a remote API, then that remote API makes the decision of how to select workspaces and how to store state snapshots, along with everything else it offers, because the goal in this case is to outsource these details to an opinionated external system that can be centrally managed.)

If any other work on backends calls for introducing new syntax beyond what our current `backend` and `cloud` blocks support, that would present an opportunity to incorporate other changes to make the configuration syntax more consistent and, ideally, then align the internal architecture to that user-facing model.

Future RFCs in this area might consider some or all of the following:

- Deprecating the confusingly-overloaded `backend` block type in favor of a new block type which is exclusively for configuring a state storage plugin, and in no case can dynamically choose to perform remote operations.

    For example, a `state_storage` block type that is designed from its outset for selecting an arbitrary state storage _plugin_, and then the deprecated `backend` block being repurposed as a backward-compatibility form either for a fixed set of plugins implementing what were formerly remote-state-only "backends" or, for `backend "remote"` in particular, just a deprecated way to write a `cloud` block.
- Defining the existing `cloud` block as _the way_ to integrate with a "TACOS-like" service which takes control over workflow details such as choosing betweeen local and remote operations, with `backend "remote"` becoming just a deprecated legacy way to write a `cloud` block.
- Reorganizing the internal details so that there are three distinct interfaces -- e.g. workflow controller vs. operation executor vs. state storage -- rather than overloading the same concept to represent all three and then combining them together in non-obvious ways.
- Offering some flexibility in configuring multiple state storage implementations (or even multiple "TACOS-like" services) at once and switching between them in a similar way as we switch between workspaces, so that users can more easily use central system for shared environments while making a different decisions for transient environments used for development and testing.

    A possible analogy here is the concept of "remotes" in Git. Each remote has its own set of refs, which is perhaps comparable to how each "state storage" or "TACOS-like service" has its own set of workspaces. The analogy isn't _quite_ right because in Git we only select _local_ refs in our working directories, with remote refs only used for synchronization, but it's potentially plausible to offer some command that says "select the `foo` workspace in my `dev` state storage" or "select the `production` workspace in my `shared` state storage", effectively making the workspace selection a (state-storage-name, workspace-name) tuple instead of just a naked workspace-name as today.

## Implementation

Since this is a "Meta-RFC", its implementation consists of writing several other RFCs that together cover all of the ideas identified here.

As we author those RFCs, the pull requests that propose them should also propose to add links at suitable points in this document -- and possibly modify the text of this RFC to better suit the new RFC content -- so that by the time we are finished this document will have evolved into an overview of what we actually intended to implement, instead of a list of RFCs to write.
