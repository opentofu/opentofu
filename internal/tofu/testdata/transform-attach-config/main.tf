data "aws_instance" "data_instance" {
	data_config = 10
}
resource "aws_instance" "resource_instance" {
	resource_config = 20
}
module "child" {
	source = "./child"	
}
