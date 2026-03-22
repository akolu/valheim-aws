package cmd

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

func TestRestoreFromLongterm_NoSaves(t *testing.T) {
	client := &mockS3{} // empty bucket
	err := restoreFromLongterm(context.Background(), client, "valheim", strings.NewReader(""))
	if err != nil {
		t.Fatalf("restoreFromLongterm() error: %v", err)
	}
}

func TestRestoreFromLongterm_ListError(t *testing.T) {
	client := &mockS3{
		listFunc: func(_ context.Context, _ *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			return nil, errors.New("NoSuchBucket")
		},
	}
	err := restoreFromLongterm(context.Background(), client, "valheim", strings.NewReader(""))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRestoreFromLongterm_SkipRestore(t *testing.T) {
	client := &mockS3{
		listFunc: func(_ context.Context, _ *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			return &s3.ListObjectsV2Output{
				Contents: []s3types.Object{
					{Key: aws.String("2024-01-01T000000Z/world.fwl")},
				},
			}, nil
		},
	}
	// User enters 0 to skip
	err := restoreFromLongterm(context.Background(), client, "valheim", strings.NewReader("0\n"))
	if err != nil {
		t.Fatalf("restoreFromLongterm() error: %v", err)
	}
}

func TestRestoreFromLongterm_ValidSelection(t *testing.T) {
	client := &mockS3{
		listFunc: func(_ context.Context, _ *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			return &s3.ListObjectsV2Output{
				Contents: []s3types.Object{
					{Key: aws.String("2024-01-01T000000Z/world.fwl")},
					{Key: aws.String("2024-02-01T000000Z/world.fwl")},
				},
			}, nil
		},
	}
	// User selects save #1
	err := restoreFromLongterm(context.Background(), client, "valheim", strings.NewReader("1\n"))
	if err != nil {
		t.Fatalf("restoreFromLongterm() error: %v", err)
	}
}

func TestRestoreFromLongterm_InvalidSelection(t *testing.T) {
	client := &mockS3{
		listFunc: func(_ context.Context, _ *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			return &s3.ListObjectsV2Output{
				Contents: []s3types.Object{
					{Key: aws.String("2024-01-01T000000Z/world.fwl")},
				},
			}, nil
		},
	}
	// User enters 99 which is out of range
	err := restoreFromLongterm(context.Background(), client, "valheim", strings.NewReader("99\n"))
	if err == nil {
		t.Fatal("expected error for out-of-range selection, got nil")
	}
}

func TestRestoreFromLongterm_NonNumericInput(t *testing.T) {
	client := &mockS3{
		listFunc: func(_ context.Context, _ *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			return &s3.ListObjectsV2Output{
				Contents: []s3types.Object{
					{Key: aws.String("save.fwl")},
				},
			}, nil
		},
	}
	// Non-numeric input — should skip gracefully
	err := restoreFromLongterm(context.Background(), client, "valheim", strings.NewReader("nope\n"))
	if err != nil {
		t.Fatalf("restoreFromLongterm() error on non-numeric input: %v", err)
	}
}

func TestRestoreFromLongterm_LongtermBucketName(t *testing.T) {
	var queriedBucket string
	client := &mockS3{
		listFunc: func(_ context.Context, params *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			queriedBucket = aws.ToString(params.Bucket)
			return &s3.ListObjectsV2Output{}, nil
		},
	}
	restoreFromLongterm(context.Background(), client, "satisfactory", strings.NewReader(""))
	if queriedBucket != "satisfactory-long-term-backups" {
		t.Errorf("queried bucket = %q, want %q", queriedBucket, "satisfactory-long-term-backups")
	}
}
