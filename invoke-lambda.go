package offline

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/hashicorp/go-cleanhttp"
	"github.com/urfave/cli/v2"
)

func LambdaInvoke(ctx context.Context, invokeEndpoint string, payload []byte) ([]byte, error) {
	httpClient := cleanhttp.DefaultClient()

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		invokeEndpoint,
		io.NopCloser(bytes.NewReader(payload)),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to build lambda invocation request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to invoke lambda: %w", err)
	}

	defer resp.Body.Close()

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, resp.Body); err != nil {
		return nil, fmt.Errorf("failed to read lambda response: %w", err)
	}

	return buf.Bytes(), nil
}

const (
	LambdaEndpointName         = "lambda-endpoint"
	InvokeEndpointNamePrefix   = "invoke-endpoint"
	FunctionNamePrefix         = "function"
	EnvVarLambdaEndpoint       = "LAMBDA_ENDPOINT"
	EnvVarFunctionNamePrefix   = "LAMBDA_FUNCTION"
	EnvVarInvokeEndpointPrefix = "LAMBDA_INVOKE_ENDPOINT"
)

func InvokeEndpointNameForFunction(functionName string) string {
	if functionName == "" || functionName == FunctionNamePrefix {
		return InvokeEndpointNamePrefix
	}

	return fmt.Sprintf("%s-%s", InvokeEndpointNamePrefix, functionName)
}

func EnvVarInvokeEndpointForFunction(functionName string) string {
	if functionName == "" || functionName == FunctionNamePrefix {
		return EnvVarInvokeEndpointPrefix
	}

	return fmt.Sprintf("%s_%s", EnvVarInvokeEndpointPrefix, strings.ToUpper(functionName))
}

func FunctionNameForFunction(functionName string) string {
	if functionName == "" || functionName == FunctionNamePrefix {
		return FunctionNamePrefix
	}

	return fmt.Sprintf("%s-%s", FunctionNamePrefix, functionName)
}

func EnvVarFunctionNameForFunction(functionName string) string {
	if functionName == "" || functionName == FunctionNamePrefix {
		return EnvVarFunctionNamePrefix
	}

	return fmt.Sprintf("%s_%s", EnvVarFunctionNamePrefix, strings.ToUpper(functionName))
}

func LambdaInvokeFromCLI(cliCtx *cli.Context, functionName string, payload []byte) ([]byte, error) {
	invokeEndpoint := cliCtx.String(InvokeEndpointNameForFunction(functionName))
	if invokeEndpoint == "" {
		base := cliCtx.String(LambdaEndpointName)
		base = strings.TrimSuffix(base, "/")
		funcName := cliCtx.String(FunctionNameForFunction(functionName))
		funcName = strings.TrimSuffix(strings.TrimPrefix(funcName, "/"), "/")
		invokeEndpoint = fmt.Sprintf("%s/2015-03-31/functions/%s/invocations", base, funcName)
	}

	return LambdaInvoke(cliCtx.Context, invokeEndpoint, payload)
}

func LambdaFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:    LambdaEndpointName,
			EnvVars: []string{EnvVarLambdaEndpoint},
			Usage:   "Endpoint to invoke lambda functions. i.e. http://localhost:8080",
		},
	}
}

func LambdaInvokeFlags(functionName string) []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:    InvokeEndpointNameForFunction(functionName),
			EnvVars: []string{EnvVarInvokeEndpointForFunction(functionName)},
			Usage: "Endpoint to invoke the lambda function. " +
				"i.e. http://localhost:8080/2015-03-31/functions/function/invocations",
		},
		&cli.StringFlag{
			Name:    FunctionNameForFunction(functionName),
			EnvVars: []string{EnvVarFunctionNameForFunction(functionName)},
			Value:   "function",
			Usage:   "Name of the lambda function to invoke",
		},
	}
}
