provider "cloudinit" {

}

data "cloudinit_config" "test" {
  part {
    content = "Hello World"
  }
}
