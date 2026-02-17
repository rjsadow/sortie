package recordings

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3API defines the subset of the S3 client used by S3Store, enabling test mocking.
type S3API interface {
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	DeleteObject(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
}

// S3Store implements RecordingStore using an S3-compatible object store.
type S3Store struct {
	client S3API
	bucket string
	prefix string
}

// NewS3Store creates an S3Store configured from AWS defaults and the given parameters.
// An empty endpoint uses the standard AWS S3 endpoint; a non-empty endpoint targets
// MinIO or another S3-compatible service. When accessKeyID and secretAccessKey are
// both non-empty, static credentials are used instead of the default credential chain.
func NewS3Store(bucket, region, endpoint, prefix, accessKeyID, secretAccessKey string) (*S3Store, error) {
	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(region),
	}

	if accessKeyID != "" && secretAccessKey != "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, ""),
		))
	}

	cfg, err := awsconfig.LoadDefaultConfig(context.Background(), opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	var s3Opts []func(*s3.Options)
	if endpoint != "" {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(endpoint)
			o.UsePathStyle = true // required for MinIO
		})
	}

	client := s3.NewFromConfig(cfg, s3Opts...)
	return NewS3StoreWithClient(client, bucket, prefix), nil
}

// NewS3StoreWithClient creates an S3Store with an injected S3API client (for testing).
func NewS3StoreWithClient(client S3API, bucket, prefix string) *S3Store {
	return &S3Store{
		client: client,
		bucket: bucket,
		prefix: prefix,
	}
}

// Save uploads a recording to S3 and returns the object key as the storage path.
func (s *S3Store) Save(id string, r io.Reader) (string, error) {
	now := time.Now()
	key := fmt.Sprintf("%s%d/%02d/%s.vncrec", s.prefix, now.Year(), now.Month(), id)

	_, err := s.client.PutObject(context.Background(), &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        r,
		ContentType: aws.String("application/octet-stream"),
	})
	if err != nil {
		return "", fmt.Errorf("failed to upload recording to S3: %w", err)
	}

	return key, nil
}

// Get returns the S3 object body as an io.ReadCloser.
func (s *S3Store) Get(storagePath string) (io.ReadCloser, error) {
	out, err := s.client.GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(storagePath),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get recording from S3: %w", err)
	}
	return out.Body, nil
}

// Delete removes the recording object from S3.
func (s *S3Store) Delete(storagePath string) error {
	_, err := s.client.DeleteObject(context.Background(), &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(storagePath),
	})
	if err != nil {
		return fmt.Errorf("failed to delete recording from S3: %w", err)
	}
	return nil
}
