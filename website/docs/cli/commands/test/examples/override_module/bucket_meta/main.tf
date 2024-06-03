data "local_file" "bucket_name" {
  filename = "bucket_name.txt"
}

output "name" {
  value = data.local_file.bucket_name.content
}

output "tags" {
  value = {
    Environment = "Dev"
  }
}
