data "aws_ami" "ubuntu" {
  most_recent = true

  filter {
    name   = "name"
    values = ["ubuntu/images/hvm-ssd/ubuntu-focal-20.04-amd64-server-*"]
  }

  filter {
    name   = "virtualization-type"
    values = ["hvm"]
  }

  owners = ["099720109477"] # Canonical
}

resource "aws_instance" "k3s-master" {
	count = var.number_of_master_servers
  ami           = data.aws_ami.ubuntu.id
  instance_type = var.master_instance_type
  key_name      = var.key_name

  tags = {
    Name = "${var.cluster_name}-master-${count.index}"
  }
}

resource "aws_instance" "k3s-nodes" {
	count = var.number_of_nodes
  ami           = data.aws_ami.ubuntu.id
  instance_type = var.node_instance_type
  key_name      = var.key_name

  tags = {
    Name = "${var.cluster_name}-node-${count.index}"
  }
}

