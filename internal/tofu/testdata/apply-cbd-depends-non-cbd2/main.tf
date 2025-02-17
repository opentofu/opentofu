resource "terraform_data" "A" {
  triggers_replace = {
    version = 0
  }

  lifecycle {
    create_before_destroy = true
  }
}

resource "terraform_data" "B" {
  triggers_replace = {
    base    = terraform_data.A.id
    version = 1
  }
}

resource "terraform_data" "C" {
  depends_on = [terraform_data.B]

  lifecycle {
    create_before_destroy = true
  }
}
