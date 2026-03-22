package cmd

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// --- formatLongtermArchives tests ---

func TestFormatLongtermArchives_None(t *testing.T) {
	got := formatLongtermArchives(nil)
	if got != "none" {
		t.Errorf("formatLongtermArchives(nil) = %q, want %q", got, "none")
	}
	got = formatLongtermArchives([]string{})
	if got != "none" {
		t.Errorf("formatLongtermArchives([]) = %q, want %q", got, "none")
	}
}

func TestFormatLongtermArchives_Single(t *testing.T) {
	got := formatLongtermArchives([]string{"2026-03-21T150405Z/"})
	want := "1 snapshot, latest 2026-03-21T150405Z"
	if got != want {
		t.Errorf("formatLongtermArchives() = %q, want %q", got, want)
	}
}

func TestFormatLongtermArchives_MultiplePicksLatest(t *testing.T) {
	prefixes := []string{
		"2026-01-10T120000Z/",
		"2026-03-21T150405Z/",
		"2026-02-05T080000Z/",
	}
	got := formatLongtermArchives(prefixes)
	want := "3 snapshots, latest 2026-03-21T150405Z"
	if got != want {
		t.Errorf("formatLongtermArchives() = %q, want %q", got, want)
	}
}

func TestDescribeGameInstance_NotFound(t *testing.T) {
	client := &mockEC2{} // empty response
	id, state, ip, err := describeGameInstance(context.Background(), client, "valheim")
	if err != nil {
		t.Fatalf("describeGameInstance() error: %v", err)
	}
	if id != "" || state != "" || ip != "" {
		t.Errorf("expected empty results, got id=%q state=%q ip=%q", id, state, ip)
	}
}

func TestDescribeGameInstance_Running(t *testing.T) {
	client := &mockEC2{
		describeFunc: func(_ context.Context, _ *ec2.DescribeInstancesInput, _ ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
			return &ec2.DescribeInstancesOutput{
				Reservations: []ec2types.Reservation{
					{
						Instances: []ec2types.Instance{
							{
								InstanceId:      aws.String("i-0abc123"),
								State:           &ec2types.InstanceState{Name: ec2types.InstanceStateNameRunning},
								PublicIpAddress: aws.String("203.0.113.5"),
							},
						},
					},
				},
			}, nil
		},
	}
	id, state, ip, err := describeGameInstance(context.Background(), client, "valheim")
	if err != nil {
		t.Fatalf("describeGameInstance() error: %v", err)
	}
	if id != "i-0abc123" {
		t.Errorf("id = %q, want %q", id, "i-0abc123")
	}
	if state != "running" {
		t.Errorf("state = %q, want %q", state, "running")
	}
	if ip != "203.0.113.5" {
		t.Errorf("ip = %q, want %q", ip, "203.0.113.5")
	}
}

func TestDescribeGameInstance_SkipsTerminated(t *testing.T) {
	client := &mockEC2{
		describeFunc: func(_ context.Context, _ *ec2.DescribeInstancesInput, _ ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
			return &ec2.DescribeInstancesOutput{
				Reservations: []ec2types.Reservation{
					{
						Instances: []ec2types.Instance{
							{
								InstanceId: aws.String("i-old"),
								State:      &ec2types.InstanceState{Name: ec2types.InstanceStateNameTerminated},
							},
						},
					},
				},
			}, nil
		},
	}
	id, state, ip, err := describeGameInstance(context.Background(), client, "valheim")
	if err != nil {
		t.Fatalf("describeGameInstance() error: %v", err)
	}
	if id != "" || state != "" || ip != "" {
		t.Errorf("terminated instance should return empty, got id=%q state=%q ip=%q", id, state, ip)
	}
}

func TestDescribeGameInstance_NoPublicIP(t *testing.T) {
	client := &mockEC2{
		describeFunc: func(_ context.Context, _ *ec2.DescribeInstancesInput, _ ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
			return &ec2.DescribeInstancesOutput{
				Reservations: []ec2types.Reservation{
					{
						Instances: []ec2types.Instance{
							{
								InstanceId:      aws.String("i-0def456"),
								State:           &ec2types.InstanceState{Name: ec2types.InstanceStateNameStopped},
								PublicIpAddress: nil,
							},
						},
					},
				},
			}, nil
		},
	}
	id, state, ip, err := describeGameInstance(context.Background(), client, "valheim")
	if err != nil {
		t.Fatalf("describeGameInstance() error: %v", err)
	}
	if id != "i-0def456" {
		t.Errorf("id = %q, want %q", id, "i-0def456")
	}
	if state != "stopped" {
		t.Errorf("state = %q, want %q", state, "stopped")
	}
	if ip != "-" {
		t.Errorf("ip = %q, want %q", ip, "-")
	}
}

func TestDescribeGameInstance_Error(t *testing.T) {
	client := &mockEC2{
		describeFunc: func(_ context.Context, _ *ec2.DescribeInstancesInput, _ ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
			return nil, errors.New("Throttling")
		},
	}
	_, _, _, err := describeGameInstance(context.Background(), client, "valheim")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestDescribeGameInstance_FiltersOnGameAndProject(t *testing.T) {
	var filterNames []string
	client := &mockEC2{
		describeFunc: func(_ context.Context, params *ec2.DescribeInstancesInput, _ ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
			for _, f := range params.Filters {
				filterNames = append(filterNames, aws.ToString(f.Name))
			}
			return &ec2.DescribeInstancesOutput{}, nil
		},
	}
	describeGameInstance(context.Background(), client, "valheim")
	wantFilters := map[string]bool{"tag:Game": false, "tag:Project": false}
	for _, name := range filterNames {
		wantFilters[name] = true
	}
	for name, found := range wantFilters {
		if !found {
			t.Errorf("filter %q not sent to DescribeInstances", name)
		}
	}
}
