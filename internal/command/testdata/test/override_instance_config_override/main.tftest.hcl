override_data {
  target = data.test_data_source.conf
  values = {
    id    = "overridden_id"
	value = "overridden_content"
  }
}

run "argument_in_values_is_accepted" {
  command = plan

  assert {
    condition     = output.out_data == "data id overridden_content"
    error_message = "expected data id overridden_content"
  }
}
