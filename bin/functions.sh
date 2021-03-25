#!/bin/bash
export ANSIBLE_HOST_KEY_CHECKING=False

create_cluster() {
        cd terraform
        terraform apply
        sleep 15 #just give the EC2 instances some time to initialize
        cd ../ansible     
        ansible-playbook site.yml -i inventory/my-cluster/hosts.ini
        sed
        cd ../terraform
        sed -i -e "s@server: https://.*:6443@server: https://$( terraform output -raw k3s_cluster_ip ):6443@" ~/.kube/config #replace private ip to public ip in local kube config file 
}

delete_cluster() {
        cd terraform
        terraform destroy
}

deploy_manifiests() {
        find ../k3s -name "*.yml" -exec kubectl apply -f {} \;
}

configure_tfvars() {
        if test -f "terraform/terraform.tfvars"; 
        then
                read -p "This action will remove the previous config, do you want to proceed? y/n:  " tempvar
                if [[ $tempvar =~ ^[Yy]$ ]]
                then
                        echo "Deleting previous configuration"
                        rm terraform/terraform.tfvars
                else
                        exit
                        echo "Abort.."
                fi
                unset tempvar
        else
                echo "Creating new configuration"
        fi

        echo "AWS Region in which to create the aws resources"
        read -p "aws_region: " tempvar
        [[ ! -z "$tempvar" ]] && echo "aws_region = \"$tempvar\"" >> terraform/terraform.tfvars
        unset tempvar
        
        echo "AWS profile to select from aws config to create resources"
        read -p "aws_profile:  " tempvar
        [[ ! -z "$tempvar" ]] && echo "aws_profile = \"$tempvar\"" >> terraform/terraform.tfvars
        unset tempvar

        echo "Name of the EC2 instances created for the K3s Cluster"
        read -p "cluster_name:  " tempvar
        [[ ! -z "$tempvar" ]] && echo "cluster_name = \"$tempvar\"" >> terraform/terraform.tfvars
        unset tempvar

        echo "Number of EC2 instances to be created as K3s Agent Nodes, this is additional to the One K3s Master node"
        read -p "number_of_nodes:  " tempvar
        [[ ! -z "$tempvar" ]] && echo "number_of_nodes = $tempvar" >> terraform/terraform.tfvars
        unset tempvar

        echo "Name of the keypair required for ansible to ssh into the EC2 instances"
        read -p "key_name:  " tempvar
        [[ ! -z "$tempvar" ]] && echo "key_name = \"$tempvar\"" >> terraform/terraform.tfvars
        unset tempvar

        echo "Instance type of the master k3s instance"
        read -p "master_instance_type:  " tempvar
        [[ ! -z "$tempvar" ]] && echo "master_instance_type = \"$tempvar\"" >> terraform/terraform.tfvars
        unset tempvar

        echo "Instance type of the node k3s instances"
        read -p "node_instance_type:  " tempvar
        [[ ! -z "$tempvar" ]] && echo "node_instance_type = \"$tempvar\"" >> terraform/terraform.tfvars
        unset tempvar

        echo "User that ansible will use to connect to the k3s instances via ssh"
        read -p "ansible_user:  " tempvar
        [[ ! -z "$tempvar" ]] && echo "ansible_user = \"$tempvar\"" >> terraform/terraform.tfvars
        unset tempvar

        echo "Ssh key for the ansible user to connect to the k3s instances"
        read -p "ansible_ssh_private_key_file:  " tempvar
        [[ ! -z "$tempvar" ]] && echo "ansible_ssh_private_key_file = \"$tempvar\"" >> terraform/terraform.tfvars
        unset tempvar

        echo "version of k3s to install in the servers"
        read -p "k3s_version:  " tempvar
        [[ ! -z "$tempvar" ]] && echo "k3s_version = \"$tempvar\"" >> terraform/terraform.tfvars
        unset tempvar

        echo "Additional arguments required to start k3s on the master server"
        read -p "extra_server_args:  " tempvar
        [[ ! -z "$tempvar" ]] && echo "extra_server_args = \"$tempvar\"" >> terraform/terraform.tfvars
        unset tempvar

        echo "Additional arguments required to start k3s on the node servers"
        read -p "extra_agent_args:  " tempvar
        [[ ! -z "$tempvar" ]] && echo "extra_agent_args = \"$tempvar\"" >> terraform/terraform.tfvars
        unset tempvar

        echo "Number of desired master nodes for a high availability k3s cluster, setting this value to a number greater than 1 will aditionally create a haproxy server and a rds mysql server"
        read -p "number_of_master_servers:  " tempvar
        [[ ! -z "$tempvar" ]] && echo "number_of_master_servers = $tempvar" >> terraform/terraform.tfvars
        unset tempvar

        echo "Path for .kube/config to be saved"
        read -p "path_to_download_kube_config:  " tempvar
        [[ ! -z "$tempvar" ]] && echo "path_to_download_kube_config = \"$tempvar\"" >> terraform/terraform.tfvars
        unset tempvar
}