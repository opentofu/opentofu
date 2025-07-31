run "http" {
  # Load the test helper instead of the main module:
  module {
    source = "./test-harness"
  }

  # Check if the webserver returned an HTTP 200 status code:
  assert {
    condition     = data.http.test.status_code == 200
    error_message = "Incorrect status code returned: ${data.http.test.status_code}"
  }
}