package main

import (
	"log"
	"os"

	"github.com/urfave/cli/v2"

	offline "github.com/geode-io/aws-emulators"
)

const (
	Function = "function"
	Payload  = "payload"
)

func main() {
	flags := []cli.Flag{
		&cli.StringFlag{
			Name:    Payload,
			Aliases: []string{"p"},
			Usage:   "Payload to send to the lambda function",
		},
	}

	flags = append(flags, offline.LambdaFlags()...)
	flags = append(flags, offline.LambdaInvokeFlags("")...)

	app := &cli.App{
		Name:  "invoke",
		Usage: "Invoke a lambda function via http",
		Flags: flags,
		Action: func(ctx *cli.Context) error {
			payload := ctx.String(Payload)
			if payload == "" {
				payload = "{}"
			}

			result, err := offline.LambdaInvokeFromCLI(ctx, Function, []byte(payload))
			if err != nil {
				return cli.Exit("failed to invoke lambda", 1)
			}

			return cli.Exit(string(result), 0)
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
