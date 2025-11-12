# OpenTofu Diagnostics Guide

"Diagnostics" is the general term we use to describe the error and warning
messages that OpenTofu returns when there are problems with the configuration,
or when interactions with external systems fail.

This document is an overview of how we typically use diagnostics in OpenTofu.
It includes both some technical information about how we represent diagnostics
in code, and some more subjective information about the writing style we most
often use in diagnostic messages.

## Diagnostics in Code

Diagnostics are modelled using the types from
[the `tfdiags` package](https://pkg.go.dev/github.com/opentofu/opentofu/internal/tfdiags).

In particular:
- [`tfdiags.Diagnostics`](https://pkg.go.dev/github.com/opentofu/opentofu/internal/tfdiags#Diagnostics)
  represents a set of zero or more diagnostics.

    A total lack of diagnostics is usually represented by a `nil` value of this
    type.

    When constructing sets of diagnostics to return we typically don't worry
    about the order they are returned in, even though we return them using a
    slice type. The UI-layer code uses
    [`tfdiags.Diagnostics.Sort`](https://pkg.go.dev/github.com/opentofu/opentofu/internal/tfdiags#Diagnostics.Sort)
    to place all of the collected diagnostics into a predictable order before
    rendering them, and so that function effectively turns the set of
    diagnostics into an ordered list of diagnostics _just in time_.

- [`tfdiags.Diagnostic`](https://pkg.go.dev/github.com/opentofu/opentofu/internal/tfdiags#Diagnostic)
  is an interface type that all diagnostic values implement.

    In practice values of this type are often created automatically as an
    implementation detail of [`Diagnostics.Append`](https://pkg.go.dev/github.com/opentofu/opentofu/internal/tfdiags#Diagnostics.Append),
    which accepts various types that _don't_ directly implement
    `Diagnostic` and then automatically wraps them in a type that does.
    In particular:

    - We often use [`hcl.Diagnostic`](https://pkg.go.dev/github.com/hashicorp/hcl/v2#Diagnostic)
      to describe problems related to the configuration or operations that are
      strongly related to parts of the configuration, because it is the most
      fully-fledged type of diagnostic we allow including support for source
      ranges and relevant expressions as described later.

        It's also acceptable to append a whole `hcl.Diagnostics` (the HCL
        equivalent of `tfdiags.Diagnostics`) in which case each diagnostic
        will be wrapped and appended in turn. This is common when calling
        HCL's own functions and passing on its diagnostics verbatim.
    - Normal `error` values can be appended to a `tfdiags.Diagnostics`, but
      that's mainly for historical reasons -- adapting code that was present
      before the diagnostic models were added -- and should not be used in new
      code because it typically results in low-quality diagnostics that don't
      meet the style guidelines later in this document.

        One exception is for "should never happen" cases: we sometimes use
        `error` directly in that case to avoid overwhelming the surrounding
        code with the construction of a full diagnostic.

    Package `tfdiags` also includes some functions for constructing other kinds
    of diagnostics, including:

    - [`tfdiags.Sourceless`](https://pkg.go.dev/github.com/opentofu/opentofu/internal/tfdiags#Sourceless)
      is good for diagnostics that don't relate to any part of the configuration,
      such as when reporting incorrect usage of a command line argument.
    - [`tfdiags.AttributeValue`](https://pkg.go.dev/github.com/opentofu/opentofu/internal/tfdiags#AttributeValue) and
      [`tfdiags.WholeContainingBody`](https://pkg.go.dev/github.com/opentofu/opentofu/internal/tfdiags#WholeContainingBody)
      produce special "contextual diagnostics" that must be transformed by
      calling [`InConfigBody`](https://pkg.go.dev/github.com/opentofu/opentofu/internal/tfdiags#Diagnostics.InConfigBody)
      on the resulting `Diagnostics` value. This is a special mechanism used
      when the subsystem generating the diagnostic does not have direct access
      to the configuration itself, such as when a provider returns a diagnostic
      via the provider wire protocol.
- [`tfdiags.Severity`](https://pkg.go.dev/github.com/opentofu/opentofu/internal/tfdiags#Severity)
  (and its HCL equivalent [`hcl.DiagnosticSeverity`](https://pkg.go.dev/github.com/hashicorp/hcl/v2#DiagnosticSeverity)) 
  are how we distinguish between "error" and "warning" diagnostics.

  The `tfdiags.Diagnostics.HasErrors` method returns true if the diagnostics
  contains at least one with the severity `tfdiags.Error`.

The most common pattern for handling diagnostics in code is:
1. Declare `var diags tfdiags.Diagnostics` at the very start of a function.
2. During the function's body, whenever calling another function that might
   produce its own diagnostics, capture them into a separate variable
   (often called `moreDiags`, or `hclDiags` if the return type is
   `hcl.Diagnostics`) and then immediately append them to the main `diags`
   using `tfdiags.Diagnostics.Append`.

    If subsequent code depends on the success of the call, check
    `moreDiags.HasErrors()` (or similar) and return early if it returns `true`.
3. If the function generates any diagnostics of its own, append them directly
   to `diags`.
4. At all exit points of the function, return `diags` regardless of whether
   it has been assigned to or whether it contains errors. This ensures that
   we always return any warnings that might have been produced and avoids
   the risk of missing certain return paths under future maintenance if we
   introduce additional diagnostics later.

Here's a code-example version of the above advice:

```go
func Example() (anything, tfdiags.Diagnostics) {
    var diags tfdiags.Diagnostics

    somethingElse, moreDiags := otherFunction()
    diags = diags.Append(moreDiags)
    if moreDiags.HasErrors() {
        // NOTE: it isn't _always_ necessary to return immediately when there
        // are errors, as long as the callee clearly documents what it
        // guarantees about an errored result and the caller is able to
        // work within those limitations. Collecting multiple errors to
        // return together is often desirable.
        //
        // If the caller cannot continue at all though, or if continuing is
        // likely to cause redundant errors that just restate the same problem
        // in more confusing terms, then...
        return nil, diags
    }
    if isProblematic(somethingElse) {
        // A function might need to generate its own diagnostics if it detects
        // a problem directly.
        diags = diags.Append(&hcl.Diagnostic{
            Severity: hcl.DiagError,
            // ...
        })
        return nil, diags
    }

    // ...

    // The final return statement should include diags even if no errors
    // were detected along the way, because it might contain warnings.
    return something, diags
}
```

Some functions diverge from this pattern for special reasons, such as capturing
multiple sets of child function diagnostics and then using some logic to decide
which ones to append, or processing multiple items in a loop and appending
new diagnostics for each iteration. The above is just a general example of the
most common case, not a fixed template to follow in all cases.

## Information in a Diagnostic

The general model of `tfdiags.Diagnostic` has the following parts, though not
all implementations of the interface make use of all of them:

- Severity: either `tfdiags.Error` or `tfdiags.Warning`.
- Description: the main human-readable text describing the problem. This
  has the following fields:

    - Summary: A short, terse description of the general type of problem
      that has occurred.
    - Detail: A longer description of the problem, sometimes including multiple
      paragraphs of information.
    - Address: The address of some object that the error relates to, which
      is most often a resource instance address.

    OpenTofu does not currently have a localized UI, so built-in diagnostics
    always have their summary and detail written in US English. There's more
    subjective guidance about the content of these fields in sections below.
- Source location information: optional references to parts of the configuration
  that the problem relates to. This has the following fields:

    - Subject: source range for the part of the configuration that caused the
      problem or that the problem is directly about.
    - Context: optional source range of a larger section of configuration that
      might make the cause of the problem easier to quickly understand if
      included in the diagnostic message. The Context source range must always
      contain the Subject source range within it.

    The UI uses the context and subject together to display a source code
    snippet. The lines of code included in the snippet cover both the context
    and the subject, and then the subject itself is rendered with an underline
    if we're rendering into a terminal that supports that style.

    We don't use "context" very often, but it can be useful if the problem
    we're describing is that just one part of a larger source element is
    problematic. For example, if one of the operands to the `+` operator
    isn't a number then that operand would be the "subject" but the entire
    addition operation could be returned as "context", so that both of the
    operands and the `+` symbol will definitely be included in the rendered
    diagnostic too.
- Expression-related information: optional information about an expression whose
  evaluation cause the problem. This has the following fields:

    - Expression: The `hcl.Expression` representing the expression itself.
    - EvalContext: The `hcl.EvalContext` that the expression was being evaluated
      in.
    
    The diagnostic renderer for the UI uses this information, when available,
    to offer some extra hints about the values of any symbols that were used
    in the expression, because it's often the dynamic values that cause a
    problem, rather than the syntax used to obtain them.
- Extra info: this is a rather underspecified collection of assorted other
  information that's only relevant in very specific contexts. Refer to the
  `tfdiags` package documentation for more information.

    There's _some_ guidance on this later in this document, but it's focused
    only on a few main cases.

## Diagnostic Description Writing Style

Although there is some variation in diagnostic writing style, particularly in
parts of the system like state storage backends which were originally written by
third-parties, most of the _built-in_ diagnostics follow a relatively consistent
writing style that is in turn based on the writing style used by HCL itself in
its own diagnostics, because HCL and OpenTofu diagnostics often mix together
in the same set of problems.

The "summary" should typically be a very short and concise description of
what was wrong and what was wrong about it. Our summaries typically don't
include any user-chosen information such as symbol names, because that means a
particular kind of problem is always described using the same text and so
readers can become familiar enough with the summaries of problems they see
frequently to skip reading the rest of the diagnostic when skimming.

The following are some real examples of summaries currently used across both
HCL and OpenTofu:

- Unsupported operator
- Duplicate argument
- Invalid index
- Unexpected end of template
- Invalid template interpolation value
- Invalid default value for variable
- Required variable not set
- Invalid "count" attribute

The "detail" text is where we tend to put most of the information, and so
there's a lot more variation here but ideally a good diagnostic detail
should mention the following information, usually in the following order:

- What was wrong and what was wrong about it: similar to the summary but this
  time including information about specifically what was wrong, such as the
  name of the input variable whose default value was invalid.
- Why the situation is problematic, if knowing that relies on some
  characteristic of OpenTofu's design that might not be obvious to a newcomer.
- What should be done to fix it, or (if it's unclear what the author's intention
  was) a question-sentence that implies a _possible_ solution, often starting
  with the words "Did you mean" and ending with a question mark.

While the summary message is often terse and uses only minimal punctuation,
the detail message should always be written in full sentences including
end-of-sentence punctuation (`.`, `?`). If "what was wrong about it" is
coming from the string representation of an `error` value, we typically
present it with a prefix ending with a colon and then append a period `.`
after the error string, and format the error itself using `tfdiags.FormatError`,
like this:

```go
    Detail: fmt.Sprintf("Unsuitable value for thingy: %s.", tfdiags.FormatError(err))
```

If the second and third items in the above take more than a few words, it's
helpful to split them into their own paragraphs for easier scanning. When
writing multiple paragraphs in a detail message they should be separated by
`\n\n` -- two newline characters.

In many cases our diagnostics only include a subset of this information because
either the reason why it's problematic is relatively clear or because we don't
have any specific suggestion for how to solve the problem, but the following
is an example of a real diagnostic message from OpenTofu at the time of writing
this documentation which includes all of these parts:

```
Error: Invalid for_each argument

The "for_each" map includes keys derived from resource attributes that cannot
be determined until apply, and so OpenTofu cannot determine the full set of keys
that will identify the instances of this resource.

When working with unknown values in for_each, it's better to define the map keys
statically in your configuration and place apply-time results only in the map
values.

Alternatively, you could use the planning option -exclude=aws_instance.example
to first apply without this object, and then apply normally to converge.
```

The text immediately after "Error:" above is the summary for this diagnostic.
The paragraphs that follow are all a single "detail" string.

That was a particularly extreme diagnostic message with lots of information to
communicate. Most diagnostics are not so complicated; the following is an
example with less information to communicate:

```
Error: Invalid value for input variable

The given value is not suitable for var.example declared
at example.tf:12,1: a string is required.
```

This example also illustrates a situation where there are two different source
locations that could be relevant: the input variable's declaration or the
expression that's used to define its value. Because this message is talking
about a problem with the _value_, the diagnostic should have the source
"Subject" set to the expression that defined it, but it also mentions the
location of the declaration as part of the detail text as some additional
context.

Some other notes about some other specific situations that arise sometimes:

- If a diagnostic message includes a suggestion for a shell command to run
  or a URL to visit for more information, use a paragraph that ends with a
  colon, followed by a single newline, four spaces for indentation, and then the
  command or URL:

    ```
    To view the root module output values, run:
        tofu output
    ```

    The goal of this formatting is to make it very clear what part of the
    message is intended to be copied and used elsewhere, by placing it on a
    line of its own without any surrounding punctuation. The indented text
    should ideally be formatted so that the user can copy it _verbatim_ into
    whatever place it will be used.

    The diagnostic renderer also has a special case where it will not try to
    word-wrap a line that begins with spaces, and so this layout has the
    useful side-effect of avoiding introducing extra newline characters into
    a command line that is intended to be copied.

- There are some terminology choices we use to refer to some OpenTofu-specific
  ideas and concepts that disagree slightly with terminology used in the code.
  These differences are the result of learning from feedback from folks who
  had been confused by the original terminology, even though the code still
  often uses the original terminology:

    - Instead of referring to "unknown values" or "computed values" we say that
      values are "known after apply" or "cannot be determined until apply".
    - In HCL the word "variable" means anything that's available to refer to
      in the current evaluation context, which is confusing because OpenTofu
      itself uses that word to refer only to input variables.

        Sometimes messages are generated by HCL itself and so it's unavoidably
        confusing, but when we're generating messages _inside OpenTofu_ we
        use the two words "input variable" to refer to an input variable,
        and "symbol" or "object" (depending on whether we're talking about
        the name itself or what the name refers to) as the general word for
        something you can refer to in an expression.
    - For consistency with our use of "input variable" to distinguish from
      HCL's more general meaning of "variable", we also tend to write
      "local value" and "output value" when referring to those concepts, rather
      than using the shorthands "locals" and "outputs".
    - HCL distinguishes between "attributes" meaning the named keys inside an
      object type, and "arguments" meaning the names used for individual
      settings inside a configuration block.

        OpenTofu itself uses those words a little more interchangeably because
        in _many_ cases the configuration arguments in a block directly
        correspond to the attributes of an object created by evaluating that
        block.

        However, if a particular error message is talking about a configuration
        setting inside a block it's better to use "argument" rather than
        "attribute" because that's then consistent with error messages that
        HCL itself might generate.

        Go uses the term "field" to describe an element of a struct type, and
        JavaScript and JSON use the word "property" to describe an element of
        an object type. We don't use either of those words in OpenTofu: the
        elements of an object are its _attributes_, and the settings available
        in a configuration block are its _arguments_. The string values that
        identify elements of a map are called "keys".
    - The `cty` terminology "marks" or "value marks" refers to an implementation
      detail that should never be mentioned directly in an error message.

        Instead, we use specific terminology related to what each mark type
        is representing: "sensitive values", "ephemeral values", etc.
    - `aws_instance` is an example of a "resource _type_", not of a "resource",
      even though the provider protocol uses the single noun "resource" to refer
      to both ideas.

        A "resource" is what's declared by a `resource`, `data`, or `ephemeral`
        block. A "resource _instance_" is what such a block can declare zero
        or more of, when using the `count`, `for_each`, or `enabled` arguments.
    - Although there are certainly some historical diagnostic messages that
      predate this adjustment of terminology, new error messages should use
      "managed resource" to refer to the kind of resource that's declared
      using a `resource` block, "data resource" for `data` blocks, and
      "ephemeral resource" for an `ephemeral` block.

        In the code we refer to these three as "resource _modes_", but that is
        internal terminology that should never appear in a diagnostic message.
- When a file or directory path appears as part of a diagnostic message, it
  should typically be presented relative to the current working directory and
  should use the syntax conventions of the platform where OpenTofu is running.

    In particular, we return paths using backslashes as the separator when we
    are running on Windows, but normal slashes otherwise. Using the Go
    `filepath` package is a good way to get this right, though you might need
    to add some complexity to your tests to make them pass on all platforms.
- If an error message is describing a "should never happen" case, we typically
  end the detail string with the sentence "This is a bug in OpenTofu.". This
  hopefully prompts the reader that this wasn't directly caused by something
  they did, and so they should probably open a bug report in the
  OpenTofu repository instead of just trying to solve it themselves.

    For this kind of error message we often relax our preference against
    mentioning implementation details in the error message, because the most
    likely next step is for the user to copy-paste the entire message into their
    bug report text and so the final reader of the message is OpenTofu
    maintainers rather than OpenTofu users.
    
    For example, it can be okay to use internal terminology like "cty marks" and
    use the `GoString` representations of values in a "This is a bug in
    OpenTofu" detail message, if that's the most concise way to capture the
    information the OpenTofu maintainers would need to debug the problem.

## Diagnostics caused by unknown or sensitive values

When a diagnostic has expression information associated with it, the diagnostic
renderer for the UI includes some additional information about the values
that were in scope, like this:

```
    var.greeting is "Hello"
    var.items is list of string with 5 elements
```

By default, this renderer will not mention any symbol which refers to an unknown
or sensitive value. That was not historically true: originally, this could
say something like "var.example is a string, known only after apply".

Those who are less familiar with these concepts often misunderstood the
"known only after apply" part of the message as being _the problem itself_,
rather than just context to help diagnose the problem, and so the UI no longer
mentions "unknown-ness" or "sensitive-ness" in most cases.

However, there are some diagnostics messages that _are_ directly caused by the
presence of an unknown or sensitive value, in which case it's helpful to
mention that in the summary of values that were in scope.

To allow for this, we set the "extra info" field of a diagnostic to contain
an implementation of one of the following interfaces:

- [`tfdiags.DiagnosticExtraBecauseUnknown`](https://pkg.go.dev/github.com/opentofu/opentofu/internal/tfdiags#DiagnosticExtraBecauseUnknown)
  for a problem that's caused by an unknown value.
  
  (Remember that the _text_ of the error message should refer to this as "known
  only after apply", or similar.)
- [`tfdiags.DiagnosticExtraBecauseSensitive`](https://pkg.go.dev/github.com/opentofu/opentofu/internal/tfdiags#DiagnosticExtraBecauseSensitive)
  for situations where a sensitive value was used in a location that OpenTofu
  cannot permit it, such as in the instance key of a resource instance.

These extra markers should be used only when mentioning the unknown or sensitive
values in the diagnostic message is likely to help with debugging a problem.
If the problem is not directly caused by unknown or sensitive values then
neither of these should be used, to avoid creating a distracting
[red herring](https://en.wikipedia.org/wiki/Red_herring) for the reader.

## Consolidation of Diagnostics

The UI layer has some special rules for finding sets of similar diagnostics
and showing them as just a single diagnostic referring to the first example
of a problem, with a short extra note about how many other similar diagnostics
there are.

```
(and 2 similar warnings elsewhere)
```

The main implementation of this behavior is in
[`tfdiags.Diagnostics.Consolidate`](https://pkg.go.dev/github.com/opentofu/opentofu/internal/tfdiags#Diagnostics.Consolidate),
but we allow end-users to customize (using command line options) whether this
consolidation applies to errors or warnings separately. By default, we
consolidate only warnings.

For a severity that is subject to consolidation, the main behavior is to group
together diagnostics that have the same "summary" text, and this is part of
why we tend to use terse, fixed strings in the summary field.

There are two extra mechanisms for customizing this behavior for specific
diagnostic messages:

- If the "extra info" of a diagnostic contains an implementation of
  [`tfdiags.DiagnosticExtraDoNotConsolidate`](https://pkg.go.dev/github.com/opentofu/opentofu/internal/tfdiags#DiagnosticExtraDoNotConsolidate)
  then that diagnostic is not eligible for consolidation at all, regardless
  of how similar it might be to other diagnostics in the same set.
- If the "extra info" of a diagnostic contains an implementation of
  [`tfdiags.Keyable`](https://pkg.go.dev/github.com/opentofu/opentofu/internal/tfdiags#Keyable)
  then the string returned by its `ExtraInfoKey` method is used _in addition to_
  the summary text for deciding what to consolidate.

    For example, if there were three warnings with the same summary text but
    two of them have the same `ExtraInfoKey` and the third has a different
    one then only the first two would be able to consolidate.

    The `ExtraInfoKey` is an internal key used for comparison only and is never
    exposed in the UI, so it can be set to whatever makes sense to define
    separate consolidation groups for diagnostics with a specific summary.
