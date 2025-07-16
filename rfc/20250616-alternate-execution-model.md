# Exploring a new execution model for OpenTofu

This RFC is the first of it's kind in that it primarily targets the developers and maintainers of OpenTofu, instead of users directly. The secondary target is to allow faster development (and execution) to benefit users.

As evidenced by the git history of this codebase, much of the growth has happened intentionally and organically as new requirements arose over the past ten+ years.  This structure has allowed the project to grow to it's current size, but has now calcified due to implicit and opaque complexity throughout the `tofu` package.  This leads to a significant maintenance burden, as well as a *very* steep learning curve for new developers.  Long term, this represents a significant hurdle to the viability of this project.

The heart of this problem is the graph in the `tofu` package.  It is currently challenging to maintain for the following reasons:
* Each "node" in the graph must intentionally "implement" specific interfaces to opt-in to the functionality that it needs
  - Any change to these interfaces or what nodes implement them can have drastic and hard to reason consequences
  - Many of these interfaces are not documented well, or the documentation is subtly out of date
* The graph is built by transformers, who's interactions are complex and difficult to reason about in a chain
  - Ordering of transformers is incredibly important, but locks the design of future components
  - Minor changes to a single transformer can subtly break several down the chain with edge cases
* Expansion was added after most of the project in it's current iteration was designed
  - We now have to reason about subgraphs of subgraphs, which are different between different execution operations (validate, plan, apply)


More generally, the problem we are trying to address is:
* Needing to keep a vast and clear mental model of the whole system when making any change
* Attempting to make the code easier to mentally trace, with clearly defined responsibilities and boundaries


Any solution to this problem should meet the following requirements:
* Co-exist with the existing `tofu` package (allowing opt-in via flag)
* Re-use as much of the `tofu` package as possible
* Not result in a reduced set of functionality
* Bug-for-bug compatibility, or document the squashing of the bug
* Consider any potential restrictions on new features in the proposed architecture
* Not significantly block or impact ongoing development

## Proposed Solution

During the Spring 2025 Hackathon, @cam72cam spent the week experimenting with different approaches to modeling the "execution of operations" in OpenTofu. The conclusion reached was that the graph building and walking code could be sidestepped by a new execution model, while leaving the existing `tofu` package logic in-tact.  If a new and simpler execution model could be created, it has a chance to meet the above criteria.

### Prototypes

All of the initial prototypes followed a model of "direct" discovery of dependencies, instead of pre-computing references. They follow a more "JIT" model of execution, compared to building and transforming a graph which is then walked.  The execution of a "node" will block until dependencies are available or a cycle is detected.

At the time of writing, the prototype phase is still on-going and therefore both represents documentation of how different paths may solve this solution as well as a bit of a dev-log. This will likely be moved somewhere else and linked when the RFC is closer to completion and more concrete prototypes have been identified and reviewed.

#### Hackathon Prototype

This [first prototype](https://github.com/opentofu/opentofu/compare/provider_experimentation_schema...cam72cam:opentofu:hackathon_super_object) is *very* crude, but served as the initial exploration of this concept.  Even among this prototype, there were quite a few paths explored that did not end up panning out (as evidenced by the git history).

The general model of this prototype is that engine.Walk creates a instance of the root module, based on the configuration and state available.  A function is also created which represents the execution of the "action" (validate,plan,apply), which performs the bulk of the work.  The initial root module instance only contains a scope of promised objects that may be resolved during the action.

The root action iterates through all potential work to be done in the model: handling resources, following expansion, etc. This recurses through the module calls and their expansions in a depth first search (with some parallelism sprinkled in).

Behind the scenes, a promise-like model was adopted and embedded within the execution context given to the `tofu` nodes. It contains an understanding of what node it represents and can follow a promise chain with a stack trace to identify circular dependencies when they occur.  In practice, the developer facing code is not drastically impacted by this change.

Discoveries:
* A reasonably clear boundary exists between the backend calling into the graph and the actual execution of OpenTofu nodes
* This model can be clearly explained to other developers in a very short amount of time (drastically lower learning curve)
* Initial performance testing (with a subset of functionality) hints that we leave a *lot* of performance on the table
* A potential technique is to only give the executing node a small state/change object that is generated on the fly
    - This allows better sandboxing and control over the nodes, but introduces a significant dev burden and brittleness
    - This was inspired by other work on granular state storage

Short-comings:
* Handles apply with the same pattern as plan, instead of driving from Changes
* Does not consider multiple states during plan (Pre, Refresh, State)
* Sidesteps provider requirements / configuration due to time constraints
* Does not handle resource depends on into child modules
* Parallelism is unwieldy and overly aggressive
* There is no method to discover what nodes "depend on" a given node in this model
  - This *could* pose limitations on implementations of existing features, create_before_destroy, ephemeral, etc...

#### First Refinements

In identifying the different patterns needed for plan and apply, a branch was taken that [split the execution model in a very direct way](https://github.com/cam72cam/opentofu/compare/hackathon_super_object...cam72cam:opentofu:hackathon_super_object_granular).  This was overly verbose and not a great abstraction, which lead to [a reversion back to a much simpler model](https://github.com/cam72cam/opentofu/compare/hackathon_super_object_granular...cam72cam:opentofu:hackathon_super_object_simple) in which the ongoing operation is used to drive the execution path.

This approach re-uses much more of the existing `tofu` package and does not try to replace the state and change access mechanisms (for better or worse) and is conceptually the clearest model so far.

#### Ongoing Development

@cam72cam is currently investigating the following:
* How to correctly implement the apply path (derived from changes)
  - How does a node reference elements in a child module call? (depends_on)
  - How should destroy function (inverted dependency direction)
  - Is create_before_destroy possible in this execution model?

### Open Questions

* Is attempting this a worth-while investment for the limited resources of the OpenTofu project?
* What future considerations are we forgetting?
* What path do we take if the alternate execution model can not reach the above milestones in a timely manner?
* Do we set a time limit on how long the two execution models should exist within the codebase?
* Will we end up with something equivalent in complexity and burden of maintenance?

### Future Considerations

* Granular State Storage / Locking
* Unknown Inputs
* Deferred Actions

## Potential Alternatives

A gradual refactoring and improvement of the existing `tofu` package could be performed, with lessons learned from the alternate execution model. The challenge with this approach is that due to the considerations listed above, any change to this package's structure can have far reaching and hard to predict ramifications.

