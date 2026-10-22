#!/usr/bin/env bash
# Read-only: verify IOMMU is on and show which IOMMU group each NVIDIA device is in.
# Each GPU (plus its own audio function) should sit in a group with nothing else, or
# passthrough will drag unrelated devices along. Run as root on the node.
set -euo pipefail

echo "== Kernel command line"
cat /proc/cmdline
if ! grep -Eq '(intel|amd)_iommu=on|iommu=pt' /proc/cmdline; then
  echo "WARN: IOMMU arguments not found; run the iommu Ansible role and reboot." >&2
fi

echo
echo "== IOMMU groups containing NVIDIA devices"
shopt -s nullglob
groups=(/sys/kernel/iommu_groups/*)
if [[ ${#groups[@]} -eq 0 ]]; then
  echo "FAIL: no IOMMU groups. Enable VT-d / AMD-Vi in BIOS and check the kernel arguments." >&2
  exit 1
fi

for group in "${groups[@]}"; do
  devices=("$group"/devices/*)
  nvidia=false
  for dev in "${devices[@]}"; do
    [[ "$(cat "$dev/vendor")" == "0x10de" ]] && nvidia=true
  done
  $nvidia || continue
  echo "group $(basename "$group"):"
  for dev in "${devices[@]}"; do
    addr=$(basename "$dev")
    driver=$(basename "$(readlink -f "$dev/driver" 2>/dev/null || echo none)")
    echo "  $(lspci -nns "$addr")  [driver: $driver]"
  done
done
