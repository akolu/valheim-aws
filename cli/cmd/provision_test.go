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
	err := autoRestoreFromLongterm(context.Background(), client, "valheim", "eu-north-1")
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
	err := autoRestoreFromLongterm(context.Background(), client, "valheim", "eu-north-1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestAutoRestoreFromLongterm_ArchiveFound_CopiesFiles(t *testing.T) {
	var copyCalls []string
	client := &mockS3{
		listFunc: func(_ context.Context, params *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			// First call: list all objects to find the latest key.
			// Second call: list objects under the latest prefix.
			if aws.ToString(params.Prefix) == "" {
				return &s3.ListObjectsV2Output{
					Contents: []s3types.Object{
						{Key: aws.String("2024-01-01T000000Z/valheim_backup_latest.tar.gz")},
						{Key: aws.String("2024-02-01T000000Z/valheim_backup_latest.tar.gz")},
					},
				}, nil
			}
			// Prefix list for the latest snapshot.
			return &s3.ListObjectsV2Output{
				Contents: []s3types.Object{
					{Key: aws.String("2024-02-01T000000Z/valheim_backup_latest.tar.gz")},
				},
			}, nil
		},
		copyFunc: func(_ context.Context, params *s3.CopyObjectInput, _ ...func(*s3.Options)) (*s3.CopyObjectOutput, error) {
			copyCalls = append(copyCalls, aws.ToString(params.Key))
			return &s3.CopyObjectOutput{}, nil
		},
	}
	err := autoRestoreFromLongterm(context.Background(), client, "valheim", "eu-north-1")
	if err != nil {
		t.Fatalf("autoRestoreFromLongterm() error: %v", err)
	}
	if len(copyCalls) != 1 {
		t.Fatalf("expected 1 copy call, got %d: %v", len(copyCalls), copyCalls)
	}
	if copyCalls[0] != "valheim_backup_latest.tar.gz" {
		t.Errorf("copy destination key = %q, want %q", copyCalls[0], "valheim_backup_latest.tar.gz")
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
	autoRestoreFromLongterm(context.Background(), client, "satisfactory", "eu-north-1")
	if queriedBucket != "satisfactory-long-term-backups" {
		t.Errorf("queried bucket = %q, want %q", queriedBucket, "satisfactory-long-term-backups")
	}
}

func TestAutoRestoreFromLongterm_CopyDstBucket(t *testing.T) {
	var dstBucket string
	client := &mockS3{
		listFunc: func(_ context.Context, params *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			if aws.ToString(params.Prefix) == "" {
				return &s3.ListObjectsV2Output{
					Contents: []s3types.Object{
						{Key: aws.String("2024-01-01T000000Z/valheim_backup_latest.tar.gz")},
					},
				}, nil
			}
			return &s3.ListObjectsV2Output{
				Contents: []s3types.Object{
					{Key: aws.String("2024-01-01T000000Z/valheim_backup_latest.tar.gz")},
				},
			}, nil
		},
		copyFunc: func(_ context.Context, params *s3.CopyObjectInput, _ ...func(*s3.Options)) (*s3.CopyObjectOutput, error) {
			dstBucket = aws.ToString(params.Bucket)
			return &s3.CopyObjectOutput{}, nil
		},
	}
	autoRestoreFromLongterm(context.Background(), client, "valheim", "eu-north-1")
	want := "bonfire-valheim-backups-eu-north-1"
	if dstBucket != want {
		t.Errorf("copy destination bucket = %q, want %q", dstBucket, want)
	}
}
