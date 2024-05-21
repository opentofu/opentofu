variables {
  var2 = var.var1 ? "true" : "false"
}

run "first" {
  assert {
    condition = output.sss == "true"
    error_message = "Should work"
  }
}