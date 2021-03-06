#!/bin/bash
source bin/functions.sh
##Global Variables 
usage() {
        echo "
NAME
       k3s-setup -

DESCRIPTION
       This  section  explains  default behavior and notations in the commands provided.
COMMANDS AVAILABLE
        create
                Create terraform resources and provision k3s cluster
                If the configure is not executed before a create command 
                execution then default values in terraform/variables.tf 
                will be used, depending on your configuration you can add
                additional master nodes, a RDS database and a haproxy server
                to enable a highly available k3s cluster.
                After the creation of the terraform resources ansible will provision
                the k3s cluster and it will copy the .kube/config file from the 
                remote master server to your local environment
        delete
                Basically just a terraform destroy
        deploy
                Automatically looks for k8s manifests located in the k3s directory
                or in any given directory during configure, the cluster needs to be
                created first before running this command, files shall be named with
                a XX- number prefix, example given:
                00-Deployment.yml
                01-Service.yml
                02-ingress.yml
        usage -h --help 
                Prints this message.
"
}



##########################################################Main script###################################################################
#check if subcommand exists, then go to subcommand function, else print usage information and exit with error code 1

case $1 in

create )
                        create_cluster
                        ;;
delete )
                        delete_cluster
                        ;;
deploy )
                        deploy_manifiests
                        ;;
configure )
                        configure_tfvars
                        ;;
-h | --help | usage )
                        usage
                        ;;
* )
                        echo "Error: Invalid option"
                        usage
                        exit 1
        esac
        shift

