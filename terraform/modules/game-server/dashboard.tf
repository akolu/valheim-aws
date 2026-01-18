resource "aws_cloudwatch_dashboard" "valheim_dashboard" {
  dashboard_name = "${var.instance_name}-dashboard"

  dashboard_body = jsonencode({
    widgets = [
      {
        type   = "metric"
        x      = 0
        y      = 0
        width  = 12
        height = 6
        properties = {
          metrics = [
            ["AWS/EC2", "CPUUtilization", "InstanceId", aws_spot_instance_request.valheim_server.spot_instance_id]
          ]
          view    = "timeSeries"
          stacked = false
          region  = var.aws_region
          title   = "CPU Utilization"
          period  = 300
        }
      },
      {
        type   = "metric"
        x      = 12
        y      = 0
        width  = 12
        height = 6
        properties = {
          metrics = [
            ["AWS/EC2", "NetworkIn", "InstanceId", aws_spot_instance_request.valheim_server.spot_instance_id],
            ["AWS/EC2", "NetworkOut", "InstanceId", aws_spot_instance_request.valheim_server.spot_instance_id]
          ]
          view    = "timeSeries"
          stacked = false
          region  = var.aws_region
          title   = "Network Traffic"
          period  = 300
        }
      },
      {
        type   = "metric"
        x      = 0
        y      = 6
        width  = 12
        height = 6
        properties = {
          metrics = [
            ["AWS/EC2", "StatusCheckFailed_Instance", "InstanceId", aws_spot_instance_request.valheim_server.spot_instance_id],
            ["AWS/EC2", "StatusCheckFailed_System", "InstanceId", aws_spot_instance_request.valheim_server.spot_instance_id]
          ]
          view    = "timeSeries"
          stacked = false
          region  = var.aws_region
          title   = "Status Checks"
          period  = 60
        }
      },
      {
        type   = "metric"
        x      = 12
        y      = 6
        width  = 12
        height = 6
        properties = {
          metrics = [
            ["AWS/EC2", "EBSReadOps", "InstanceId", aws_spot_instance_request.valheim_server.spot_instance_id],
            ["AWS/EC2", "EBSWriteOps", "InstanceId", aws_spot_instance_request.valheim_server.spot_instance_id]
          ]
          view    = "timeSeries"
          stacked = false
          region  = var.aws_region
          title   = "Disk Operations"
          period  = 300
        }
      }
    ]
  })
}
