resource "test_object" "A" {
  test_string = "A"

  lifecycle {
    create_before_destroy = true
  }
}

resource "test_object" "B" {
  test_string = test_object.A.test_string
  test_number  = 1
}

resource "test_object" "C" {
  test_string = "C"
  depends_on = [test_object.B]

  lifecycle {
    create_before_destroy = true
  }
}
