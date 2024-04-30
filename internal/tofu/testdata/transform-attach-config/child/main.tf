data "aws_instance" "child_data_instance" {
	data_config = 30
}
resource "aws_instance" "child_resource_instance" {
	data_config = 40
}
