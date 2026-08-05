moved {
    from = test_object.a
    to = test_object.b
}

moved {
    from = test_object.a
    to = test_object.c
}

resource "test_object" "b" {}

resource "test_object" "c" {}
