package cmd

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// mockS3 implements s3API for testing.
type mockS3 struct {
	listFunc func(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
	copyFunc func(ctx context.Context, params *s3.CopyObjectInput, optFns ...func(*s3.Options)) (*s3.CopyObjectOutput, error)
	getFunc  func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
}

func (m *mockS3) ListObjectsV2(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx, params, optFns...)
	}
	return &s3.ListObjectsV2Output{}, nil
}

func (m *mockS3) CopyObject(ctx context.Context, params *s3.CopyObjectInput, optFns ...func(*s3.Options)) (*s3.CopyObjectOutput, error) {
	if m.copyFunc != nil {
		return m.copyFunc(ctx, params, optFns...)
	}
	return &s3.CopyObjectOutput{}, nil
}

func (m *mockS3) GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	if m.getFunc != nil {
		return m.getFunc(ctx, params, optFns...)
	}
	return &s3.GetObjectOutput{Body: io.NopCloser(strings.NewReader(""))}, nil
}

// mockEC2 implements ec2API for testing.
type mockEC2 struct {
	describeFunc func(ctx context.Context, params *ec2.DescribeInstancesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error)
}

func (m *mockEC2) DescribeInstances(ctx context.Context, params *ec2.DescribeInstancesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
	if m.describeFunc != nil {
		return m.describeFunc(ctx, params, optFns...)
	}
	return &ec2.DescribeInstancesOutput{}, nil
}

// --- listObjects tests ---

