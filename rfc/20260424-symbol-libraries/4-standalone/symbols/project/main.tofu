symbols "lib" {
  source = "../library"
}

output "lp_basic" {
  value = symbols::lib::leftpad("foobar", 10)
}

output "lp_char" {
  value = symbols::lib::leftpad("foobar", 10, "_")
}
