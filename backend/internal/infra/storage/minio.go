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
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
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

// ListOlder returns keys under prefix last modified before cutoff.
func (m *MinioClient) ListOlder(ctx context.Context, prefix string, cutoff time.Time) ([]string, error) {
	var keys []string
	pages := s3.NewListObjectsV2Paginator(m.client, &s3.ListObjectsV2Input{Bucket: aws.String(m.cfg.Bucket), Prefix: aws.String(prefix)})
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("s3 list %s: %w", prefix, err)
		}
		for _, object := range page.Contents {
			if object.Key != nil && object.LastModified != nil && object.LastModified.Before(cutoff) {
				keys = append(keys, *object.Key)
			}
		}
	}
	return keys, nil
}

// GetRange reads length bytes starting at offset.
func (m *MinioClient) GetRange(ctx context.Context, key string, offset, length int64) (io.ReadCloser, error) {
	if offset < 0 || length <= 0 {
		return nil, fmt.Errorf("s3 range %s: invalid range", key)
	}
	out, err := m.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(m.cfg.Bucket),
		Key:    aws.String(key),
		Range:  aws.String(fmt.Sprintf("bytes=%d-%d", offset, offset+length-1)),
	})
	if err != nil {
		return nil, fmt.Errorf("s3 range get %s: %w", key, err)
	}
	return out.Body, nil
}

func (m *MinioClient) CreateMultipart(ctx context.Context, key, contentType string) (string, error) {
	out, err := m.client.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{
		Bucket:      aws.String(m.cfg.Bucket),
		Key:         aws.String(key),
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return "", fmt.Errorf("s3 create multipart %s: %w", key, err)
	}
	if out.UploadId == nil || *out.UploadId == "" {
		return "", fmt.Errorf("s3 create multipart %s: empty upload id", key)
	}
	return *out.UploadId, nil
}

// UploadPart needs a seekable body: the SDK signs and may retry the payload.
func (m *MinioClient) UploadPart(ctx context.Context, key, uploadID string, number int32, body io.ReadSeeker, size int64) error {
	_, err := m.client.UploadPart(ctx, &s3.UploadPartInput{
		Bucket:        aws.String(m.cfg.Bucket),
		Key:           aws.String(key),
		UploadId:      aws.String(uploadID),
		PartNumber:    aws.Int32(number),
		Body:          body,
		ContentLength: aws.Int64(size),
	})
	if err != nil {
		return fmt.Errorf("s3 upload part %s #%d: %w", key, number, err)
	}
	return nil
}

func (m *MinioClient) ListParts(ctx context.Context, key, uploadID string) ([]ports.StoragePart, error) {
	var parts []ports.StoragePart
	pages := s3.NewListPartsPaginator(m.client, &s3.ListPartsInput{Bucket: aws.String(m.cfg.Bucket), Key: aws.String(key), UploadId: aws.String(uploadID)})
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("s3 list parts %s: %w", key, err)
		}
		for _, p := range page.Parts {
			if p.PartNumber == nil || p.Size == nil || p.ETag == nil {
				continue
			}
			parts = append(parts, ports.StoragePart{Number: *p.PartNumber, Size: *p.Size, ETag: *p.ETag})
		}
	}
	return parts, nil
}

func (m *MinioClient) CompleteMultipart(ctx context.Context, key, uploadID string, parts []ports.StoragePart) error {
	completed := make([]s3types.CompletedPart, 0, len(parts))
	for _, p := range parts {
		completed = append(completed, s3types.CompletedPart{PartNumber: aws.Int32(p.Number), ETag: aws.String(p.ETag)})
	}
	_, err := m.client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket:          aws.String(m.cfg.Bucket),
		Key:             aws.String(key),
		UploadId:        aws.String(uploadID),
		MultipartUpload: &s3types.CompletedMultipartUpload{Parts: completed},
	})
	if err != nil {
		return fmt.Errorf("s3 complete multipart %s: %w", key, err)
	}
	return nil
}

func (m *MinioClient) AbortMultipart(ctx context.Context, key, uploadID string) error {
	_, err := m.client.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{Bucket: aws.String(m.cfg.Bucket), Key: aws.String(key), UploadId: aws.String(uploadID)})
	if err != nil {
		return fmt.Errorf("s3 abort multipart %s: %w", key, err)
	}
	return nil
}

func (m *MinioClient) ObjectSize(ctx context.Context, key string) (int64, error) {
	out, err := m.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(m.cfg.Bucket), Key: aws.String(key)})
	if err != nil {
		return 0, fmt.Errorf("s3 head %s: %w", key, err)
	}
	if out.ContentLength == nil {
		return 0, fmt.Errorf("s3 head %s: missing length", key)
	}
	return *out.ContentLength, nil
}
