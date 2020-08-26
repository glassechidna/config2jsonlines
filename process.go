package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
	"io"
	"os"
	"strings"
	"time"
)

type processInput struct {
	Bucket string
	Key    string
	Size   int
}

func handleProcessor(ctx context.Context, event *events.SQSEvent) error {
	sess, err := session.NewSession()
	if err != nil {
		return errors.WithStack(err)
	}

	api := s3.New(sess)

	for _, record := range event.Records {
		input := processInput{}
		err = json.Unmarshal([]byte(record.Body), &input)
		if err != nil {
			return errors.WithStack(err)
		}

		jlog(map[string]interface{}{
			"msg":         "start processing",
			"inputBucket": input.Bucket,
			"inputKey":    input.Key,
			"size":        input.Size,
		})

		outputBucket := os.Getenv("OUTPUT_BUCKET")
		outputPrefix := strings.Trim(os.Getenv("OUTPUT_PREFIX"), "/")
		outputKey := strings.Trim(fmt.Sprintf("%s%s", outputPrefix, input.Key), "/")
		err = processConfigSnapshot(ctx, api, input.Bucket, input.Key, outputBucket, outputKey)
		if err != nil {
			return err
		}
	}

	return nil
}

func processConfigSnapshot(ctx context.Context, api s3iface.S3API, inputBucket, inputKey, outputBucket, outputKey string) error {
	g, ctx := errgroup.WithContext(ctx)

	start := time.Now()

	buf := aws.NewWriteAtBuffer(make([]byte, 0, 60_000_000))
	downloader := s3manager.NewDownloaderWithClient(api)
	downloader.Concurrency = 1

	_, err := downloader.DownloadWithContext(ctx, buf, &s3.GetObjectInput{Bucket: &inputBucket, Key: &inputKey})
	if err != nil {
		return errors.WithStack(err)
	}

	gzr, err := gzip.NewReader(bytes.NewReader(buf.Bytes()))
	if err != nil {
		return errors.WithStack(err)
	}

	pr, pw := io.Pipe()
	gzw := gzip.NewWriter(pw)

	g.Go(func() error {
		uploader := s3manager.NewUploaderWithClient(api)
		_, err := uploader.UploadWithContext(ctx, &s3manager.UploadInput{Bucket: &outputBucket, Key: &outputKey, Body: pr})
		return errors.WithStack(err)
	})

	countch := make(chan int, 1)

	g.Go(func() error {
		count, err := jsonArrayToJsonlines(gzr, gzw)
		if err != nil {
			return err
		}

		countch <- count

		err = gzw.Close()
		if err != nil {
			return errors.WithStack(err)
		}

		err = pw.Close()
		if err != nil {
			return errors.WithStack(err)
		}

		return nil
	})

	err = g.Wait()
	if err != nil {
		return err
	}

	count := <-countch
	duration := time.Now().Sub(start).Milliseconds()

	jlog(map[string]interface{}{
		"msg":         "finished processing",
		"inputBucket": inputBucket,
		"inputKey":    inputKey,
		"count":       count,
		"durationMs":  duration,
	})

	return nil
}

func jsonArrayToJsonlines(in io.Reader, out io.Writer) (int, error) {
	dec := json.NewDecoder(in)
	_, err := dec.Token()
	if err != nil {
		return 0, errors.WithStack(err)
	}

	j := json.RawMessage{}

	count := 0

	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return count, errors.WithStack(err)
		}

		if prop, ok := t.(string); !ok || prop != "configurationItems" {
			continue
		}

		_, err = dec.Token()
		if err != nil {
			return count, errors.WithStack(err)
		}

		for dec.More() {
			count++

			err = dec.Decode(&j)
			if err != nil {
				return count, errors.WithStack(err)
			}

			_, err = out.Write(append(j, '\n'))
			if err != nil {
				return count, errors.WithStack(err)
			}

			j = j[:0]
		}
	}

	return count, nil
}
