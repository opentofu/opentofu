This example shows the story of a user starting to expand a project. Their project involves creating and managing multiple cloud resources, with a different user access policy for both primary and secondary systems. Only the policy validation and creation code is included, the rest is left to the readers imagination.

The user starts out in the [./copypaste](./copypaste) directory, they have refactored their configuration from a single system into "primary" and "secondary" systems. The easiest option here is to copy-paste, but it leaves a bad taste in their mouth as any updates to primary need to be considered and applied to secondary as well.

They discover modules and refactor their code to use a local helper module in the [./module](./module) directory. This simplifies the validation logic, but requires the variable type to be duplicated even more times and makes comprehending the structure of the project a bit more difficult (subjective, but real feedback from users).

Once symbol libraries are introduced, the user is able to re-structure in a way that eliminates code duplication and improves readability in [./symbols](./symbols). They factor out the types, validation logic, and policy formatting, leaving the main.tofu in a simple to read and minimal state.
