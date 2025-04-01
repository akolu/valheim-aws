version: "3"
services:
  valheim:
    image: lloesche/valheim-server
    container_name: valheim-server
    restart: always
    ports:
      - "2456-2458:2456-2458/udp"
    environment:
      - SERVER_NAME=${server_name}
      - WORLD_NAME=${world_name}
      - SERVER_PASS=${server_pass}
      - SERVER_PUBLIC=false
      - TZ=Europe/Stockholm
      - AUTO_UPDATE=1
      - AUTO_BACKUP=1
    cap_add:
      - SYS_NICE
    volumes:
      - /opt/valheim/data:/config
      - /opt/valheim/data:/opt/valheim 