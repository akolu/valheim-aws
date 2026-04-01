version: '3'
services:
%{ if init_service != null ~}
  ${game_name}-init:
    image: ${init_service.image}
    entrypoint: ["/bin/sh", "-c"]
    command:
      - |
        ${indent(8, init_service.command)}
    environment:
%{ for key, value in init_service.env_vars ~}
      - "${key}=${value}"
%{ endfor ~}
    volumes:
      - ${data_path}:${data_path}
    restart: "no"

%{ endif ~}
  ${game_name}-server:
    image: ${docker_image}
    container_name: ${game_name}-server
    restart: always
%{ if init_service != null ~}
    depends_on:
      ${game_name}-init:
        condition: service_completed_successfully
%{ endif ~}
    ports:
%{ for port in udp_ports ~}
      - "${port}:${port}/udp"
%{ endfor ~}
%{ for port in tcp_ports ~}
      - "${port}:${port}/tcp"
%{ endfor ~}
    environment:
%{ for key, value in env_vars ~}
      - "${key}=${value}"
%{ endfor ~}
    cap_add:
      - SYS_NICE
    volumes:
      - ${data_path}:${data_path}
