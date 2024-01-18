package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/kinesis"
	consumer "github.com/harlow/kinesis-consumer"
	"github.com/urfave/cli/v2"
	"go.uber.org/zap"

	offline "github.com/geode-io/aws-emulators"
)

const (
	AwsRegion       = "aws-region"
	KinesisEndpoint = "kinesis-endpoint"
	KinesisStream   = "kinesis-stream"
)

type kinesisLoggerShim struct {
	logger *zap.SugaredLogger
}

func (l *kinesisLoggerShim) Log(args ...interface{}) {
	l.logger.Info(args...)
}

func init() {
	logger, err := zap.NewDevelopment()
	if err != nil {
		log.Fatalf("cannot initialize zap logger: %v", err)
	}
	zap.ReplaceGlobals(logger)
	//nolint:errcheck
	defer logger.Sync()
}

func main() {
	flags := []cli.Flag{
		&cli.StringFlag{
			Name:    AwsRegion,
			EnvVars: []string{"AWS_REGION"},
			Value:   "us-east-1",
			Usage:   "AWS region to use",
		},
		&cli.StringFlag{
			Name:    KinesisEndpoint,
			EnvVars: []string{"KINESIS_ENDPOINT"},
			Usage:   "Endpoint to use for kinesis. i.e. http://localhost:4566",
		},
		&cli.StringFlag{
			Name:    KinesisStream,
			EnvVars: []string{"KINESIS_STREAM"},
			Usage:   "Kinesis stream to read from",
		},
	}

	flags = append(flags, offline.LambdaFlags()...)
	flags = append(flags, offline.LambdaInvokeFlags("")...)

	app := &cli.App{
		Name:  "kinesis-subscription-emulator",
		Usage: "Invoke a lambda function via http for every message in the kinesis stream",
		Flags: flags,
		Action: func(cliCtx *cli.Context) error {
			awsRegion := cliCtx.String(AwsRegion)
			kinesisEndpoint := cliCtx.String(KinesisEndpoint)
			streamName := cliCtx.String(KinesisStream)

			zap.L().Info("initializing kinesis subscriber",
				zap.String("kinesis.endpoint", kinesisEndpoint),
				zap.String("aws.region", awsRegion),
				zap.String("kinesis.stream", streamName),
			)

			resolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...any) (aws.Endpoint, error) {
				if service != kinesis.ServiceID {
					return aws.Endpoint{}, fmt.Errorf("unsupported service %s", service)
				}
				if region != awsRegion {
					return aws.Endpoint{}, fmt.Errorf("unsupported region %s", region)
				}
				return aws.Endpoint{
					PartitionID:   "aws",
					URL:           kinesisEndpoint,
					SigningRegion: awsRegion,
				}, nil
			})

			cfg, err := config.LoadDefaultConfig(
				context.Background(),
				config.WithRegion(awsRegion),
				config.WithEndpointResolverWithOptions(resolver),
				config.WithCredentialsProvider(
					credentials.NewStaticCredentialsProvider("canned", "canned", "kinesis-subscription-emulator"),
				),
			)
			if err != nil {
				zap.L().Fatal("unable to load SDK config", zap.Error(err))
			}
			var client = kinesis.NewFromConfig(cfg)

			c, err := consumer.New(
				streamName,
				consumer.WithClient(client),
				consumer.WithLogger(&kinesisLoggerShim{logger: zap.S()}),
			)
			if err != nil {
				zap.L().Fatal("consumer error", zap.Error(err))
			}

			zap.L().Info("starting kinesis consumer for lambda",
				zap.String("kinesis.stream", streamName),
			)

			ctx := offline.TrapProcess()
			err = c.Scan(ctx, func(r *consumer.Record) error {
				return handle(cliCtx, "", r)
			})
			if err != nil {
				zap.L().Fatal("scan error", zap.Error(err))
			}
			return err
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func handle(ctx *cli.Context, functionName string, record *consumer.Record) error {
	sequenceNumber := "0"
	if record.SequenceNumber != nil {
		sequenceNumber = *record.SequenceNumber
	}
	arrivalTime := time.Now()
	if record.ApproximateArrivalTimestamp != nil {
		arrivalTime = *record.ApproximateArrivalTimestamp
	}
	partitionKey := ""
	if record.PartitionKey != nil {
		partitionKey = *record.PartitionKey
	}

	event := events.KinesisEvent{
		Records: []events.KinesisEventRecord{
			{
				EventSource:       "aws:kinesis",
				EventVersion:      "0",
				EventID:           fmt.Sprintf("%s:%s", record.ShardID, sequenceNumber),
				EventName:         "aws:kinesis:record",
				InvokeIdentityArn: "arn:aws:iam::000000000000:role/canned-role",
				Kinesis: events.KinesisRecord{
					ApproximateArrivalTimestamp: events.SecondsEpochTime{Time: arrivalTime},
					Data:                        record.Data,
					PartitionKey:                partitionKey,
					SequenceNumber:              sequenceNumber,
					KinesisSchemaVersion:        "0",
				},
			},
		},
	}

	eventJSON, err := json.Marshal(event)
	if err != nil {
		zap.L().Error("failed to marshal event", zap.Error(err))
		return err
	}

	res, err := offline.LambdaInvokeFromCLI(ctx, functionName, eventJSON)
	if err != nil {
		return err
	}

	zap.L().Info("lambda invoked", zap.String("response", string(res)))

	return nil
}
