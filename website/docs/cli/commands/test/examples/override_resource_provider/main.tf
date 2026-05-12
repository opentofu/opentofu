locals {
    my_files = toset(["file_a", "file_b", "file_c"])
}

data "local_file" "greeting" {
    for_each = local.my_files

    filename = each.value
}

output "greeting_a" {
    value = "${data.local_file.greeting["file_a"].content} World"
}
output "greeting_b" {
    value = "${data.local_file.greeting["file_b"].content} World"
}
output "greeting_c" {
    value = "${data.local_file.greeting["file_c"].content} World"
}
