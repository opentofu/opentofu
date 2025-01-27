locals {
  first_year = 2024
  current_timestamp = plantimestamp()
}

output "table_years" {
  value = toset(
    [
      for year in range(local.first_year, tonumber(formatdate("YYYY", local.current_timestamp)) + 2) : tostring(year)
    ]
  )
}
