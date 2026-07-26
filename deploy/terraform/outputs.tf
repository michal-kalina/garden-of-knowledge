output "instance_id" {
  description = "EC2 instance ID — used for stop/start commands (see README.md 'Saving money between sessions')."
  value       = aws_instance.k3s_host.id
}

output "public_ip" {
  description = "Stable public IP of the k3s host (Elastic IP). Point your domain's A record here."
  value       = aws_eip.k3s_host.public_ip
}

output "ssh_command" {
  description = "SSH in to check on the instance or re-run the kubeconfig fetch below."
  value       = "ssh ec2-user@${aws_eip.k3s_host.public_ip}"
}

output "wait_for_k3s_command" {
  description = "Poll until k3s has actually finished installing (takes 1-2 minutes after the instance is SSH-reachable) before fetching the kubeconfig."
  value       = "until ssh -o ConnectTimeout=5 ec2-user@${aws_eip.k3s_host.public_ip} 'test -f /var/log/k3s-install-done' 2>/dev/null; do echo waiting...; sleep 10; done"
}

output "fetch_kubeconfig_command" {
  description = "Copies the k3s kubeconfig locally and rewrites its embedded 127.0.0.1 to the real public IP, so kubectl on your laptop can reach it directly."
  value       = "ssh ec2-user@${aws_eip.k3s_host.public_ip} 'cat ~/.kube/config' | sed 's/127.0.0.1/${aws_eip.k3s_host.public_ip}/' > kubeconfig.yaml && export KUBECONFIG=$(pwd)/kubeconfig.yaml"
}
