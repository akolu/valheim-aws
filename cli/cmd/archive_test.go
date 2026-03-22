package cmd

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

func TestArchiveGame_EmptyBucket(t *testing.T) {
	client := &mockS3{} // returns empty ListObjectsV2
	err := archiveGame(context.Background(), client, "eu-north-1", "valheim")
	if err != nil {
		t.Fatalf("archiveGame() error: %v", err)
	}
}

func TestArchiveGame_CopiesObjects(t *testing.T) {
	var copiedSrcs []string
	client := &mockS3{
		listFunc: func(_ context.Context, _ *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			return &s3.ListObjectsV2Output{
				Contents: []s3types.Object{
					{Key: aws.String("worlds/Valheim.fwl")},
					{Key: aws.String("worlds/Valheim.db")},
				},
			}, nil
		},
		copyFunc: func(_ context.Context, params *s3.CopyObjectInput, _ ...func(*s3.Options)) (*s3.CopyObjectOutput, error) {
			copiedSrcs = append(copiedSrcs, aws.ToString(params.CopySource))
			return &s3.CopyObjectOutput{}, nil
		},
	}
	err := archiveGame(context.Background(), client, "eu-north-1", "valheim")
	if err != nil {
		t.Fatalf("archiveGame() error: %v", err)
	}
	if len(copiedSrcs) != 2 {
		t.Errorf("expected 2 copies, got %d: %v", len(copiedSrcs), copiedSrcs)
	}
}

func TestArchiveGame_SkipsDirectoryMarkers(t *testing.T) {
	var copyCount int
	client := &mockS3{
		listFunc: func(_ context.Context, _ *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			return &s3.ListObjectsV2Output{
				Contents: []s3types.Object{
					{Key: aws.String("worlds/")},           // directory marker — skip
					{Key: aws.String("worlds/world.fwl")},  // real file — copy
				},
			}, nil
		},
		copyFunc: func(_ context.Context, _ *s3.CopyObjectInput, _ ...func(*s3.Options)) (*s3.CopyObjectOutput, error) {
			copyCount++
			return &s3.CopyObjectOutput{}, nil
		},
	}
	err := archiveGame(context.Background(), client, "eu-north-1", "valheim")
	if err != nil {
		t.Fatalf("archiveGame() error: %v", err)
	}
	if copyCount != 1 {
		t.Errorf("expected 1 copy (directory skipped), got %d", copyCount)
	}
}

func TestArchiveGame_ListError(t *testing.T) {
	client := &mockS3{
		listFunc: func(_ context.Context, _ *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			return nil, errors.New("NoSuchBucket")
		},
	}
	err := archiveGame(context.Background(), client, "eu-north-1", "valheim")
	if err == nil {
		t.Fatal("expected error from list failure, got nil")
	}
}

func TestArchiveGame_CopyError(t *testing.T) {
	client := &mockS3{
		listFunc: func(_ context.Context, _ *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			return &s3.ListObjectsV2Output{
				Contents: []s3types.Object{
					{Key: aws.String("worlds/world.fwl")},
				},
			}, nil
		},
		copyFunc: func(_ context.Context, _ *s3.CopyObjectInput, _ ...func(*s3.Options)) (*s3.CopyObjectOutput, error) {
			return nil, errors.New("AccessDenied")
		},
	}
	err := archiveGame(context.Background(), client, "eu-north-1", "valheim")
	if err == nil {
		t.Fatal("expected error from copy failure, got nil")
	}
}

func TestArchiveGame_BucketNamesCorrect(t *testing.T) {
	var srcBuckets, dstBuckets []string
	client := &mockS3{
		listFunc: func(_ context.Context, params *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			srcBuckets = append(srcBuckets, aws.ToString(params.Bucket))
			return &s3.ListObjectsV2Output{
				Contents: []s3types.Object{
					{Key: aws.String("save.zip")},
				},
			}, nil
		},
		copyFunc: func(_ context.Context, params *s3.CopyObjectInput, _ ...func(*s3.Options)) (*s3.CopyObjectOutput, error) {
			dstBuckets = append(dstBuckets, aws.ToString(params.Bucket))
			return &s3.CopyObjectOutput{}, nil
		},
	}
	err := archiveGame(context.Background(), client, "eu-north-1", "satisfactory")
	if err != nil {
		t.Fatalf("archiveGame() error: %v", err)
	}
	if len(srcBuckets) == 0 || srcBuckets[0] != "bonfire-satisfactory-backups-eu-north-1" {
		t.Errorf("src bucket = %v, want bonfire-satisfactory-backups-eu-north-1", srcBuckets)
	}
	if len(dstBuckets) == 0 || dstBuckets[0] != "satisfactory-long-term-backups" {
		t.Errorf("dst bucket = %v, want satisfactory-long-term-backups", dstBuckets)
	}
}
