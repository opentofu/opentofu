output "untyped" {
  value = "hello"
}

output "primitive" {
  value = 5
  type  = string
}

output "collection" {
  value = ["a", "b"]
  type  = list(string)
}

output "object" {
  value = {
	name = "thing"
	count = 123
  }
  type = object({
    name  = string
    count = number
  })
}

variable "nums" {
  default = [1, 2, 3]
  type = list(number)
}

output "external-value" {
  value = var.nums
  type  = list(string)
}
