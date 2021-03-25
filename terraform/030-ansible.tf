resource "local_file" "ansible-inventory" {
  content = templatefile("ansible-inventory.tmpl",
    {
      master-dns = aws_instance.k3s-master.*.public_dns,
      master-ip = aws_instance.k3s-master.*.public_ip,
      master-id = aws_instance.k3s-master.*.id,
      node-dns = aws_instance.k3s-nodes.*.public_dns,
      node-ip = aws_instance.k3s-nodes.*.public_ip,
      node-id = aws_instance.k3s-nodes.*.id
      haproxy-dns = aws_instance.haproxy-server.*.public_dns,
      haproxy-ip = aws_instance.haproxy-server.*.public_ip,
      haproxy-id = aws_instance.haproxy-server.*.id
    }
  )
 filename = "../ansible/inventory/my-cluster/hosts.ini"
}

resource "local_file" "ansible-vars" {
  content = templatefile("ansible-vars.tmpl",
    {
      ansible_user = var.ansible_user,
      ansible_ssh_private_key_file = var.ansible_ssh_private_key_file
      k3s_version = var.k3s_version
      master_ip= var.number_of_master_servers != 1 ? aws_instance.haproxy-server[0].private_ip : "\"{{ hostvars[groups['master'][0]]['ansible_host'] | default(groups['master'][0]) }}\"" #Master IP is the IP of the master node unless there is more than one master node, in such case master ip is the haproxy server ip
      extra_server_args = var.number_of_master_servers != 1 ? "--datastore-endpoint \"mysql://${random_string.db_user[0].id}:${random_string.db_pass[0].id}@tcp(${aws_db_instance.k3s_datastore_endpoint[0].endpoint})/${random_string.db_name[0].id}\" --tls-san ${aws_instance.haproxy-server[0].private_ip}  --tls-san ${aws_instance.haproxy-server[0].public_ip}" : " --tls-san ${aws_instance.k3s-master[0].public_ip}"
      extra_agent_args = var.extra_agent_args
      server_names = var.number_of_master_servers != 1 ? aws_instance.k3s-master.*.id : []
      server_ips = var.number_of_master_servers != 1 ? aws_instance.k3s-master.*.private_ip : []
      path_to_download_kube_config = var.path_to_download_kube_config
    }
  )
 filename = "../ansible/inventory/my-cluster/group_vars/all.yml"
}

output "k3s_cluster_ip" {
  value = var.number_of_master_servers != 1 ? aws_instance.haproxy-server[0].public_ip : aws_instance.k3s-master[0].public_ip #Master IP is the IP of the master node unless there is more than one master node, in such case master ip is the haproxy server ip
}