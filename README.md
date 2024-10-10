# omni-infra-provider-bare-metal

This repository contains the QEMU cloud provider for the Omni project.

It aims to facilitate the development of Omni by providing a cloud provider working similarly to `talosctl cluster create`.

It also serves as a reference implementation for other cloud providers.

## Prerequisites

To be able to run and test the provider locally, you need to meet the following requirements:

- Running on a Linux machine
- [Omni](https://github.com/siderolabs/omni) running
- Qemu installed (`qemu-user-static`)
- Enough disk space/RAM/CPU to run the machines
- `talosctl` being present in your `$PATH` (or pass its location via `--talosctl-path` flag)

## Usage

Create a cloud provider service account named `qemu` on Omni:

```shell
omnictl serviceaccount create --use-user-role=false --role=CloudProvider cloud-provider:qemu
```

Export the printed environment variables:

```shell
export OMNI_ENDPOINT=...
export OMNI_SERVICE_ACCOUNT_KEY=...
```

Build the project:

```shell
make omni-infra-provider-bare-metal-linux-amd64
```

Run the cloud provider:

```shell
./_out/omni-infra-provider-bare-metal-linux-amd64
```

Now, when Omni creates a `MachineRequest`, you will see that one of the available machines will be provisioned and joined to Omni.
