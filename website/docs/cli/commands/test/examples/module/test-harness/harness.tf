# Load the main module:
module "main" {
  source = "../"
}

# Fetch the website so the assert can do its job:
data "http" "test" {
  url = "http://localhost:8080"

  # Important! Wait for the main module to finish:
  depends_on = [module.main]
}