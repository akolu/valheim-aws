#!/bin/bash
# Install required packages
yum update -y
yum install -y docker awscli

# Enable and start Docker
systemctl enable docker
systemctl start docker

# Create directory structure
mkdir -p /opt/valheim
mkdir -p /opt/valheim/scripts

# Create backup script
cat > /opt/valheim/scripts/backup.sh << 'EOBAK'
${backup_script_content}
EOBAK

# Create restore script
cat > /opt/valheim/scripts/restore.sh << 'EORES'
${restore_script_content}
EORES

# Make scripts executable
chmod +x /opt/valheim/scripts/backup.sh
chmod +x /opt/valheim/scripts/restore.sh

# Create Docker Compose file
cat > /opt/valheim/docker-compose.yml << 'EOT'
${docker_compose_content}
EOT

# Install Docker Compose
curl -L "https://github.com/docker/compose/releases/download/v2.20.0/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose
chmod +x /usr/local/bin/docker-compose

# Setup systemd service for auto-start
cat > /etc/systemd/system/valheim.service << 'EOSVC'
[Unit]
Description=Valheim Server
After=docker.service
Requires=docker.service

[Service]
Type=oneshot
RemainAfterExit=yes
WorkingDirectory=/opt/valheim
ExecStartPre=/opt/valheim/scripts/restore.sh
ExecStart=/usr/local/bin/docker-compose up -d
ExecStop=/usr/local/bin/docker-compose down
ExecStop=/opt/valheim/scripts/backup.sh
TimeoutStartSec=0
TimeoutStopSec=300

[Install]
WantedBy=multi-user.target
EOSVC

# Enable and start valheim service
systemctl daemon-reload
systemctl enable valheim.service
systemctl start valheim.service 