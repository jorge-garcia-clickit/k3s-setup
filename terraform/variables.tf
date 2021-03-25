variable aws_region {
    type = string
    description = "AWS Region in which to create the aws resources"
    default = "us-east-1"
}

variable aws_profile {
    type = string
    description = "AWS profile to select from aws config to create resources"
    default = "default"
}

variable cluster_name {
    type = string
    description = "Name of the EC2 instances created for the K3s Cluster"
    default = "K3s"
}

variable number_of_nodes {
    type = number 
    description = "Number of EC2 instances to be created as K3s Agent Nodes, this is additional to the One K3s Master node"
    default = 2
}

variable key_name {
    type = string
    description = "Name of the keypair required for ansible to ssh into the EC2 instances"
}

variable master_instance_type {
    type = string
    description = "Instance type of the master k3s instance"
    default = "t3.micro"
}

variable node_instance_type {
    type = string
    description = "Instance type of the node k3s instances"
    default = "t3.micro"
}

variable ansible_user {
    type = string
    description = "User that ansible will use to connect to the k3s instances via ssh"
    default = "ubuntu"
}

variable ansible_ssh_private_key_file {
    type = string
    description = "Ssh key for the ansible user to connect to the k3s instances"
    default = "/home/ubuntu/.ssh/id_rsa"
}

variable k3s_version {
    type = string
    description = "version of k3s to install in the servers"
    default = "v1.17.5+k3s1"
}

variable extra_server_args {
    type = string
    description = "Additional arguments required to start k3s on the master server"
    default = "\"\""
}

variable extra_agent_args {
    type = string
    description = "Additional arguments required to start k3s on the node servers"
    default = "\"\""
}

variable number_of_master_servers {
    type = number
    description = "Number of desired master nodes for a high availability k3s cluster, setting this value to a number greater than 1 will aditionally create a haproxy server and a rds mysql server"
    default = 1
}