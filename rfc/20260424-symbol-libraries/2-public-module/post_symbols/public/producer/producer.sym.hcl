typedef "producer" {
  type = object({
    id = string
    data = object({
      text = string
      other = number
    })
  })
}
