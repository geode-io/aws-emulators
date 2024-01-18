package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/kinesis"
	"github.com/gorilla/mux"
	"github.com/urfave/cli/v2"
	"go.uber.org/zap"

	offline "github.com/geode-io/aws-emulators"
	"github.com/geode-io/aws-emulators/websocket"
)

const (
	AwsRegion          = "aws-region"
	WebsocketAPIPort   = "websocket-api-port"
	WebsocketAPIStage  = "websocket-api-stage"
	ManagementAPIPort  = "mgmt-api-port"
	KinesisEndpoint    = "kinesis-endpoint"
	KinesisStream      = "kinesis-stream"
	FunctionConnect    = "connect"
	FunctionDisconnect = "disconnect"
)

var (
	logger *zap.Logger
)

func init() {
	var err error
	logger, err = zap.NewDevelopment()
	if err != nil {
		log.Fatalf("cannot initialize zap logger: %v", err)
	}
	zap.ReplaceGlobals(logger)
}

func registerConnectLambda(cliCtx *cli.Context, hub *websocket.Hub) {
	var onConnect func(websocket.Connection)
	var onDisconnect func(websocket.Connection)

	if cliCtx.IsSet(offline.FunctionNameForFunction(FunctionConnect)) {
		onConnect = func(connection websocket.Connection) {
			zap.L().Info("invoking connect lambda",
				zap.String("connection.id", connection.ID),
			)

			payload := events.APIGatewayWebsocketProxyRequest{
				RequestContext: events.APIGatewayWebsocketProxyRequestContext{
					ConnectionID: connection.ID,
				},
			}

			payloadBytes, err := json.Marshal(payload)
			if err != nil {
				zap.L().Error("failed to marshal payload", zap.Error(err))
				return
			}

			_, err = offline.LambdaInvokeFromCLI(cliCtx, FunctionConnect, payloadBytes)
			if err != nil {
				zap.L().Error("failed to invoke connect lambda", zap.Error(err))
			}
		}
	}

	if cliCtx.IsSet(offline.FunctionNameForFunction(FunctionDisconnect)) {
		onDisconnect = func(connection websocket.Connection) {
			zap.L().Info("invoking disconnect lambda",
				zap.String("connection.id", connection.ID),
			)

			payload := events.APIGatewayWebsocketProxyRequest{
				RequestContext: events.APIGatewayWebsocketProxyRequestContext{
					ConnectionID: connection.ID,
				},
			}

			payloadBytes, err := json.Marshal(payload)
			if err != nil {
				zap.L().Error("failed to marshal payload", zap.Error(err))
				return
			}

			_, err = offline.LambdaInvokeFromCLI(cliCtx, FunctionDisconnect, payloadBytes)
			if err != nil {
				zap.L().Error("failed to invoke disconnect lambda", zap.Error(err))
			}
		}
	}

	if onConnect != nil || onDisconnect != nil {
		hub.RegisterListener(&websocket.Listener{
			ID:           "connection-lambdas",
			OnConnect:    onConnect,
			OnDisconnect: onDisconnect,
		})
	}
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
		&cli.IntFlag{
			Name:    WebsocketAPIPort,
			EnvVars: []string{"WEBSOCKET_API_PORT"},
			Value:   8080,
			Usage:   "Port to listen on for websocket connections",
		},
		&cli.StringFlag{
			Name:    WebsocketAPIStage,
			EnvVars: []string{"WEBSOCKET_API_STAGE"},
			Value:   "/ws",
			Usage:   "Emulated API gateway stage for websocket connections",
		},
		&cli.IntFlag{
			Name:    ManagementAPIPort,
			EnvVars: []string{"MANAGEMENT_API_PORT"},
			Value:   8081,
			Usage:   "Port to listen on for API gateway management requests",
		},
	}

	flags = append(flags, offline.LambdaFlags()...)
	flags = append(flags, offline.LambdaInvokeFlags(FunctionConnect)...)
	flags = append(flags, offline.LambdaInvokeFlags(FunctionDisconnect)...)

	app := &cli.App{
		Name:  "api-gateway-websocket-emulator",
		Usage: "Websocket server that emulates API Gateway websocket capabilities",
		Flags: flags,
		Action: func(cliCtx *cli.Context) error {
			//nolint:errcheck
			defer logger.Sync()

			awsRegion := cliCtx.String(AwsRegion)
			kinesisEndpoint := cliCtx.String(KinesisEndpoint)
			streamName := cliCtx.String(KinesisStream)

			zap.L().Info("initializing api-gateway websocket emulator",
				zap.String("aws.region", awsRegion),
				zap.String("kinesis.endpoint", kinesisEndpoint),
				zap.String("kinesis.stream", streamName),
				zap.String("websocket.api.stage", cliCtx.String(WebsocketAPIStage)),
				zap.Int("websocket.api.port", cliCtx.Int(WebsocketAPIPort)),
				zap.Int("management.api.port", cliCtx.Int(ManagementAPIPort)),
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
					credentials.NewStaticCredentialsProvider("canned", "canned", "api-gateway-websocket-emulator"),
				),
			)
			if err != nil {
				return err
			}

			var client = kinesis.NewFromConfig(cfg)
			hub := websocket.NewHub()
			ctx := offline.TrapProcess()

			go hub.Run(ctx)
			hub.RegisterListener(&websocket.Listener{
				ID: fmt.Sprintf("kinesis-listener-%s", streamName),
				OnMessage: func(msg websocket.Msg) {
					zap.L().Info("receive message from websocket connection for kinesis",
						zap.String("connection.id", msg.ConnectionID),
						zap.String("kinesis.stream", streamName),
					)

					// TODO: generate the following from the api gateway request template
					templatedData := struct {
						ConnectionID string `json:"connection_id"`
						SentAtMillis int64  `json:"sent_at_millis"`
						Data         []byte `json:"data"`
					}{
						ConnectionID: msg.ConnectionID,
						SentAtMillis: time.Now().UnixNano() / int64(time.Millisecond),
						Data:         msg.Data,
					}

					templatedDataBytes, err := json.Marshal(templatedData)
					if err != nil {
						zap.L().Error("failed to marshal templated data", zap.Error(err))
						return
					}

					_, err = client.PutRecord(context.Background(), &kinesis.PutRecordInput{
						Data:         []byte(base64.StdEncoding.EncodeToString(templatedDataBytes)),
						StreamName:   aws.String(streamName),
						PartitionKey: aws.String(msg.ConnectionID),
					})
					if err != nil {
						zap.L().Error("failed to put record to kinesis", zap.Error(err))
					}
				},
			})
			registerConnectLambda(cliCtx, hub)

			websocketPath := strings.TrimPrefix(cliCtx.String(WebsocketAPIStage), "/")
			mgmtRouter := mux.NewRouter()
			mgmtRouter.HandleFunc(
				fmt.Sprintf("/%s/@connections/{connectionID}", websocketPath),
				func(w http.ResponseWriter, r *http.Request) {
					params := mux.Vars(r)
					connectionID := params["connectionID"]

					zap.L().Info(
						"received message for websocket from management API",
						zap.String("connection.id", connectionID),
					)

					if !hub.HasConnection(connectionID) {
						w.WriteHeader(http.StatusGone)
						return
					}

					bodyData, err := io.ReadAll(r.Body)
					if err != nil {
						w.WriteHeader(http.StatusInternalServerError)
						return
					}

					hub.SendOutboundMessage(&websocket.Msg{
						ConnectionID: connectionID,
						Data:         bodyData,
					})
					w.WriteHeader(http.StatusOK)
				})

			mgmtServer := http.Server{
				Addr:              fmt.Sprintf(":%d", cliCtx.Int(ManagementAPIPort)),
				Handler:           mgmtRouter,
				ReadHeaderTimeout: time.Second * 1,
			}

			wsRouter := mux.NewRouter()
			wsRouter.HandleFunc(fmt.Sprintf("/%s", websocketPath), hub.ServeRequest)
			wsServer := http.Server{
				Addr:              fmt.Sprintf(":%d", cliCtx.Int(WebsocketAPIPort)),
				Handler:           wsRouter,
				ReadHeaderTimeout: time.Second * 1,
			}

			serverErr := make(chan error, 1)
			go func() {
				zap.L().Info("starting manamagent server", zap.Int("port", cliCtx.Int(ManagementAPIPort)))
				err := mgmtServer.ListenAndServe()
				if !errors.Is(err, http.ErrServerClosed) {
					zap.L().Error("failed to serve management API", zap.Error(err))
					serverErr <- err
				}
			}()

			go func() {
				zap.L().Info("starting websocket server", zap.Int("port", cliCtx.Int(WebsocketAPIPort)))
				err := wsServer.ListenAndServe()
				if !errors.Is(err, http.ErrServerClosed) {
					zap.L().Error("failed to serve websockets", zap.Error(err))
					serverErr <- err
				}
			}()

			defer func() {
				_ = wsServer.Shutdown(context.Background())
				_ = mgmtServer.Shutdown(context.Background())
			}()

			select {
			case err := <-serverErr:
				return err
			case <-ctx.Done():
				zap.L().Info("shutting down servers")
				return nil
			}
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
