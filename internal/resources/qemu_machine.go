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

// NewQemuMachine creates a new QemuMachine resource.
func NewQemuMachine(id string) *QemuMachine {
	return typed.NewResource[QemuMachineSpec, QemuMachineExtension](
		resource.NewMetadata(namespace, QemuMachineType, id, resource.VersionUndefined),
		protobuf.NewResourceSpec(&specs.QemuMachineSpec{}),
	)
}

// QemuMachineType is the type of QemuMachine resource.
var QemuMachineType = "QemuMachines" + resourceTypeSuffix

// QemuMachine resource describes a machine allocation.
type QemuMachine = typed.Resource[QemuMachineSpec, QemuMachineExtension]

// QemuMachineSpec wraps specs.QemuMachineSpec.
type QemuMachineSpec = protobuf.ResourceSpec[specs.QemuMachineSpec, *specs.QemuMachineSpec]

// QemuMachineExtension providers auxiliary methods for QemuMachine resource.
type QemuMachineExtension struct{}

// ResourceDefinition implements [typed.Extension] interface.
func (QemuMachineExtension) ResourceDefinition() meta.ResourceDefinitionSpec {
	return meta.ResourceDefinitionSpec{
		Type:             QemuMachineType,
		Aliases:          []resource.Type{},
		DefaultNamespace: namespace,
		PrintColumns: []meta.PrintColumn{
			{
				Name:     "UUID",
				JSONPath: "{.uuid}",
			},
		},
	}
}
