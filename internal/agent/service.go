package agent

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/siderolabs/omni-infra-provider-bare-metal/api/agent"
	"github.com/siderolabs/omni-infra-provider-bare-metal/internal/agent/bmc"
)

const ipmiUsername = "talos-agent"

type serviceServer struct {
	agent.UnimplementedAgentServiceServer

	logger *zap.Logger
}

func (s *serviceServer) SetIPMICredentials(_ context.Context, req *agent.SetIPMICredentialsRequest) (*agent.SetIPMICredentialsResponse, error) {
	s.logger.Debug("set ipmi credentials", zap.Uint32("ipmi_address", req.Uid))

	password, err := bmc.AttemptBMCUserSetup(ipmiUsername, s.logger)
	if err != nil {
		return nil, fmt.Errorf("error setting ipmi credentials: %w", err)
	}

	_ = password

	return &agent.SetIPMICredentialsResponse{Message: "success"}, nil
}
