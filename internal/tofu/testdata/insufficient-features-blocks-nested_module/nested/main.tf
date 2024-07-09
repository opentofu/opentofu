provider "test" {
    alias = "test2" 
    test_string = "config"
    features {}
}

resource "test_object" "a" {
}