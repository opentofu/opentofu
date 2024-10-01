# Equivalence Tests

The "equivalence tests" are [charactization tests](https://en.m.wikipedia.org/wiki/Characterization_test) that can compare output from a current OpenTofu version against "golden snapshots" captured against some earlier version, as some additional insurance against unintended changes.

Whereas most of our other testing strategies involve hand-written tests that aim to capture _intended_ behavior, the equivalence tests are only capable of detecting if the behavior has changed without giving any insight on whether the changes are intended or desirable. Therefore a failure of these tests cannot be assumed to be a regression, but should be carefully reviewed to ensure that the new behavior after a change does indeed match what the author(s) intended.

The equivalence testing mechanism is currently tethered deeply to our GitHub Actions workflow "compare-snapshots" and is not straightforward to use locally. The checks for each Pull Request include comparing the current output from the code in the PR against the captured snapshots.

If you find that you need to update the captured snapshots to match intentional changes of behavior, you can mimic some of the steps that [the `compare-snapshots` GitHub Actions workflow](https://github.com/opentofu/opentofu/blob/main/.github/workflows/compare-snapshots.yml) performs and then commit the result as part of your pull request:

1. Run a command equivalent to the step named "Get the equivalence test binary", adjusting the command line options to match the operating system and CPU architecture where you will be running the command. This creates `./bin/equivalence-testing`, which is the test harness.
2. Run a command equivalent to the step named "Run the equivalence tests", changing the `--binary` option to refer to the `tofu` executable you intend to use as the reference for the new correct behavior.
3. Review the diff in `testing/equivalence-tests/outputs` to verify that the changes match what you intended.
4. If everything looks plausible, commit the changes in that directory and add them to your pull request. The `compare-snapshots` workflow should then re-run and find that the current behavior matches the updated golden snapshots.

## Evaluating Detected Changes

The equivalence tests have a mixture of tests that cover machine-readable output and tests that cover human-oriented output.

Most machine-readable output is covered by the [OpenTofu v1.x Compatibility Promises](https://opentofu.org/docs/language/v1-compatibility-promises/), but what exactly is guaranteed varies depending on the feature. For example, for JSON output it's typically guaranteed that we won't remove previously-present object properties or change the types of existing values, but it typically is _not_ guaranteed exactly what order object properties would be serialized in, whether there are insignificant spaces or indentation between tokens, etc. When evaluating this it's best to consider what external software is known to be or likely to be relying on the output.

The exact details of human-readable output are explicitly _not_ covered by the compatibility promises, because that allows us to make gradual improvements to the user experience over time. When evaluating changes to human-oriented output then, the primary goal is to make sure that all of the expected information is present in a readable form and is presented correctly. It is typically not important to maintain the exact layout and formatting of existing output, although there may be some pragmatic exceptions.

If you're not sure then other forms of test such as unit tests covering the affected features can often be informative, since unlike te equivalence tests those were written with explicit human intent and so tend to capture what the designers of the feature considered to be guaranteed vs. implementation detail. Explicit community feedback and testing against prereleases can also be helpful for assessing changes whose potential impact is unclear to us.

## Output Rewrites

Various parts of `tofu` output inherently vary between runs, such as the total time elapsed during an operation.

The configuration file [`rewrites.jsonc`](../rewrites.jsonc) contains some regular expression patterns used to mask those dynamic portions of the output so that they don't create false negatives when running the tests on different systems. It might be necessary to change the rules in that file if the corresponding parts of the real `tofu` output are changed in future releases.
