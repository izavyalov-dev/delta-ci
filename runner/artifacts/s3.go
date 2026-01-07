package artifacts

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Config configures the S3 uploader.
type S3Config struct {
	Bucket string
	Prefix string
	Region string
}

// S3Uploader uploads artifacts to AWS S3.
type S3Uploader struct {
	client *s3.Client
	bucket string
	prefix string
}

// NewS3Uploader loads AWS config and prepares an uploader.
func NewS3Uploader(ctx context.Context, cfg S3Config) (*S3Uploader, error) {
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("s3 bucket is required")
	}

	loadOpts := []func(*config.LoadOptions) error{}
	if cfg.Region != "" {
		loadOpts = append(loadOpts, config.WithRegion(cfg.Region))
	}

	awsCfg, err := config.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return nil, err
	}

	return &S3Uploader{
		client: s3.NewFromConfig(awsCfg),
		bucket: cfg.Bucket,
		prefix: strings.Trim(cfg.Prefix, "/"),
	}, nil
}

// UploadLog uploads the log file and returns a s3:// URI.
func (u *S3Uploader) UploadLog(ctx context.Context, runID, jobID, logPath string) (string, error) {
	key := u.objectKey("runs", runID, "jobs", jobID, "log.txt")
	file, err := os.Open(logPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	_, err = u.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      &u.bucket,
		Key:         &key,
		Body:        file,
		ContentType: ptr("text/plain"),
	})
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("s3://%s/%s", u.bucket, key), nil
}

func (u *S3Uploader) objectKey(parts ...string) string {
	if u.prefix == "" {
		return path.Join(parts...)
	}
	return path.Join(append([]string{u.prefix}, parts...)...)
}

func ptr[T any](v T) *T {
	return &v
}