func TestListObjects_Empty(t *testing.T) {
	client := &mockS3{}
	keys, err := listObjects(context.Background(), client, "my-bucket", "")
	if err != nil {
		t.Fatalf("listObjects() error: %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("expected 0 keys, got %d", len(keys))
	}
}

func TestListObjects_WithKeys(t *testing.T) {
	client := &mockS3{
		listFunc: func(_ context.Context, _ *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			return &s3.ListObjectsV2Output{
				Contents: []s3types.Object{
					{Key: aws.String("saves/world.zip")},
					{Key: aws.String("saves/backup.zip")},
				},
			}, nil
		},
	}
	keys, err := listObjects(context.Background(), client, "bucket", "")
	if err != nil {
		t.Fatalf("listObjects() error: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d: %v", len(keys), keys)
	}
	if keys[0] != "saves/world.zip" || keys[1] != "saves/backup.zip" {
		t.Errorf("unexpected keys: %v", keys)
	}
}

func TestListObjects_Error(t *testing.T) {
	client := &mockS3{
		listFunc: func(_ context.Context, _ *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			return nil, errors.New("NoSuchBucket")
		},
	}
	_, err := listObjects(context.Background(), client, "bucket", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- copyObject tests ---

func TestCopyObject_Success(t *testing.T) {
	var gotCopySource string
	client := &mockS3{
		copyFunc: func(_ context.Context, params *s3.CopyObjectInput, _ ...func(*s3.Options)) (*s3.CopyObjectOutput, error) {
			gotCopySource = aws.ToString(params.CopySource)
			return &s3.CopyObjectOutput{}, nil
		},
	}
	err := copyObject(context.Background(), client, "src-bucket", "key.zip", "dst-bucket", "archive/key.zip")
	if err != nil {
		t.Fatalf("copyObject() error: %v", err)
	}
	if gotCopySource != "src-bucket/key.zip" {
		t.Errorf("CopySource = %q, want %q", gotCopySource, "src-bucket/key.zip")
	}
}

func TestCopyObject_Error(t *testing.T) {
	client := &mockS3{
		copyFunc: func(_ context.Context, _ *s3.CopyObjectInput, _ ...func(*s3.Options)) (*s3.CopyObjectOutput, error) {
			return nil, errors.New("AccessDenied")
		},
	}
	err := copyObject(context.Background(), client, "src", "key", "dst", "dstkey")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- getObject tests ---

func TestGetObject_Success(t *testing.T) {
	content := "save file contents"
	client := &mockS3{
		getFunc: func(_ context.Context, _ *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			return &s3.GetObjectOutput{Body: io.NopCloser(strings.NewReader(content))}, nil
		},
	}
	var buf strings.Builder
	err := getObject(context.Background(), client, "bucket", "save.zip", &buf)
	if err != nil {
		t.Fatalf("getObject() error: %v", err)
	}
	if buf.String() != content {
		t.Errorf("getObject() wrote %q, want %q", buf.String(), content)
	}
}

func TestGetObject_Error(t *testing.T) {
	client := &mockS3{
		getFunc: func(_ context.Context, _ *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			return nil, errors.New("NoSuchKey")
		},
	}
	err := getObject(context.Background(), client, "bucket", "missing.zip", io.Discard)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- instanceState tests ---

func TestInstanceState_EmptyID(t *testing.T) {
	client := &mockEC2{}
	state, ip, err := instanceState(context.Background(), client, "")
	if err != nil {
		t.Fatalf("instanceState() error: %v", err)
	}
	if state != "" || ip != "" {
		t.Errorf("expected empty state/ip for empty ID, got state=%q ip=%q", state, ip)
	}
}

func TestInstanceState_Running(t *testing.T) {
	client := &mockEC2{
		describeFunc: func(_ context.Context, _ *ec2.DescribeInstancesInput, _ ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
			return &ec2.DescribeInstancesOutput{
				Reservations: []ec2types.Reservation{
					{
						Instances: []ec2types.Instance{
							{
								State:           &ec2types.InstanceState{Name: ec2types.InstanceStateNameRunning},
								PublicIpAddress: aws.String("1.2.3.4"),
							},
						},
					},
				},
			}, nil
		},
	}
	state, ip, err := instanceState(context.Background(), client, "i-12345")
	if err != nil {
		t.Fatalf("instanceState() error: %v", err)
	}
	if state != "running" {
		t.Errorf("state = %q, want %q", state, "running")
	}
	if ip != "1.2.3.4" {
		t.Errorf("ip = %q, want %q", ip, "1.2.3.4")
	}
}

func TestInstanceState_NotFoundError(t *testing.T) {
	client := &mockEC2{
		describeFunc: func(_ context.Context, _ *ec2.DescribeInstancesInput, _ ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
			return nil, errors.New("InvalidInstanceID.NotFound")
		},
	}
	state, ip, err := instanceState(context.Background(), client, "i-bad")
	if err != nil {
		t.Fatalf("instanceState() expected graceful handling, got error: %v", err)
	}
	if state != "not-found" {
		t.Errorf("state = %q, want %q", state, "not-found")
	}
	if ip != "" {
		t.Errorf("ip = %q, want empty", ip)
	}
}

func TestInstanceState_NoInstances(t *testing.T) {
	client := &mockEC2{} // returns empty output
	state, _, err := instanceState(context.Background(), client, "i-ghost")
	if err != nil {
		t.Fatalf("instanceState() error: %v", err)
	}
	if state != "not-found" {
		t.Errorf("state = %q, want %q", state, "not-found")
	}
}

// --- spotInstanceState tests ---

func TestSpotInstanceState_Running(t *testing.T) {
	client := &mockEC2{
		describeFunc: func(_ context.Context, _ *ec2.DescribeInstancesInput, _ ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
			return &ec2.DescribeInstancesOutput{
				Reservations: []ec2types.Reservation{
					{
						Instances: []ec2types.Instance{
							{
								State:           &ec2types.InstanceState{Name: ec2types.InstanceStateNameRunning},
								PublicIpAddress: aws.String("5.6.7.8"),
							},
						},
					},
				},
			}, nil
		},
	}
	stateName, ip, err := spotInstanceState(context.Background(), client, "i-spot")
	if err != nil {
		t.Fatalf("spotInstanceState() error: %v", err)
	}
	if stateName != ec2types.InstanceStateNameRunning {
		t.Errorf("state = %q, want %q", stateName, ec2types.InstanceStateNameRunning)
	}
	if ip != "5.6.7.8" {
		t.Errorf("ip = %q, want %q", ip, "5.6.7.8")
	}
}

func TestSpotInstanceState_NoInstances(t *testing.T) {
	client := &mockEC2{} // empty response
	stateName, _, err := spotInstanceState(context.Background(), client, "i-none")
	if err != nil {
		t.Fatalf("spotInstanceState() error: %v", err)
	}
	if stateName != ec2types.InstanceStateNameTerminated {
		t.Errorf("state = %q, want %q", stateName, ec2types.InstanceStateNameTerminated)
	}
}

func TestSpotInstanceState_Error(t *testing.T) {
	client := &mockEC2{
		describeFunc: func(_ context.Context, _ *ec2.DescribeInstancesInput, _ ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
			return nil, errors.New("RequestExpired")
		},
	}
	_, _, err := spotInstanceState(context.Background(), client, "i-err")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- latestObjectByPrefix tests ---

func TestLatestObjectByPrefix_Empty(t *testing.T) {
	client := &mockS3{} // returns empty
	key, err := latestObjectByPrefix(context.Background(), client, "bucket", "")
	if err != nil {
		t.Fatalf("latestObjectByPrefix() error: %v", err)
	}
	if key != "" {
		t.Errorf("expected empty key, got %q", key)
	}
}

func TestLatestObjectByPrefix_Single(t *testing.T) {
	client := &mockS3{
		listFunc: func(_ context.Context, _ *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			return &s3.ListObjectsV2Output{
				Contents: []s3types.Object{{Key: aws.String("saves/world.zip")}},
			}, nil
		},
	}
	key, err := latestObjectByPrefix(context.Background(), client, "bucket", "saves/")
	if err != nil {
		t.Fatalf("latestObjectByPrefix() error: %v", err)
	}
	if key != "saves/world.zip" {
		t.Errorf("key = %q, want %q", key, "saves/world.zip")
	}
}

func TestLatestObjectByPrefix_ReturnsLexLast(t *testing.T) {
	client := &mockS3{
		listFunc: func(_ context.Context, _ *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			return &s3.ListObjectsV2Output{
				Contents: []s3types.Object{
					{Key: aws.String("2024-01-01T000000Z/world.zip")},
					{Key: aws.String("2024-03-15T120000Z/world.zip")},
					{Key: aws.String("2024-02-10T060000Z/world.zip")},
				},
			}, nil
		},
	}
	key, err := latestObjectByPrefix(context.Background(), client, "bucket", "")
	if err != nil {
		t.Fatalf("latestObjectByPrefix() error: %v", err)
	}
	if key != "2024-03-15T120000Z/world.zip" {
		t.Errorf("key = %q, want %q", key, "2024-03-15T120000Z/world.zip")
	}
}

func TestLatestObjectByPrefix_Error(t *testing.T) {
	client := &mockS3{
		listFunc: func(_ context.Context, _ *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			return nil, errors.New("NoSuchBucket")
		},
	}
	_, err := latestObjectByPrefix(context.Background(), client, "bucket", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- listCommonPrefixes tests ---

func TestListCommonPrefixes_Empty(t *testing.T) {
	client := &mockS3{}
	prefixes, err := listCommonPrefixes(context.Background(), client, "my-bucket")
	if err != nil {
		t.Fatalf("listCommonPrefixes() error: %v", err)
	}
	if len(prefixes) != 0 {
		t.Errorf("expected 0 prefixes, got %d", len(prefixes))
	}
}

func TestListCommonPrefixes_WithPrefixes(t *testing.T) {
	client := &mockS3{
		listFunc: func(_ context.Context, _ *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			return &s3.ListObjectsV2Output{
				CommonPrefixes: []s3types.CommonPrefix{
					{Prefix: aws.String("2026-03-20T120000Z/")},
					{Prefix: aws.String("2026-03-21T150405Z/")},
				},
			}, nil
		},
	}
	prefixes, err := listCommonPrefixes(context.Background(), client, "my-bucket")
	if err != nil {
		t.Fatalf("listCommonPrefixes() error: %v", err)
	}
	if len(prefixes) != 2 {
		t.Fatalf("expected 2 prefixes, got %d: %v", len(prefixes), prefixes)
	}
}

func TestListCommonPrefixes_UsesDelimiter(t *testing.T) {
	var gotDelimiter string
	client := &mockS3{
		listFunc: func(_ context.Context, params *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			if params.Delimiter != nil {
				gotDelimiter = *params.Delimiter
			}
			return &s3.ListObjectsV2Output{}, nil
		},
	}
	listCommonPrefixes(context.Background(), client, "bucket")
	if gotDelimiter != "/" {
		t.Errorf("delimiter = %q, want %q", gotDelimiter, "/")
	}
}

func TestListCommonPrefixes_Error(t *testing.T) {
	client := &mockS3{
		listFunc: func(_ context.Context, _ *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			return nil, errors.New("NoSuchBucket")
		},
	}
	_, err := listCommonPrefixes(context.Background(), client, "bucket")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
