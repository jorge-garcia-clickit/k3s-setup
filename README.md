## To run properly this tool requires the following:
* Terraform v0.14.6 or greater
* ansible 2.10.7
* An AWS cli profile already configured



## Usage Page 

```sh
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
```

## Usage Examples

```
./main.sh create
```

```
./main.sh delete
```

```
./main.sh deploy
```
```
./main.sh usage
```

