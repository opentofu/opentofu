run "b" {
    assert {
        condition = test_resource.resource.value == null
        error_message = "should pass"
    }
}
