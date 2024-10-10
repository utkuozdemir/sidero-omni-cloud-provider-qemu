package agent

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	grpcrecovery "github.com/grpc-ecosystem/go-grpc-middleware/recovery"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	"github.com/siderolabs/omni-infra-provider-bare-metal/api/agent"
)

// Server is an agent GRPC server.
type Server struct {
	logger        *zap.Logger
	listenAddress string
}

// NewServer creates a new agent GRPC server.
func NewServer(listenAddress string, logger *zap.Logger) *Server {
	return &Server{
		listenAddress: listenAddress,
		logger:        logger,
	}
}

// Run starts the agent server.
func (s *Server) Run(ctx context.Context) error {
	s.logger.Info("starting agent server", zap.String("listen_address", s.listenAddress))

	listener, err := net.Listen("tcp", s.listenAddress)
	if err != nil {
		return fmt.Errorf("failed to listen address %q: %w", s.listenAddress, err)
	}

	recoveryOption := grpcrecovery.WithRecoveryHandler(recoveryHandler(s.logger))

	server := grpc.NewServer(
		grpc.ChainUnaryInterceptor(grpcrecovery.UnaryServerInterceptor(recoveryOption)),
		grpc.ChainStreamInterceptor(grpcrecovery.StreamServerInterceptor(recoveryOption)),
		grpc.Creds(insecure.NewCredentials()),
	)

	agent.RegisterAgentServiceServer(server, &serviceServer{
		logger: s.logger,
	})

	eg, ctx := errgroup.WithContext(ctx)

	eg.Go(func() error {
		serveErr := server.Serve(listener)
		if serveErr == nil || errors.Is(serveErr, context.Canceled) {
			return nil
		}

		return fmt.Errorf("failed to serve: %w", serveErr)
	})

	eg.Go(func() error {
		<-ctx.Done()

		s.logger.Info("stopping agent server")

		stopServer(server)

		return nil
	})

	if err = eg.Wait(); err != nil {
		return fmt.Errorf("failed to wait: %w", err)
	}

	return nil
}

func recoveryHandler(logger *zap.Logger) grpcrecovery.RecoveryHandlerFunc {
	return func(p any) error {
		if logger != nil {
			logger.Error("grpc panic", zap.Any("panic", p), zap.Stack("stack"))
		}

		return status.Errorf(codes.Internal, "%v", p)
	}
}

// stopServer stops the GRPC server with a timeout.
func stopServer(server *grpc.Server) {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	stopped := make(chan struct{})

	go func() {
		server.GracefulStop()

		close(stopped)
	}()

	select {
	case <-shutdownCtx.Done():
	case <-stopped:
	}

	server.Stop()
}
