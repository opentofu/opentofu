moved {
    from = test_object.a
    to = test_object.c
}

moved {
    from = test_object.b
    to = test_object.c
}

resource "test_object" "c" {}
