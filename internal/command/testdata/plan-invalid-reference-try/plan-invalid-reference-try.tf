resource "test" "a" {
  values = {
    phase = "updated"
  }
}

resource "test" "b" {
  values = {
    a_phase = try(
      # This is intentionally an invalid reference that is nonetheless
      # visible to static reference analysis, and so can be detected
      # by the "relevant attributes" heuristic in the language runtime.
      test.a.values[0].phase,

      # This is a valid version of the above, thereby allowing this
      # overall expression to succeed evaluation.
      test.a.values.phase,
    )
  }
}
