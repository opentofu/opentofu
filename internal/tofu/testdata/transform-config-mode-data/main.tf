data "aws_ami" "foo" {}

resource "aws_instance" "web" {}

ephemeral "aws_secret" "secret" {}