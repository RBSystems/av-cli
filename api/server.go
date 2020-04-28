package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/byuoitav/auth/wso2"
	avcli "github.com/byuoitav/av-cli"
	"github.com/spf13/pflag"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

var (
	errMissingMetadata = status.Errorf(codes.InvalidArgument, "missing metadata")
	errMissingToken    = status.Errorf(codes.Unauthenticated, "missing token")
	errMissingUser     = status.Errorf(codes.Unauthenticated, "missing user")
	errNotAuthorized   = status.Errorf(codes.Unauthenticated, "you are not authorized to do that")
)

func main() {
	var (
		port        int
		logLevel    int8
		authAddr    string
		authToken   string
		disableAuth bool

		gatewayAddr  string
		clientID     string
		clientSecret string
	)

	pflag.IntVarP(&port, "port", "P", 8080, "port to run lazarette on")
	pflag.Int8VarP(&logLevel, "log-level", "L", 0, "level to log at. refer to https://godoc.org/go.uber.org/zap/zapcore#Level for options")
	pflag.StringVar(&authAddr, "auth-addr", "", "address of the auth server")
	pflag.StringVar(&authToken, "auth-token", "", "authorization token to use when calling the auth server")
	pflag.BoolVar(&disableAuth, "disable-auth", false, "disables auth checks")
	pflag.StringVar(&gatewayAddr, "gateway-addr", "api.byu.edu", "wso2 gateway address")
	pflag.StringVar(&clientID, "client-id", "", "wso2 key")
	pflag.StringVar(&clientSecret, "client-secret", "", "wso2 secret")
	pflag.Parse()

	// build the logger
	config := zap.Config{
		Level:       zap.NewAtomicLevelAt(zapcore.Level(logLevel)),
		Development: false,
		Sampling: &zap.SamplingConfig{
			Initial:    100,
			Thereafter: 100,
		},
		Encoding: "json",
		EncoderConfig: zapcore.EncoderConfig{
			TimeKey:        "@",
			LevelKey:       "level",
			NameKey:        "logger",
			CallerKey:      "caller",
			MessageKey:     "msg",
			StacktraceKey:  "stacktrace",
			LineEnding:     zapcore.DefaultLineEnding,
			EncodeLevel:    zapcore.LowercaseLevelEncoder,
			EncodeTime:     zapcore.ISO8601TimeEncoder,
			EncodeDuration: zapcore.StringDurationEncoder,
			EncodeCaller:   zapcore.ShortCallerEncoder,
		},
		OutputPaths:      []string{"stderr"},
		ErrorOutputPaths: []string{"stderr"},
	}

	lPlain, err := config.Build()
	if err != nil {
		fmt.Printf("failed to build zap logger: %s\n", err)
		os.Exit(1)
	}
	defer func() {
		_ = lPlain.Sync()
	}()

	log := lPlain.Sugar()

	// build opa client
	authClient := &authClient{
		Address:  authAddr,
		Token:    authToken,
		Disabled: disableAuth,
		Logger:   log,
	}

	if !authClient.Disabled && len(authClient.Address) == 0 {
		log.Fatalf("auth is enabled, but opa URL is not set")
	}

	// build the grpc server
	cli := &avcli.Server{
		Logger:     log,
		DBUsername: os.Getenv("DB_USERNAME"),
		DBPassword: os.Getenv("DB_PASSWORD"),
		DBAddress:  os.Getenv("DB_ADDRESS"),
		Client: &wso2.Client{
			GatewayURL:   fmt.Sprintf("https://%s", gatewayAddr),
			ClientID:     clientID,
			ClientSecret: clientSecret,
		},
	}

	server := grpc.NewServer(grpc.UnaryInterceptor(authClient.unaryServerInterceptor()))
	avcli.RegisterAvCliServer(server, cli)

	// bind to a port
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Fatalf("failed to bind listener: %s", err)
	}

	// start the server
	log.Infof("Starting server on %s", lis.Addr().String())
	if err := server.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %s", err)
	}
}

type authClient struct {
	Address  string
	Token    string
	Disabled bool
	Logger   *zap.SugaredLogger
}

type authRequest struct {
	Input struct {
		Token  string `json:"token"`
		User   string `json:"user"`
		Method string `json:"method"`
	} `json:"input"`
}

type authResponse struct {
	DecisionID string `json:"decision_id"`
	Result     struct {
		Allow bool `json:"allow"`
	} `json:"result"`
}

// TODO logging
func (client *authClient) unaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if client.Disabled {
			return handler(ctx, req)
		}

		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, errMissingMetadata
		}

		fmt.Printf("md: %s\n", md)

		auth := md["authorization"]
		user := md["x-user"]

		if len(auth) == 0 {
			return nil, errMissingToken
		}

		if len(user) == 0 {
			return nil, errMissingUser
		}

		// build opa request
		var authReq authRequest
		authReq.Input.Token = strings.TrimPrefix(auth[0], "Bearer ")
		authReq.Input.User = user[0]
		authReq.Input.Method = info.FullMethod

		reqBody, err := json.Marshal(authReq)
		if err != nil {
			return nil, fmt.Errorf("unable to marshal request body: %w", err)
		}

		fmt.Printf("sending this request: %s\n", reqBody)
		url := fmt.Sprintf("https://%s/v1/data/cli", client.Address)

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
		if err != nil {
			return nil, fmt.Errorf("unable to build request: %w", err)
		}

		httpReq.Header.Add("authorization", client.Token)

		resp, err := http.DefaultClient.Do(httpReq)
		if err != nil {
			return nil, fmt.Errorf("unable to do request: %w", err)
		}
		defer resp.Body.Close()

		respBody, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("unable to read response: %w", err)
		}

		fmt.Printf("response from opa: %s\n", respBody)

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("got a %v from OPA. response:\n%s", resp.StatusCode, respBody)
		}

		var authResp authResponse
		if err := json.Unmarshal(respBody, &authResp); err != nil {
			return nil, fmt.Errorf("unable to unmarshal response: %w", err)
		}

		fmt.Printf("parsed response: %+v\n", authResp)

		if !authResp.Result.Allow {
			return nil, errNotAuthorized
		}

		return handler(ctx, req)
	}
}
