moved {
    from = test_object.a
    to = test_object.b
}

resource "test_object" "a" {}
resource "test_object" "b" {}
