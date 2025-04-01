locals {
  docker_compose = templatefile("${path.module}/templates/docker-compose.yml.tpl", {
    server_name = var.valheim_server_name
    world_name  = var.valheim_world_name
    server_pass = var.valheim_server_pass
  })
} 
