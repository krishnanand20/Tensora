# Power and cooling

Power is often the real bottleneck, before GPUs or money. In India, plan for grid instability,
high ambient temperatures, and slow utility upgrades.

## Rough draw per node

These are board-power (TDP) figures. Measure your real draw with the BMC under burn-in.

| Config | GPUs | GPU power | Rest of system | Plan for |
|---|---|---|---|---|
| 2× RTX 4090 workstation | 2 × 450 W | 0.9 kW | ~0.4 kW | **1.5 kW** |
| 4× L40S server | 4 × 350 W | 1.4 kW | ~0.8 kW (2 CPUs, RAM, NVMe, fans) | **2.5–3 kW** |
| 8× RTX 4090/5090 server | 8 × 450–575 W | 3.6–4.6 kW | ~1 kW | **5–6 kW** |
| 4× A100 80GB PCIe | 4 × 300 W | 1.2 kW | ~0.8 kW | **2.5 kW** |
| HGX 8× H100 SXM | 8 × 700 W | 5.6 kW | ~3–4 kW (CPUs, NVSwitch, NICs, fans) | **~10.2 kW** max system |

GPUs spike above their TDP for milliseconds, so PSUs and UPSes need headroom.

## UPS sizing

```
UPS kVA ≥ (sum of planned kW) ÷ power factor (≈0.9) × 1.25 headroom
```

For example, one 4× L40S node plus a switch and router comes to about 3.3 kW, which needs a UPS of
about **4.6 kVA** or more. Use an **online double-conversion** UPS, never line-interactive, for GPU servers.

- **Runtime:** batteries only need to cover the time until a generator takes over (5–10 minutes) plus a clean shutdown. Don't plan to ride out long outages on batteries.
- **Generator (DG set):** needed for any service you sell. Size it for UPS input plus cooling. Test it under load monthly.
- **Wiring:** three-phase supply for anything above about 5 kW. Use separate A and B feeds to the dual PSUs.
- **Monitoring:** use the UPS's network card (SNMP) and BMC power readings. Trigger a graceful drain and shutdown at a battery threshold (Phase 2).

## Cooling

- Each kW of IT load is about 3,412 BTU/h of heat. A 3 kW node needs roughly 1 ton of cooling by itself, and more with redundancy.
- Keep server inlet air at or below 27 °C (the ASHRAE recommended range). GPUs throttle and fail early when inlet air is hot, and Indian summer ambient makes this a real risk in offices.
- Passive datacenter GPUs (L40S, A100, H100) depend on server fans and front-to-back airflow, so use blanking panels and hot/cold aisle separation.
- Watch GPU temperature and clock-throttle reasons through DCGM, and alert on sustained throttling.

## Colocation versus on-premises

If you're selling compute, **colocation in a Tier III datacenter** (major Indian metros all have them)
is usually cheaper than building reliable power and cooling yourself. It gets you:

- dual utility feeds, a generator, UPS, and N+1 cooling
- carrier-neutral connectivity with DDoS options
- remote hands for disk and GPU swaps

On-premises makes sense for a lab or internal-only setup.

## Budgeting the electricity

```
monthly kWh ≈ average kW × 24 × 30 × PUE (colocation ~1.5, office ~2+)
```

For example, a node averaging 2.5 kW uses about 2.5 × 720 × 1.5 ≈ 2,700 kWh a month. Multiply by your
₹/kWh, or the colocation's per-kW rate, to find the floor price per GPU-hour before any margin.
