package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const defaultRegion = "eu-north-1"

// awsConfig loads the AWS config (uses environment / profile credentials).
func awsConfig(ctx context.Context) (aws.Config, error) {
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

// listObjects returns all object keys in an S3 bucket with the given prefix.
func listObjects(ctx context.Context, s3Client *s3.Client, bucket, prefix string) ([]string, error) {
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
func copyObject(ctx context.Context, s3Client *s3.Client, srcBucket, srcKey, dstBucket, dstKey string) error {
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

// getObject downloads an S3 object and writes it to w.
func getObject(ctx context.Context, s3Client *s3.Client, bucket, key string, w io.Writer) error {
	resp, err := s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("getting s3://%s/%s: %w", bucket, key, err)
	}
	defer resp.Body.Close()
	if _, err := io.Copy(w, resp.Body); err != nil {
		return fmt.Errorf("reading s3://%s/%s: %w", bucket, key, err)
	}
	return nil
}

// instanceState returns the EC2 instance state for the given instance ID.
// Returns empty string if not found.
func instanceState(ctx context.Context, ec2Client *ec2.Client, instanceID string) (string, string, error) {
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

// spotInstanceState returns state for the given spot instance request.
func spotInstanceState(ctx context.Context, ec2Client *ec2.Client, instanceID string) (ec2types.InstanceStateName, string, error) {
	resp, err := ec2Client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		InstanceIds: []string{instanceID},
	})
	if err != nil {
		return "", "", err
	}
	for _, r := range resp.Reservations {
		for _, i := range r.Instances {
			ip := ""
			if i.PublicIpAddress != nil {
				ip = *i.PublicIpAddress
			}
			return i.State.Name, ip, nil
		}
	}
	return ec2types.InstanceStateNameTerminated, "", nil
}

// latestObjectByPrefix returns the lexicographically last key in a bucket with the given prefix.
func latestObjectByPrefix(ctx context.Context, s3Client *s3.Client, bucket, prefix string) (string, error) {
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
