resource "aws_instance" "haproxy-server" {
	count = var.number_of_master_servers != 1 ? 1 : 0
  ami           = data.aws_ami.ubuntu.id
  instance_type = var.master_instance_type
  key_name      = var.key_name

  tags = {
    Name = "${var.cluster_name}-haproxy"
  }
}

resource "random_string" "db_user" {
	count = var.number_of_master_servers != 1 ? 1 : 0
  length           = 16
  special          = false
}

resource "random_string" "db_pass" {
	count = var.number_of_master_servers != 1 ? 1 : 0
  length           = 16
  special          = false
}

resource "random_string" "db_name" {
	count = var.number_of_master_servers != 1 ? 1 : 0
  length           = 16
  special          = false
}

resource "aws_db_instance" "k3s_datastore_endpoint" {
	count = var.number_of_master_servers != 1 ? 1 : 0
  allocated_storage    = 10
  engine               = "mysql"
  engine_version       = "5.7"
  instance_class       = "db.t3.micro"
  identifier           = "k3s-datastore"
  name                 = random_string.db_name[0].id
  username             = random_string.db_user[0].id
  password             = random_string.db_pass[0].id
  parameter_group_name = "default.mysql5.7"
  skip_final_snapshot  = true
  publicly_accessible  = true
}