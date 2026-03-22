package cmd

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

func TestAutoRestoreFromLongterm_NoArchive(t *testing.T) {
	client := &mockS3{} // empty bucket
	err := autoRestoreFromLongterm(context.Background(), client, "valheim")
	if err != nil {
		t.Fatalf("autoRestoreFromLongterm() error: %v", err)
	}
}

func TestAutoRestoreFromLongterm_ListError(t *testing.T) {
	client := &mockS3{
		listFunc: func(_ context.Context, _ *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			return nil, errors.New("NoSuchBucket")
		},
	}
	err := autoRestoreFromLongterm(context.Background(), client, "valheim")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestAutoRestoreFromLongterm_ArchiveFound(t *testing.T) {
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
	err := autoRestoreFromLongterm(context.Background(), client, "valheim")
	if err != nil {
		t.Fatalf("autoRestoreFromLongterm() error: %v", err)
	}
}

func TestAutoRestoreFromLongterm_BucketName(t *testing.T) {
	var queriedBucket string
	client := &mockS3{
		listFunc: func(_ context.Context, params *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			queriedBucket = aws.ToString(params.Bucket)
			return &s3.ListObjectsV2Output{}, nil
		},
	}
	autoRestoreFromLongterm(context.Background(), client, "satisfactory")
	if queriedBucket != "satisfactory-long-term-backups" {
		t.Errorf("queried bucket = %q, want %q", queriedBucket, "satisfactory-long-term-backups")
	}
}
