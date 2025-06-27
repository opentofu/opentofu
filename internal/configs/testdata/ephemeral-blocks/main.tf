provider test {
  alias = "name"
}

ephemeral "test_resource" "foo" {
  for_each = toset(["a"])
}

ephemeral "test_resource" "bar" {
  depends_on = [
    test_resource.foo["a"]
  ]
  provider = test.name
  count = 1
  attribute = "test value"
  attribute2 = "test value"

  connection {
    // connection blocks are not allowed so we are expecting an error on this
  }
  provisioner "local-exec" {
    // provisioner blocks are not allowed so we are expecting an error on this
  }
  lifecycle {
    // standard attributes in the lifecycle block are not allowed so we are expecting 4 errors on this
    create_before_destroy = true
    prevent_destroy = true
    replace_triggered_by = true
    ignore_changes = true
    // precondition and postconditions are allowed in ephemeral resources
    precondition {
      condition = ephemeral.test_resource.foo.id == ""
      error_message = "precondition error"
    }
    postcondition {
      condition = ephemeral.test_resource.foo.id == ""
      error_message = "postcondition error"
    }
  }
}