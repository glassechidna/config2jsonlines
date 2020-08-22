package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/pkg/errors"
	"os"
	"regexp"
)

func main() {
	switch os.Getenv("MODE") {
	case "S3Event":
		lambda.Start(handleS3Event)
	case "Processor":
		lambda.Start(handleProcessor)
	case "Backfill":
		panic("not yet implemented")
	default:
		panic("unexpected mode")
	}
	lambda.Start(handleS3Event)
}

var snapshotRegexp = regexp.MustCompile(`AWSLogs/\d+/Config/[^/]+/\d+/\d+/\d+/ConfigSnapshot/.+`)

func handleS3Event(ctx context.Context, input *events.S3Event) error {
	sess, err := session.NewSession()
	if err != nil {
		return errors.WithStack(err)
	}

	api := sqs.New(sess)

	for _, record := range input.Records {
		bucket := record.S3.Bucket.Name
		key := record.S3.Object.Key

		if !snapshotRegexp.MatchString(key) {
			continue
		}

		payload := processInput{Bucket: bucket, Key: key, Size: int(record.S3.Object.Size)}
		payloadJson, _ := json.Marshal(payload)

		resp, err := api.SendMessageWithContext(ctx, &sqs.SendMessageInput{
			QueueUrl:    aws.String(os.Getenv("QUEUE_URL")),
			MessageBody: aws.String(string(payloadJson)),
		})
		if err != nil {
			return errors.WithStack(err)
		}

		jlog(map[string]interface{}{"msg": "sent message", "msgid": *resp.MessageId})
	}

	return nil
}

func jlog(in interface{}) {
	j, _ := json.Marshal(in)
	fmt.Println(string(j))
}
