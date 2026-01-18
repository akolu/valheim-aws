version: '3'
services:
  ${game_name}-server:
    image: ${docker_image}
    container_name: ${game_name}-server
    restart: always
    ports:
%{ for port in udp_ports ~}
      - "${port}:${port}/udp"
%{ endfor ~}
%{ for port in tcp_ports ~}
      - "${port}:${port}/tcp"
%{ endfor ~}
    environment:
%{ for key, value in env_vars ~}
      - ${key}=${value}
%{ endfor ~}
    cap_add:
      - SYS_NICE
    volumes:
      - ${data_path}:${data_path}
