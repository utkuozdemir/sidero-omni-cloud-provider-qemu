// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package resources

import (
	"github.com/cosi-project/runtime/pkg/resource"
	"github.com/cosi-project/runtime/pkg/resource/meta"
	"github.com/cosi-project/runtime/pkg/resource/protobuf"
	"github.com/cosi-project/runtime/pkg/resource/typed"

	"github.com/siderolabs/omni-infra-provider-bare-metal/api/specs"
)

// NewQemuMachineAllocation creates a new QemuMachineAllocation resource.
func NewQemuMachineAllocation(id string) *QemuMachineAllocation {
	return typed.NewResource[QemuMachineAllocationSpec, QemuMachineAllocationExtension](
		resource.NewMetadata(namespace, QemuMachineAllocationType, id, resource.VersionUndefined),
		protobuf.NewResourceSpec(&specs.QemuMachineAllocationSpec{}),
	)
}

// QemuMachineAllocationType is the type of QemuMachineAllocation resource.
var QemuMachineAllocationType = "QemuMachineAllocations" + resourceTypeSuffix

// QemuMachineAllocation resource describes a machine allocation.
type QemuMachineAllocation = typed.Resource[QemuMachineAllocationSpec, QemuMachineAllocationExtension]

// QemuMachineAllocationSpec wraps specs.QemuMachineAllocationSpec.
type QemuMachineAllocationSpec = protobuf.ResourceSpec[specs.QemuMachineAllocationSpec, *specs.QemuMachineAllocationSpec]

// QemuMachineAllocationExtension providers auxiliary methods for QemuMachineAllocation resource.
type QemuMachineAllocationExtension struct{}

// ResourceDefinition implements [typed.Extension] interface.
func (QemuMachineAllocationExtension) ResourceDefinition() meta.ResourceDefinitionSpec {
	return meta.ResourceDefinitionSpec{
		Type:             QemuMachineAllocationType,
		Aliases:          []resource.Type{},
		DefaultNamespace: namespace,
		PrintColumns:     []meta.PrintColumn{},
	}
}
