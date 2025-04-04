#!/bin/bash
# Script to install CloudWatch agent on a running Lightsail instance
# Usage: ./install_monitoring.sh ec2-user@your-server-ip lightsail-key.pem access-key secret-key
# Documentation: https://aws.amazon.com/blogs/compute/monitoring-memory-usage-lightsail-instance/

if [ "$#" -ne 4 ]; then
    echo "Usage: $0 <user@hostname> <key_file> <access_key> <secret_key>"
    echo "Example: $0 ec2-user@1.2.3.4 valheim-key.pem AKIAXXXXXXXXXXXXXXXX wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
    exit 1
fi

SERVER="$1"
KEY_FILE="$2"
ACCESS_KEY="$3"
SECRET_KEY="$4"
INSTANCE_NAME="valheim-server"  # Hardcoded for simplicity

echo "Creating CloudWatch agent config file..."
cat > /tmp/cloudwatch-agent-config.json << EOF
{
    "agent": {
        "metrics_collection_interval": 60,
        "run_as_user": "root"
    },
    "metrics": {
        "append_dimensions": {
            "ImageID": "\${aws:ImageId}",
            "InstanceId": "\${aws:InstanceId}",
            "InstanceType": "\${aws:InstanceType}"
        },
        "metrics_collected": {
            "mem": {
                "measurement": [
                    "mem_used_percent"
                ],
                "metrics_collection_interval": 60
            }
        }
    }
}
EOF

echo "Installing CloudWatch agent on ${SERVER}..."
ssh -i "$KEY_FILE" "$SERVER" "sudo dnf install -y amazon-cloudwatch-agent"

echo "Copying config file to server..."
scp -i "$KEY_FILE" /tmp/cloudwatch-agent-config.json "$SERVER":/tmp/

echo "Configuring CloudWatch agent on the server..."
ssh -i "$KEY_FILE" "$SERVER" "sudo mkdir -p /opt/aws/amazon-cloudwatch-agent/etc"
ssh -i "$KEY_FILE" "$SERVER" "sudo mv /tmp/cloudwatch-agent-config.json /opt/aws/amazon-cloudwatch-agent/etc/amazon-cloudwatch-agent.json"

echo "Adding shared credentials profile to common-config.toml..."
ssh -i "$KEY_FILE" "$SERVER" "sudo bash -c 'cat >> /opt/aws/amazon-cloudwatch-agent/etc/common-config.toml' << EOF

[credentials]
shared_credential_profile = "AmazonCloudWatchAgent"
EOF"

# Create credentials file
ssh -i "$KEY_FILE" "$SERVER" "sudo mkdir -p /root/.aws"
ssh -i "$KEY_FILE" "$SERVER" "sudo bash -c 'cat > /root/.aws/credentials' << EOF
[AmazonCloudWatchAgent]
aws_access_key_id = $ACCESS_KEY
aws_secret_access_key = $SECRET_KEY
region = $(aws configure get region || echo 'eu-north-1')
EOF"

echo "Starting CloudWatch agent..."
ssh -i "$KEY_FILE" "$SERVER" "sudo amazon-cloudwatch-agent-ctl -c file:/opt/aws/amazon-cloudwatch-agent/bin/config.json -a fetch-config -s"

echo "✅ CloudWatch agent has been installed and configured with IAM credentials."
echo "Memory metrics should appear in CloudWatch console within a few minutes." 