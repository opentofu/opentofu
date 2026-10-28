data "test_data_source" "conf" {
  id = "id"
}

output "out_data" {
  value = "data ${data.test_data_source.conf.id} ${data.test_data_source.conf.value}"
}
