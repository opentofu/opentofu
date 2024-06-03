variables {
  var2 = var.var1 ? "true" : "false"
}

run "first" {
  assert {
    condition = output.sss == "false"
    error_message = "Should work"
  }
}