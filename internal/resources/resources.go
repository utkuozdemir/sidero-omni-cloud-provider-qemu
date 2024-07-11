// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package resources contains resource definitions of the omni-cloud-provider-qemu.
package resources

import (
	"github.com/siderolabs/omni/client/pkg/omni/resources"

	"github.com/siderolabs/omni-cloud-provider-qemu/internal/meta"
)

// MachineUUIDLabel is the label key for the machine UUID.
const MachineUUIDLabel = "machine-uuid"

// namespace is the namespace of the resources specific/internal to this cloud provider.
//
// It has the format `cloud-provider:<ID_OF_THE_PROVIDER>`, by default, `cloud-provider:qemu` for this provider.
var namespace = resources.CloudProviderSpecificNamespacePrefix + meta.ProviderID

// resourceTypeSuffix is the suffix of the resource type expected from this cloud provider by Omni.
var resourceTypeSuffix = "." + meta.ProviderID + ".cloudprovider.sidero.dev"
