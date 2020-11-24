package main

import (
	"context"
	"fmt"
	"net/http"
	"os"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_zap "github.com/grpc-ecosystem/go-grpc-middleware/logging/zap"
	grpc_ctxtags "github.com/grpc-ecosystem/go-grpc-middleware/tags"
	grpc_validator "github.com/grpc-ecosystem/go-grpc-middleware/validator"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/packethost/pkg/log/logr"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	"github.com/tinkerbell/pbnj/pkg/zaplog"
	"github.com/tinkerbell/pbnj/server/grpcsvr"
	"goa.design/goa/grpc/middleware"
	"google.golang.org/grpc"
)

const (
	requestIDKey    = "x-request-id"
	requestIDLogKey = "requestID"
)

var (
	port        string
	metricsAddr string
	// serverCmd represents the server command
	serverCmd = &cobra.Command{
		Use:   "server",
		Short: "Run PBnJ server",
		Long:  `Run PBnJ server for interacting with BMCs.`,
		Run: func(cmd *cobra.Command, args []string) {
			ctx := context.Background()
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()

			logger, zlog, err := logr.NewPacketLogr(
				logr.WithServiceName("github.com/tinkerbell/pbnj"),
				logr.WithLogLevel(logLevel),
			)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(1)
			}
			defer zlog.Sync() // nolint

			// Make sure that log statements internal to gRPC library are logged using the zapLogger as well.
			grpc_zap.ReplaceGrpcLoggerV2(zlog)

			grpcServer := grpc.NewServer(
				grpc_middleware.WithUnaryServerChain(
					grpc_prometheus.UnaryServerInterceptor,
					grpc_ctxtags.UnaryServerInterceptor(grpc_ctxtags.WithFieldExtractor(grpc_ctxtags.CodeGenRequestFieldExtractor)),
					middleware.UnaryRequestID(middleware.UseXRequestIDMetadataOption(true), middleware.XRequestMetadataLimitOption(512)),
					zaplog.UnaryLogRequestID(zlog, requestIDKey, requestIDLogKey),
					grpc_zap.UnaryServerInterceptor(zlog),
					grpc_validator.UnaryServerInterceptor(),
				),
			)

			go func() {
				http.Handle("/metrics", promhttp.Handler())
				err := http.ListenAndServe(metricsAddr, nil)
				if err != nil {
					logger.Error(err, "failed to serve http")
					os.Exit(1)
				}
			}()

			if err := grpcsvr.RunServer(ctx, zaplog.RegisterLogger(logger), grpcServer, port); err != nil {
				logger.Error(err, "error running server")
				os.Exit(1)
			}
		},
	}
)

func init() {
	serverCmd.PersistentFlags().StringVar(&port, "port", "9090", "grpc server port")
	serverCmd.PersistentFlags().StringVar(&metricsAddr, "metrics-listen-addr", ":8080", "metrics server listen address")
	rootCmd.AddCommand(serverCmd)
}