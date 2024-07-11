// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package ipxe provides an iPXE server that serves iPXE scripts to boot machines.
package ipxe

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/cosi-project/runtime/pkg/resource"
	"github.com/cosi-project/runtime/pkg/safe"
	"github.com/cosi-project/runtime/pkg/state"
	"github.com/siderolabs/gen/containers"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/sync/errgroup"

	"github.com/siderolabs/omni-cloud-provider-qemu/internal/resources"
)

var errMachineNotAllocated = errors.New("machine is not allocated")

// Server is an iPXE server serving as a proxy to the image factory, capturing machine information before chaining to the image factory.
type Server struct {
	logger *zap.Logger
	state  state.State

	// unallocatedMachineUUIDSet is used to control log verbosity.
	unallocatedMachineUUIDSet containers.ConcurrentMap[string, struct{}]

	imageFactoryPXEURL string
	port               int
}

// NewServer creates a new iPXE server.
func NewServer(st state.State, imageFactoryPXEURL string, port int, logger *zap.Logger) *Server {
	return &Server{
		logger:             logger,
		state:              st,
		imageFactoryPXEURL: imageFactoryPXEURL,
		port:               port,
	}
}

const ipxeScriptTemplate = `#!ipxe
chain --replace {{ .URL }}/pxe/{{ .SchematicID }}/{{ .TalosVersion }}/metal-amd64
`

// Run starts the iPXE server.
func (server *Server) Run(ctx context.Context) error {
	eg, ctx := errgroup.WithContext(ctx)

	httpServer := &http.Server{
		Addr: net.JoinHostPort("", strconv.Itoa(server.port)),
		Handler: http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			nodeUUID := request.URL.Query().Get("uuid")
			logger := server.logger.With(zap.String("uuid", nodeUUID))
			ipxeRequestLogLevel := zapcore.InfoLevel

			if _, isUnallocated := server.unallocatedMachineUUIDSet.Get(nodeUUID); isUnallocated {
				ipxeRequestLogLevel = zapcore.DebugLevel // reduce log level, as unallocated machines keep hitting iPXE endpoint repeatedly
			}

			logger.Log(ipxeRequestLogLevel, "received iPXE request", zap.String("uuid", nodeUUID))

			allocation, err := server.ensureQemuMachine(ctx, nodeUUID)
			if err != nil {
				if errors.Is(err, errMachineNotAllocated) {
					server.unallocatedMachineUUIDSet.Set(nodeUUID, struct{}{})

					logger.Log(ipxeRequestLogLevel, "machine is not yet allocated to a request")

					writer.WriteHeader(http.StatusNotFound)

					if _, err = writer.Write([]byte("no pending requests, come later")); err != nil {
						logger.Error("failed to write response", zap.Error(err))
					}

					return
				}

				server.handleError(writer, logger, fmt.Errorf("failed to ensure machine: %w", err))

				return
			}

			server.unallocatedMachineUUIDSet.Remove(nodeUUID)

			logger.Info("matched machine to request", zap.String("machine_request_id", allocation.Metadata().ID()))

			tmpl, err := template.New("ipxe-script").Parse(ipxeScriptTemplate)
			if err != nil {
				server.handleError(writer, logger, fmt.Errorf("failed to parse iPXE script template: %w", err))

				return
			}

			talosVersion := allocation.TypedSpec().Value.TalosVersion
			if !strings.HasPrefix(talosVersion, "v") {
				talosVersion = "v" + talosVersion
			}

			var buf bytes.Buffer

			if err = tmpl.Execute(&buf, struct {
				URL          string
				SchematicID  string
				TalosVersion string
			}{
				URL:          server.imageFactoryPXEURL,
				SchematicID:  allocation.TypedSpec().Value.SchematicId,
				TalosVersion: talosVersion,
			}); err != nil {
				server.handleError(writer, logger, fmt.Errorf("failed to execute iPXE script template: %w", err))
			}

			writer.Header().Set("Content-Type", "text/plain")
			writer.WriteHeader(http.StatusOK)

			if _, err = writer.Write(buf.Bytes()); err != nil {
				server.logger.Error("failed to write response", zap.Error(err))
			}
		}),
	}

	eg.Go(func() error {
		<-ctx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := httpServer.Shutdown(shutdownCtx); err != nil { //nolint:contextcheck
			return fmt.Errorf("failed to shutdown iPXE server: %w", err)
		}

		return nil
	})

	eg.Go(func() error {
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("failed to run iPXE server: %w", err)
		}

		return nil
	})

	return eg.Wait()
}

// ensureQemuMachine makes sure that a QemuMachine resource exists for the given machine UUID by creating one if it doesn't exist.
//
// It returns the QemuMachineAllocation resource if the machine is allocated, otherwise it returns errMachineNotAllocated.
func (server *Server) ensureQemuMachine(ctx context.Context, uuid string) (*resources.QemuMachineAllocation, error) {
	res := resources.NewQemuMachine(uuid)
	md := res.Metadata()

	// check if the machine has a matching allocation
	allocationList, err := safe.StateListAll[*resources.QemuMachineAllocation](ctx, server.state, state.WithLabelQuery(resource.LabelEqual(resources.MachineUUIDLabel, uuid)))
	if err != nil && !state.IsNotFoundError(err) {
		return nil, err
	}

	if allocationList.Len() > 0 {
		return allocationList.Get(0), nil
	}

	// create a qemu machine
	existing, err := server.state.Get(ctx, md)
	if err != nil && !state.IsNotFoundError(err) {
		return nil, err
	}

	if existing != nil {
		return nil, errMachineNotAllocated
	}

	if err = server.state.Create(ctx, res); err != nil {
		return nil, fmt.Errorf("failed to create QemuMachine: %w", err)
	}

	return nil, errMachineNotAllocated
}

func (server *Server) handleError(w http.ResponseWriter, logger *zap.Logger, err error) {
	logger.Error("internal server error", zap.Error(err))

	w.WriteHeader(http.StatusInternalServerError)

	if _, err = w.Write([]byte("internal server error")); err != nil {
		logger.Error("failed to write response", zap.Error(err))
	}
}
