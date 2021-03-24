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
  ami           = data.aws_ami.ubuntu.id
  instance_type = var.master_instance_type
  key_name      = var.key_name

  tags = {
    Name = "${var.cluster_name}-master"
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

resource "local_file" "ansible-inventory" {
  content = templatefile("ansible-inventory.tmpl",
    {
      master-dns = aws_instance.k3s-master.public_dns,
      master-ip = aws_instance.k3s-master.public_ip,
      master-id = aws_instance.k3s-master.id,
      node-dns = aws_instance.k3s-nodes.*.public_dns,
      node-ip = aws_instance.k3s-nodes.*.public_ip,
      node-id = aws_instance.k3s-nodes.*.id
    }
  )
 filename = "../ansible/inventory/my-cluster/hosts.ini"
}

resource "local_file" "ansible-vars" {
  content = templatefile("ansible-vars.tmpl",
    {
      ansible_user = var.ansible_user,
      ansible_ssh_private_key_file = var.ansible_ssh_private_key_file,
      k3s_version = var.k3s_version,
      extra_server_args = var.extra_server_args,
      extra_agent_args = var.extra_agent_args
    }
  )
 filename = "../ansible/inventory/my-cluster/group_vars/all.yml"
}

