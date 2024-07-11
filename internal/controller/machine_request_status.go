// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package controller provides the controller for the machine request status.
package controller

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/cosi-project/runtime/pkg/controller"
	"github.com/cosi-project/runtime/pkg/controller/generic/qtransform"
	"github.com/cosi-project/runtime/pkg/resource"
	"github.com/cosi-project/runtime/pkg/safe"
	"github.com/cosi-project/runtime/pkg/state"
	"github.com/siderolabs/gen/optional"
	cloudspecs "github.com/siderolabs/omni/client/api/omni/specs/cloud"
	"github.com/siderolabs/omni/client/pkg/omni/resources/cloud"
	"go.uber.org/zap"

	"github.com/siderolabs/omni-cloud-provider-qemu/internal/meta"
	"github.com/siderolabs/omni-cloud-provider-qemu/internal/provisioner"
	"github.com/siderolabs/omni-cloud-provider-qemu/internal/resources"
)

const (
	labelCloudProviderID = "omni.sidero.dev/cloud-provider-id"
	requeueInterval      = 30 * time.Second
)

var errNoMachineAvailable = errors.New("no machine available")

// MachineRequestStatusController creates the system config patch that contains the maintenance config.
type MachineRequestStatusController = qtransform.QController[*cloud.MachineRequest, *cloud.MachineRequestStatus]

// NewMachineRequestStatusController initializes MachineRequestStatusController.
func NewMachineRequestStatusController(provisioner *provisioner.Provisioner) *MachineRequestStatusController {
	return qtransform.NewQController(
		qtransform.Settings[*cloud.MachineRequest, *cloud.MachineRequestStatus]{
			Name: "MachineRequestStatusController",
			MapMetadataOptionalFunc: func(request *cloud.MachineRequest) optional.Optional[*cloud.MachineRequestStatus] {
				providerID, ok := request.Metadata().Labels().Get(labelCloudProviderID)

				if ok && providerID == meta.ProviderID {
					return optional.Some(cloud.NewMachineRequestStatus(request.Metadata().ID()))
				}

				return optional.None[*cloud.MachineRequestStatus]()
			},
			UnmapMetadataFunc: func(status *cloud.MachineRequestStatus) *cloud.MachineRequest {
				return cloud.NewMachineRequest(status.Metadata().ID())
			},
			TransformExtraOutputFunc: func(ctx context.Context, r controller.ReaderWriter, logger *zap.Logger, request *cloud.MachineRequest, status *cloud.MachineRequestStatus) error {
				status.Metadata().Labels().Set(labelCloudProviderID, meta.ProviderID)

				schematicID := request.TypedSpec().Value.SchematicId
				talosVersion := request.TypedSpec().Value.TalosVersion

				logger.Info("received machine request", zap.String("schematic_id", schematicID), zap.String("talos_version", talosVersion))

				machineUUID, isAllocated, err := findMachineUUID(ctx, r, request.Metadata().ID(), logger)
				if err != nil {
					if errors.Is(err, errNoMachineAvailable) {
						logger.Info("no machine available yet, requeue")

						status.TypedSpec().Value.Stage = cloudspecs.MachineRequestStatusSpec_PROVISIONING

						return controller.NewRequeueInterval(requeueInterval)
					}

					return err
				}

				if isAllocated {
					status.TypedSpec().Value.Id = machineUUID
					status.TypedSpec().Value.Stage = cloudspecs.MachineRequestStatusSpec_PROVISIONED

					return nil
				}

				logger.Info("picked available qemu machine", zap.String("qemu_machine_id", machineUUID))

				// allocate the machine
				allocation := resources.NewQemuMachineAllocation(request.Metadata().ID())

				allocation.Metadata().Labels().Set(resources.MachineUUIDLabel, machineUUID)

				allocation.TypedSpec().Value.TalosVersion = talosVersion
				allocation.TypedSpec().Value.SchematicId = schematicID

				if err = r.Create(ctx, allocation); err != nil {
					return fmt.Errorf("failed to create a QemuMachineAllocation: %w", err)
				}

				status.TypedSpec().Value.Id = machineUUID
				status.TypedSpec().Value.Stage = cloudspecs.MachineRequestStatusSpec_PROVISIONED

				return nil
			},
			FinalizerRemovalExtraOutputFunc: func(ctx context.Context, r controller.ReaderWriter, logger *zap.Logger, request *cloud.MachineRequest) error {
				allocation, err := r.Get(ctx, resources.NewQemuMachineAllocation(request.Metadata().ID()).Metadata())
				if err != nil {
					if state.IsNotFoundError(err) {
						return nil
					}

					return err
				}

				destroyReady, err := r.Teardown(ctx, allocation.Metadata())
				if err != nil {
					return err
				}

				if !destroyReady {
					return controller.NewRequeueErrorf(requeueInterval, "allocation is not yet ready to be destroyed")
				}

				if err = r.Destroy(ctx, allocation.Metadata()); err != nil {
					return err
				}

				machineUUID, ok := allocation.Metadata().Labels().Get(resources.MachineUUIDLabel)
				if !ok {
					logger.Warn("missing machine UUID label in the QemuMachineAllocation", zap.String("allocation_id", request.Metadata().ID()))

					return nil
				}

				if err = provisioner.ResetMachine(ctx, machineUUID); err != nil {
					return fmt.Errorf("failed to reset the machine: %w", err)
				}

				return nil
			},
		},
		qtransform.WithExtraOutputs(
			controller.Output{
				Type: resources.QemuMachineAllocationType,
				Kind: controller.OutputExclusive,
			},
		),
		qtransform.WithExtraMappedInput(
			qtransform.MapperNone[*resources.QemuMachine](),
		),
	)
}

// findMachineUUID tries to find the UUID of the QEMU machine for the given MachineRequest id.
//
// If the machine is already allocated, the function returns the UUID of the already assigned machine and isAllocated=true.
//
// If the machine is not allocated, the function returns the UUID of an available (unassigned) machine and isAllocated=false.
//
// If there is no available machine, the function returns errNoMachineAvailable.
func findMachineUUID(ctx context.Context, r controller.Reader, requestID resource.ID, logger *zap.Logger) (machineUUID string, isAllocated bool, err error) {
	qemuMachineList, err := safe.ReaderListAll[*resources.QemuMachine](ctx, r)
	if err != nil {
		return "", false, err
	}

	machineUUIDToRequestID := make(map[string]resource.ID, qemuMachineList.Len())

	allocationList, err := safe.ReaderListAll[*resources.QemuMachineAllocation](ctx, r)
	if err != nil {
		return "", false, err
	}

	for iter := allocationList.Iterator(); iter.Next(); {
		allocation := iter.Value()

		uuid, ok := allocation.Metadata().Labels().Get(resources.MachineUUIDLabel)
		if !ok {
			logger.Warn("missing machine UUID label in the QemuMachineAllocation", zap.String("allocation_id", allocation.Metadata().ID()))

			continue
		}

		machineUUIDToRequestID[uuid] = allocation.Metadata().ID()
	}

	for iter := qemuMachineList.Iterator(); iter.Next(); {
		qemuMachine := iter.Value()

		if reqID, ok := machineUUIDToRequestID[qemuMachine.Metadata().ID()]; ok {
			if reqID == requestID { // match, return
				return qemuMachine.Metadata().ID(), true, nil
			}

			// machine is allocated to another request, skip
			continue
		}

		// machine is available, return
		return qemuMachine.Metadata().ID(), false, nil
	}

	return "", false, errNoMachineAvailable
}
