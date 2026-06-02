package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/cenkalti/backoff/v4"
)

type MinioConfig struct {
	Endpoint     string
	AccessKey    string
	SecretKey    string
	Region       string
	Bucket       string
	UsePathStyle bool
}

type MinioClient struct {
	cfg    MinioConfig
	client *s3.Client
}

func NewMinioClient(ctx context.Context, mc MinioConfig) (*MinioClient, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(mc.Region),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(mc.AccessKey, mc.SecretKey, ""),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("aws config load: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(mc.Endpoint)
		o.UsePathStyle = mc.UsePathStyle
	})

	return &MinioClient{cfg: mc, client: client}, nil
}

// EnsureBucket waits for MinIO readiness with exponential backoff,
// then creates the bucket if missing. Idempotent.
func (m *MinioClient) EnsureBucket(ctx context.Context) error {
	op := func() error {
		_, err := m.client.HeadBucket(ctx, &s3.HeadBucketInput{
			Bucket: aws.String(m.cfg.Bucket),
		})
		if err == nil {
			return nil
		}

		var notFound *s3types.NotFound
		if errors.As(err, &notFound) {
			log.Printf("[minio] bucket %q not found, creating...", m.cfg.Bucket)
			_, cerr := m.client.CreateBucket(ctx, &s3.CreateBucketInput{
				Bucket: aws.String(m.cfg.Bucket),
			})
			if cerr != nil {
				return fmt.Errorf("create bucket: %w", cerr)
			}
			log.Printf("[minio] bucket %q created", m.cfg.Bucket)
			return nil
		}

		return fmt.Errorf("head bucket: %w", err)
	}

	bo := backoff.NewExponentialBackOff()
	bo.InitialInterval = 500 * time.Millisecond
	bo.MaxInterval = 5 * time.Second
	bo.MaxElapsedTime = 60 * time.Second

	return backoff.Retry(op, backoff.WithContext(bo, ctx))
}

func (m *MinioClient) Client() *s3.Client { return m.client }

func (m *MinioClient) Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) error {
	_, err := m.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(m.cfg.Bucket),
		Key:           aws.String(key),
		Body:          body,
		ContentLength: aws.Int64(size),
		ContentType:   aws.String(contentType),
	})
	if err != nil {
		return fmt.Errorf("s3 put %s: %w", key, err)
	}
	return nil
}

func (m *MinioClient) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	out, err := m.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(m.cfg.Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("s3 get %s: %w", key, err)
	}
	return out.Body, nil
}

func (m *MinioClient) Delete(ctx context.Context, key string) error {
	_, err := m.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(m.cfg.Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("s3 delete %s: %w", key, err)
	}
	return nil
}
