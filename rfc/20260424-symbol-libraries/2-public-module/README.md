This simplified example shows a publicly consumable module exposing it's types for re-use.

Prior to the introduction of symbol libraries, consumers of modules would need to update their variable types when new features were added or removed. In a simplified example like this, it is not a huge burden. As projects and dependency chains grow however, this becomes more and more of a problem. A more "internal" version of this can be seen in the `1-org-monolith` example.

By distributing consistent and versioned types, mismatches and cross-cutting updates are much less hassle.
