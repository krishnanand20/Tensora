# Bill of materials

Prices change weekly and vary by vendor, so there are none here. Record the quotes you get in the table at the bottom.

## Node tiers

| Tier | GPUs | Use | Sellable? |
|---|---|---|---|
| **Lab box** | 2× RTX 4090 or 5090 in a workstation | Development, internal experiments | **Internal only** (GeForce licence; see gpu-sharing.md) |
| **Starter node** (recommended for Phase 1) | 4× **L40S 48GB** PCIe in a 2U/4U server | Inference endpoints, notebooks, fine-tuning, GPU VMs | Yes |
| **Large-model node** | 4–8× A100 80GB or H100 PCIe | Bigger models, more memory per GPU | Yes |
| **Training node** | HGX 8× H100/H200 SXM with NVLink | Multi-GPU training | Yes, but needs a training fabric and about 10 kW per node |

L40S is the pragmatic start: 48 GB per card, strong inference performance, PCIe (no HGX baseboard),
and supported by passthrough and the GPU Operator. It has no MIG.
If small fractional GPUs matter to customers, choose A100 or H100, which do have MIG.

## Starter node: what to specify

| Component | Spec | Why |
|---|---|---|
| Chassis | 2U or 4U GPU server validated for 4× double-width passive GPUs | Passive datacenter cards need server airflow |
| CPU | 1–2× AMD EPYC (9004/9005) or Xeon Scalable with ≥ 64 PCIe 5.0 lanes free for GPUs and NICs | x16 lanes per GPU; IOMMU group separation |
| RAM | ≥ 2× total VRAM: **384 GB minimum** for 4× 48 GB; 512 GB–1 TB preferred | Model loading, VM memory (64 GiB per GPU in the catalog), page cache |
| OS disks | 2× 480–960 GB NVMe or SATA SSD, RAID 1 | Survives a boot disk failure |
| Data disks | 2–4× 3.84–7.68 TB NVMe (U.2 or U.3, PLP) | Longhorn or Ceph, model cache, VM disks |
| Network | 2× 25 GbE (SFP28) minimum; 100 GbE if you'll run Ceph | Model pulls and storage replication |
| Management | Dedicated BMC/IPMI port (Redfish) | Remote power, console, sensors |
| Power supplies | Redundant 2+2 or 1+1, Titanium or Platinum, sized for the GPU TDP sum × 1.3 | GPU power spikes; one PSU can fail |
| BIOS | VT-d/AMD-Vi, SR-IOV, Above-4G decoding, ACS (if offered) | Needed for passthrough (runbook 01) |

## Shared infrastructure

| Item | Spec |
|---|---|
| Top-of-rack switch | 25/100 GbE with enough ports for 2 per node plus uplinks. Must do VLANs and LACP. |
| Management switch | 1 GbE for BMCs, on a separate VLAN with no internet |
| Router/firewall | Static public IPs (/29 or larger), dual ISP, WireGuard endpoint |
| UPS and power | See [power-and-cooling.md](power-and-cooling.md) |
| Rack | PDUs with per-outlet metering (2× per rack, A and B feeds) |

## Questions for vendors

1. Is this chassis validated (thermally and with BIOS) for 4× L40S, or whatever card you choose?
2. Which PCIe topology results: does each GPU land in its own IOMMU group? Ask for `lspci -tv` from a reference unit.
3. What's the lead time on GPUs, and is there warranty and RMA turnaround **in India**?
4. Is a BMC with Redfish and a remote console licence included?
5. What's the power draw at full GPU load (measured, not nameplate)?

## Quotes

| Date | Vendor | Config | Price (₹, ex-GST) | Lead time | Notes |
|---|---|---|---|---|---|
| | | | | | |
