package cmd

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

func TestGameInstanceInfo_NotProvisioned(t *testing.T) {
	client := &mockEC2{} // empty response
	state, ip := gameInstanceInfo(context.Background(), client, "valheim")
	if state != "not-provisioned" {
		t.Errorf("state = %q, want %q", state, "not-provisioned")
	}
	if ip != "-" {
		t.Errorf("ip = %q, want %q", ip, "-")
	}
}

func TestGameInstanceInfo_Running(t *testing.T) {
	client := &mockEC2{
		describeFunc: func(_ context.Context, _ *ec2.DescribeInstancesInput, _ ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
			return &ec2.DescribeInstancesOutput{
				Reservations: []ec2types.Reservation{
					{
						Instances: []ec2types.Instance{
							{
								State:           &ec2types.InstanceState{Name: ec2types.InstanceStateNameRunning},
								PublicIpAddress: aws.String("10.0.0.1"),
							},
						},
					},
				},
			}, nil
		},
	}
	state, ip := gameInstanceInfo(context.Background(), client, "valheim")
	if state != "running" {
		t.Errorf("state = %q, want %q", state, "running")
	}
	if ip != "10.0.0.1" {
		t.Errorf("ip = %q, want %q", ip, "10.0.0.1")
	}
}

func TestGameInstanceInfo_SkipsTerminated(t *testing.T) {
	client := &mockEC2{
		describeFunc: func(_ context.Context, _ *ec2.DescribeInstancesInput, _ ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
			return &ec2.DescribeInstancesOutput{
				Reservations: []ec2types.Reservation{
					{
						Instances: []ec2types.Instance{
							{
								State: &ec2types.InstanceState{Name: ec2types.InstanceStateNameTerminated},
							},
						},
					},
				},
			}, nil
		},
	}
	state, ip := gameInstanceInfo(context.Background(), client, "valheim")
	if state != "not-provisioned" {
		t.Errorf("terminated instance should yield not-provisioned, got %q", state)
	}
	if ip != "-" {
		t.Errorf("ip = %q, want %q", ip, "-")
	}
}

func TestGameInstanceInfo_NoPublicIP(t *testing.T) {
	client := &mockEC2{
		describeFunc: func(_ context.Context, _ *ec2.DescribeInstancesInput, _ ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
			return &ec2.DescribeInstancesOutput{
				Reservations: []ec2types.Reservation{
					{
						Instances: []ec2types.Instance{
							{
								State:           &ec2types.InstanceState{Name: ec2types.InstanceStateNameStopped},
								PublicIpAddress: nil,
							},
						},
					},
				},
			}, nil
		},
	}
	state, ip := gameInstanceInfo(context.Background(), client, "valheim")
	if state != "stopped" {
		t.Errorf("state = %q, want %q", state, "stopped")
	}
	if ip != "-" {
		t.Errorf("ip = %q, want %q", ip, "-")
	}
}

func TestGameInstanceInfo_Error(t *testing.T) {
	client := &mockEC2{
		describeFunc: func(_ context.Context, _ *ec2.DescribeInstancesInput, _ ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
			return nil, errors.New("RequestExpired")
		},
	}
	state, ip := gameInstanceInfo(context.Background(), client, "valheim")
	if state != "error" {
		t.Errorf("state = %q, want %q", state, "error")
	}
	if ip != "-" {
		t.Errorf("ip = %q, want %q", ip, "-")
	}
}

func TestGameInstanceInfo_FiltersOnGameTag(t *testing.T) {
	var gotFilters []string
	client := &mockEC2{
		describeFunc: func(_ context.Context, params *ec2.DescribeInstancesInput, _ ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
			for _, f := range params.Filters {
				if aws.ToString(f.Name) == "tag:Game" {
					gotFilters = append(gotFilters, f.Values...)
				}
			}
			return &ec2.DescribeInstancesOutput{}, nil
		},
	}
	gameInstanceInfo(context.Background(), client, "satisfactory")
	if len(gotFilters) != 1 || gotFilters[0] != "satisfactory" {
		t.Errorf("tag:Game filter values = %v, want [satisfactory]", gotFilters)
	}
}
