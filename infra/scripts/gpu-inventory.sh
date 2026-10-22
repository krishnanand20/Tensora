#!/usr/bin/env bash
# Read-only: list NVIDIA GPUs with the PCI vendor:device IDs needed for KubeVirt
# permittedHostDevices (deploy/environments/*.yaml), plus driver state.
set -euo pipefail

echo "== NVIDIA PCI devices (use [10de:xxxx] as pciVendorSelector)"
lspci -nn -d 10de: || true

echo
if command -v nvidia-smi >/dev/null 2>&1; then
  echo "== nvidia-smi (container-mode node)"
  nvidia-smi --query-gpu=index,name,memory.total,driver_version,pci.bus_id --format=csv
else
  echo "nvidia-smi not found on the host. Expected: the GPU Operator runs the driver in a"
  echo "container, and vm-passthrough nodes bind GPUs to vfio-pci instead."
fi

echo
echo "== Power and thermals (needs ipmitool and a BMC)"
if command -v ipmitool >/dev/null 2>&1; then
  ipmitool dcmi power reading 2>/dev/null || echo "DCMI power reading not supported by this BMC"
fi
