resource "aws_cloudwatch_dashboard" "game_server_dashboard" {
  dashboard_name = "${local.instance_name}-dashboard"

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
            ["AWS/EC2", "CPUUtilization", "InstanceId", aws_spot_instance_request.game_server.spot_instance_id]
          ]
          view    = "timeSeries"
          stacked = false
          region  = var.aws_region
          title   = "${local.display_name} CPU Utilization"
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
            ["AWS/EC2", "NetworkIn", "InstanceId", aws_spot_instance_request.game_server.spot_instance_id],
            ["AWS/EC2", "NetworkOut", "InstanceId", aws_spot_instance_request.game_server.spot_instance_id]
          ]
          view    = "timeSeries"
          stacked = false
          region  = var.aws_region
          title   = "${local.display_name} Network Traffic"
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
            ["AWS/EC2", "StatusCheckFailed_Instance", "InstanceId", aws_spot_instance_request.game_server.spot_instance_id],
            ["AWS/EC2", "StatusCheckFailed_System", "InstanceId", aws_spot_instance_request.game_server.spot_instance_id]
          ]
          view    = "timeSeries"
          stacked = false
          region  = var.aws_region
          title   = "${local.display_name} Status Checks"
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
            ["AWS/EC2", "EBSReadOps", "InstanceId", aws_spot_instance_request.game_server.spot_instance_id],
            ["AWS/EC2", "EBSWriteOps", "InstanceId", aws_spot_instance_request.game_server.spot_instance_id]
          ]
          view    = "timeSeries"
          stacked = false
          region  = var.aws_region
          title   = "${local.display_name} Disk Operations"
          period  = 300
        }
      }
    ]
  })
}
