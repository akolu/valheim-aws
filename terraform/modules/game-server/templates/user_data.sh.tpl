#!/bin/bash
# Install required packages
yum update -y
yum install -y docker awscli

# Enable and start Docker
systemctl enable docker
systemctl start docker

# Create directory structure
mkdir -p ${data_path}
mkdir -p /opt/${game_name}/scripts

# Create backup script
cat > /opt/${game_name}/scripts/backup.sh << 'EOBAK'
${backup_script_content}
EOBAK

# Create restore script
cat > /opt/${game_name}/scripts/restore.sh << 'EORES'
${restore_script_content}
EORES

# Make scripts executable
chmod +x /opt/${game_name}/scripts/backup.sh
chmod +x /opt/${game_name}/scripts/restore.sh

# Create Docker Compose file
cat > /opt/${game_name}/docker-compose.yml << 'EOT'
${docker_compose_content}
EOT

# Install Docker Compose
curl -L "https://github.com/docker/compose/releases/download/v2.20.0/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose
chmod +x /usr/local/bin/docker-compose

# Setup systemd service for auto-start
cat > /etc/systemd/system/${game_name}.service << 'EOSVC'
[Unit]
Description=${display_name}
After=docker.service
Requires=docker.service

[Service]
Type=oneshot
RemainAfterExit=yes
WorkingDirectory=/opt/${game_name}
ExecStartPre=/opt/${game_name}/scripts/restore.sh
ExecStart=/usr/local/bin/docker-compose up -d
ExecStop=/usr/local/bin/docker-compose down
ExecStop=/opt/${game_name}/scripts/backup.sh
TimeoutStartSec=0
TimeoutStopSec=300

[Install]
WantedBy=multi-user.target
EOSVC

# Enable and start game service
systemctl daemon-reload
systemctl enable ${game_name}.service
systemctl start ${game_name}.service
