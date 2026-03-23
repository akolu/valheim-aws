package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

const defaultRegion = "eu-north-1"

// awsConfig loads the AWS config (uses environment / profile credentials).
// If AWS_PROFILE is not set, it defaults to "bonfire-deploy".
func awsConfig(ctx context.Context) (aws.Config, error) {
	if os.Getenv("AWS_PROFILE") == "" {
		os.Setenv("AWS_PROFILE", "bonfire-deploy")
	}
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = os.Getenv("AWS_DEFAULT_REGION")
	}
	if region == "" {
		region = defaultRegion
	}
	return config.LoadDefaultConfig(ctx, config.WithRegion(region))
}

// backupBucketName returns the S3 backup bucket name for a game.
// Convention: bonfire-<game>-backups-<region>
func backupBucketName(game, region string) string {
	return fmt.Sprintf("bonfire-%s-backups-%s", game, region)
}

// longtermBucketName returns the long-term S3 bucket name for a game.
// Convention: <game>-long-term-backups (from terraform/archive/main.tf)
func longtermBucketName(game string) string {
	return fmt.Sprintf("%s-long-term-backups", game)
}

// s3API is the subset of the S3 client API used by this package.
type s3API interface {
	s3.ListObjectsV2APIClient
	CopyObject(ctx context.Context, params *s3.CopyObjectInput, optFns ...func(*s3.Options)) (*s3.CopyObjectOutput, error)
	ListObjectVersions(ctx context.Context, params *s3.ListObjectVersionsInput, optFns ...func(*s3.Options)) (*s3.ListObjectVersionsOutput, error)
	DeleteObjects(ctx context.Context, params *s3.DeleteObjectsInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectsOutput, error)
}

// ec2API is the subset of the EC2 client API used by this package.
type ec2API interface {
	DescribeInstances(ctx context.Context, params *ec2.DescribeInstancesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error)
}

// listObjects returns all object keys in an S3 bucket with the given prefix.
func listObjects(ctx context.Context, s3Client s3API, bucket, prefix string) ([]string, error) {
	var keys []string
	paginator := s3.NewListObjectsV2Paginator(s3Client, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(prefix),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("listing objects in s3://%s/%s: %w", bucket, prefix, err)
		}
		for _, obj := range page.Contents {
			if obj.Key != nil {
				keys = append(keys, *obj.Key)
			}
		}
	}
	return keys, nil
}

// copyObject copies an S3 object from srcBucket/srcKey to dstBucket/dstKey.
func copyObject(ctx context.Context, s3Client s3API, srcBucket, srcKey, dstBucket, dstKey string) error {
	_, err := s3Client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(dstBucket),
		Key:        aws.String(dstKey),
		CopySource: aws.String(srcBucket + "/" + srcKey),
	})
	if err != nil {
		return fmt.Errorf("copying s3://%s/%s → s3://%s/%s: %w", srcBucket, srcKey, dstBucket, dstKey, err)
	}
	return nil
}

// instanceState returns the EC2 instance state for the given instance ID.
// Returns empty string if not found.
func instanceState(ctx context.Context, ec2Client ec2API, instanceID string) (string, string, error) {
	if instanceID == "" {
		return "", "", nil
	}
	resp, err := ec2Client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		InstanceIds: []string{instanceID},
	})
	if err != nil {
		// Instance not found is not fatal for status display
		if strings.Contains(err.Error(), "InvalidInstanceID") {
			return "not-found", "", nil
		}
		return "", "", fmt.Errorf("describing instance %s: %w", instanceID, err)
	}
	for _, r := range resp.Reservations {
		for _, i := range r.Instances {
			state := string(i.State.Name)
			ip := ""
			if i.PublicIpAddress != nil {
				ip = *i.PublicIpAddress
			}
			return state, ip, nil
		}
	}
	return "not-found", "", nil
}

// listCommonPrefixes returns top-level "directory" prefixes in an S3 bucket by listing
// with delimiter '/'. Each returned string is a common prefix (e.g. "2026-03-21T150405Z/").
func listCommonPrefixes(ctx context.Context, s3Client s3API, bucket string) ([]string, error) {
	var prefixes []string
	paginator := s3.NewListObjectsV2Paginator(s3Client, &s3.ListObjectsV2Input{
		Bucket:    aws.String(bucket),
		Delimiter: aws.String("/"),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("listing prefixes in s3://%s: %w", bucket, err)
		}
		for _, p := range page.CommonPrefixes {
			if p.Prefix != nil {
				prefixes = append(prefixes, *p.Prefix)
			}
		}
	}
	return prefixes, nil
}

// latestObjectByPrefix returns the lexicographically last key in a bucket with the given prefix.
func latestObjectByPrefix(ctx context.Context, s3Client s3API, bucket, prefix string) (string, error) {
	keys, err := listObjects(ctx, s3Client, bucket, prefix)
	if err != nil {
		return "", err
	}
	if len(keys) == 0 {
		return "", nil
	}
	latest := keys[0]
	for _, k := range keys[1:] {
		if k > latest {
			latest = k
		}
	}
	return latest, nil
}

// emptyVersionedBucket deletes all object versions and delete markers from a
// versioned S3 bucket, leaving it empty so terraform destroy can remove it.
func emptyVersionedBucket(ctx context.Context, s3Client s3API, bucket string) error {
	input := &s3.ListObjectVersionsInput{Bucket: aws.String(bucket)}
	for {
		out, err := s3Client.ListObjectVersions(ctx, input)
		if err != nil {
			return fmt.Errorf("listing versions in s3://%s: %w", bucket, err)
		}

		var objects []s3types.ObjectIdentifier
		for _, v := range out.Versions {
			objects = append(objects, s3types.ObjectIdentifier{Key: v.Key, VersionId: v.VersionId})
		}
		for _, dm := range out.DeleteMarkers {
			objects = append(objects, s3types.ObjectIdentifier{Key: dm.Key, VersionId: dm.VersionId})
		}

		if len(objects) > 0 {
			_, err = s3Client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
				Bucket: aws.String(bucket),
				Delete: &s3types.Delete{Objects: objects, Quiet: aws.Bool(true)},
			})
			if err != nil {
				return fmt.Errorf("deleting objects from s3://%s: %w", bucket, err)
			}
		}

		if !aws.ToBool(out.IsTruncated) {
			break
		}
		input.KeyMarker = out.NextKeyMarker
		input.VersionIdMarker = out.NextVersionIdMarker
	}
	return nil
}
