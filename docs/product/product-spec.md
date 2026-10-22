# Tensora: product definition

> **"Tensora" is a working name.** Several companies already use it, so the final name is
> pending a trademark search (see the roadmap's open decisions).

This is the north star for what we're building and for whom. Engineering docs say *how*; this
document says *what* and *why*. Every roadmap item should trace back to something here.

## In one sentence

**Tensora is a global GPU cloud for AI teams.** Rent GPUs by the second, run notebooks, or serve
open models through an OpenAI-compatible API in the region you choose, with your data kept in that
region. It's simple enough for a student and solid enough for an enterprise.

## The bet

- **Customers increasingly need to choose where compute and data live.** Privacy law (GDPR, DPDP, and others) and national "sovereign AI" programs push AI workloads into specific countries. The big GPU clouds concentrate capacity in a few US and EU locations. Regions where good, developer-friendly GPU clouds are thin are our wedge.
- **Developer experience wins users that price can't.** That means launching in minutes, clear pricing, good docs, and an API and CLI for everything.
- **Inference is the growth market.** Most companies will *use* models more than they train them. Raw GPUs and a model API in one place cover the whole journey: experiment in a notebook, fine-tune on a VM, serve on an endpoint.
- **Sell worldwide from day one, build regions by demand.** Selling globally costs only software (global payments, tax, and compliance). Each region costs hardware. The first region is **India**: home market, cost and presence advantage, and local payment methods like UPI. Customers anywhere can sign up and use it. New regions open where demand proves out.

## Regions

A **region** is one location (a city and datacenter) with its own GPU clusters, storage, and
endpoint traffic. Customers pick a region per workload, and their data stays there
([ADR-0009](../adr/0009-regions-global-control-plane-regional-cells.md)).

| Region | Why | When |
|---|---|---|
| **India** (Mumbai or another metro with Tier III colocation) | Home market; cost and presence advantage; in-country data for Indian customers | First (M1–M3) |
| **Second region**, chosen from demand data: EU (GDPR and sovereignty demand), US (largest market), or the Middle East or Southeast Asia (sovereign-AI programs) | Wherever paying signups and latency complaints come from | M4 |
| More, including **dedicated sovereign regions** for a government or large enterprise | Contracted capacity in a specific country | M5+ |

**A region is a repeatable kit.** It's the same `infra/` and `deploy/` stack, plus a colocation contract, hardware,
IP transit, and local legal and tax registration where required. The fastest way to add regions
without capital is a **partner region**: a datacenter partner hosts the GPUs, and Tensora supplies the software and the customers.

## Customers, in stages

| Stage | Who | What they need most | Starts |
|---|---|---|---|
| **1. Self-serve** | AI startups, developers, researchers, and students, worldwide | Instant signup, cheap per-second GPUs, notebooks, working templates, card payment (plus UPI in India) | Private beta (M2) |
| **2. Growing companies** | Teams with production apps | Organizations and roles, API keys, tax-compliant invoices, reserved capacity, a region near their users | Public launch (M3) |
| **3. Enterprise and government** | Regulated industries, the public sector, national AI programs | In-country data residency, SSO, audit logs, dedicated or private clusters, contracts, SOC 2 and ISO 27001 | v2 (M5) |

We build stage 1 first, but never in a way that blocks stage 3. Isolation, audit trails, orgs,
and regions are designed in from the start.

## What we sell

### Tensora Compute

| Product | What the customer gets | Billing |
|---|---|---|
| **GPU VMs** | A full Ubuntu VM with whole GPUs, root access over SSH, and a persistent disk. Stop it and you keep the disk. | Per second while running, plus disk GB-month |
| **Notebooks** | JupyterLab in the browser with a GPU, ready-made templates (PyTorch, fine-tuning, diffusion), and a persistent workspace | Per second while running, plus disk GB-month |
| **CPU instances** | CPU-only VMs and notebooks: cheap for data prep and small-model inference, and the opt-in fallback when GPUs are sold out ([ADR-0010](../adr/0010-fallbacks-cpu-placement-and-inference-failover.md)) | Per second, far below GPU prices |

### Tensora Inference

| Product | What the customer gets | Billing | When |
|---|---|---|---|
| **Dedicated endpoints** | Their chosen open model (safetensors) on their own GPUs in their chosen region, behind an OpenAI-compatible API with keys, logs, and autoscaling | Per GPU-hour | v1 |
| **Serverless model API** | Curated popular open models (Llama, Qwen, DeepSeek, Mistral and similar), shared and always on, in every region | Per million tokens | v1.5 |

The serverless API is the highest-margin product. We run the model code ourselves, so the
tenant sends prompts, not code. Isolation is simpler, but it needs token metering and rate limiting.

### Later

S3-compatible storage buckets, fine-tuning jobs, batch jobs, multi-node training clusters, and
**Tensora Private Cloud** (this platform licensed to companies that own their GPUs; see below).

## Feature map

| Area | M2 private beta | M3 public launch (v1) | M4 growth (v1.5) | M5 enterprise (v2) |
|---|---|---|---|---|
| **Regions** | One region (India) | Region chosen in API, dashboard, and CLI, even with one region; status per region | **Second region**; per-region pricing and capacity | Dedicated or sovereign regions, partner regions |
| **Fallbacks** | Queue only | Opt-in placement fallback (another GPU class, or CPU); retry on another endpoint replica; `Idempotency-Key` on the API | CPU and serverless fallback for endpoints, fallback models, automatic return to GPU | Opt-in cross-region failover, guaranteed failover capacity |
| **Accounts** | Email and Google signup, one user per org | Orgs with members and roles, MFA, API keys, **sanctions screening** | Projects within orgs, scoped keys | SSO (SAML/OIDC), SCIM, audit log export |
| **Compute** | VMs with SSH, notebooks, stop/start, persistent disks | Templates, HTTPS ports for web apps, disk resize | Custom container images (Kata), snapshots, more GPU types | Dedicated nodes, private networking (VPC/WireGuard) |
| **Inference** | Dedicated endpoints from Hugging Face, API keys | Autoscaling, request logs and metrics, model upload from S3 | **Serverless model API** (per token), fine-tuned adapters (LoRA) | Private model registry, guaranteed capacity |
| **Billing** | Free credits, usage page, manual invoicing | All prices in **USD**; prepaid credits by card worldwide (Stripe) and UPI in India (Razorpay, converted at checkout); tax-compliant invoices; spend limits | Reserved instances, team budgets, more local payment methods | Postpaid contracts, PO-based invoicing |
| **Developer tools** | REST API and dashboard | `tensora` CLI, Python SDK, public docs site | Terraform provider, webhooks | — |
| **Trust** | Status page, ToS, AUP, privacy policy (GDPR and DPDP) | 99.5% monthly SLA, security page, incident comms | 99.9% SLA on endpoints | SOC 2 Type II, ISO 27001, DPAs |
| **Support** | Email and community chat | Ticketing with response targets (follow-the-sun later) | Paid support tier | Named contact, enterprise SLAs |

## Quality bar

| Property | Target |
|---|---|
| Time to launch | Notebook or endpoint in 2 minutes or less after capacity is admitted; VM in 3 minutes or less |
| Platform availability (global API and dashboard) | 99.5% at v1, 99.9% at v1.5 |
| **Region independence** | An outage in one region never affects workloads in another. A global control-plane outage never stops running workloads. |
| **Data residency** | Workload data (disks, models, endpoint traffic) never leaves its region unless the customer moves it |
| **No dead ends** | When a customer has opted in to fallbacks, a sold-out GPU class or a failed replica never produces an error while an accepted substitute exists. Every response says what served it ([ADR-0010](../adr/0010-fallbacks-cpu-placement-and-inference-failover.md)). |
| Isolation | No shared kernel between tenants, ever (ADR-0004); no cross-tenant data path |
| Billing accuracy | Per second; usage visible within the hour; zero double-billing (an idempotent pipeline) |
| Durability | Platform data has off-site backups, with restores tested monthly. Customer disks are replicated from M3 (at least 3 nodes per region). |
| Security response | Critical vulnerabilities patched or mitigated within 72 hours |

## Why customers pick us

| Versus | They win on | We win on |
|---|---|---|
| Hyperscalers (AWS, Azure, GCP, Oracle) | Breadth, a presence in every major market, enterprise trust | Price, simplicity, per-second GPUs, an AI-focused product |
| Large GPU clouds (CoreWeave, Lambda, Crusoe, Nebius and others) | Huge capacity, training clusters | Self-serve simplicity for small teams; regions they don't serve |
| Developer GPU clouds (RunPod, Vast.ai, Paperspace and others) | Community, price, many locations | Data residency, VM-grade isolation by default, an integrated inference API, compliance |
| Inference APIs (Together, Fireworks and others) | Model breadth, raw speed | Raw GPUs and an API in one place; private dedicated endpoints in-region |
| Regional providers (for example E2E Networks and Yotta in India) | Local capacity, government ties | Developer experience; reach across several regions |

We won't win on price or scale at the start, because bigger players buy GPUs cheaper. We'll win on
**region, experience, and isolation**, and grow capacity with revenue. Many governments run
**sovereign-AI programs** that buy or subsidize compute from local providers (India's IndiaAI Mission
is one example). In each region, that's a possible stage 3 channel.

## Business model

Revenue comes from GPU-hours, inference tokens, storage, and later support plans and software licences.
**All prices are in USD**, set per region (each region has its own cost base). Where a payment method
needs local currency (UPI in India, for example), the USD amount is converted at checkout.

```
cost per GPU-hour (at 100% use) = (hardware amortization + colocation & power + bandwidth) per month
                                   ÷ (GPUs × 720 hours)
break-even utilization          = cost per GPU-hour ÷ price per GPU-hour
```

Here's a worked example. **The inputs are made up to show the formula, not quotes.**
A 4× L40S server costing $70,000, amortized over 36 months, is ≈ $1,944 a month. Add colocation,
power, and bandwidth at ≈ $600 a month, for ≈ $2,544 a month, or ≈ $636 per GPU per month,
which is ≈ $0.88 per GPU-hour at full use. Priced at $1.80 an hour, break-even is **≈ 49% utilization**, and at
70% the gross margin is about 30% before people and software costs.

**Utilization is the business.** Every product decision should be judged by whether it fills idle GPUs.
Worldwide customers help here: one region's night is another market's working day.

## Legal and compliance for a worldwide service

| Topic | What it means for us | Needed by |
|---|---|---|
| Company and contracts | Which entity signs contracts and invoices; some countries need a local entity or tax registration before you sell or operate a region there | M3 |
| Sales taxes | GST (India), VAT (EU and UK, with reverse charge for B2B), US sales tax (varies by state). Use a tax engine (for example Stripe Tax) and an accountant. | M3 |
| Privacy | GDPR (EU and UK), DPDP (India), CCPA (California), and others: privacy policy, DPAs, data-subject requests, and knowing where account data is stored and transferred | M2–M3 |
| **Export controls and sanctions** | Advanced GPUs are subject to export controls (US rules especially), and serving sanctioned countries or persons is prohibited. Screen customers, block restricted jurisdictions, and **get legal advice before taking payments.** | M3 |
| Abuse and content | AUP enforcement, abuse reports, lawful-request handling per country | M2 |
| Certifications | SOC 2 Type II (expected by US buyers), ISO 27001 (EU, India, and others) | M5 |

## Running this solo with AI agents

What's realistic: one person with AI agents **can** build the software. This repo is set up for
exactly that, with AGENTS.md rules, reviewer subagents, and tests as guardrails. The hard parts aren't code:

| Constraint | Mitigation |
|---|---|
| **Capital** (GPUs are expensive, and each region needs hardware) | One region first; grow from revenue; partner regions; the capital-light options below |
| **24/7 operations across time zones** | Colocation with remote hands, aggressive alerting and automation, maintenance windows, an honest beta SLA, a small stack. Follow-the-sun support comes with hires. |
| **Trust** (why would a company rely on a solo founder?) | A public status page, clear security docs, design partners first, transparency about scale |
| **Sales and legal across countries** | Self-serve first. Enterprise sales waits until there's a team. Use external counsel for export, tax, and privacy. |

Capital-light ways to start (they aren't exclusive):

1. **Private beta on one server** in the first region. Prove demand with 10–20 design partners from anywhere.
2. **Partner regions.** Datacenters that already own GPUs host them; Tensora supplies the software and customers for a revenue share. This is also the fastest way to add countries.
3. **Tensora Private Cloud.** License this platform to companies and institutions anywhere that own GPUs but have no cloud layer. You need no GPU capex.

**First hire:** an infrastructure or SRE engineer, once there are paying customers on more than one
server. **Second hire:** support and developer relations, ideally in a different time zone.

## Milestones

| Milestone | Exit criteria | Maps to |
|---|---|---|
| **M0 Foundation** ✓ | Repo, architecture, core platform on a fake cluster, ML validator, dashboard skeleton | Phase 0 |
| **M1 Lab** | One real server in the first region; a VM, a notebook, and an endpoint all work end to end through the API; regions in the data model; burn-in done; backups off-node | Phase 1 + kube backend from Phase 3 |
| **M2 Private beta** | 10–20 invited users (from any country) using all three products; signup, orgs, API keys, access to every product, usage page, status page, docs | Phase 3 core |
| **M3 Public launch (v1)** | Anyone eligible worldwide can sign up and pay in USD (cards worldwide, UPI in India), with tax-compliant invoices, sanctions screening, spend limits, CLI and SDK, legal pages, 99.5% SLA. The control plane runs on separate small CPU servers. At least 2 GPU servers. | Phases 2 and 4 core |
| **M4 Growth (v1.5)** | **Second region live**; serverless model API; custom images; snapshots; reserved instances | Phase 5 |
| **M5 Enterprise (v2)** | SSO and audit, dedicated or sovereign regions, contracts, SOC 2 or ISO 27001 underway, private-cloud edition | Phase 6 |

## Not doing (until a milestone says so)

Windows VMs, workloads that span regions or move between them while running, spot or preemptible
pricing, a peer-to-peer GPU marketplace, bring-your-own hardware, a crypto-mining allowance (banned by
the AUP), serving sanctioned jurisdictions, training fabrics (InfiniBand) before there's demand, and
customer access to the Kubernetes API.

## Decision log

| Date | Decision |
|---|---|
| 2026-10-23 | Customers: all segments, **staged**, starting with self-serve, then growing companies, then enterprise |
| 2026-10-23 | Flagship at launch: **GPU VMs + notebooks + inference** together |
| 2026-10-23 | Hardware scale: **undecided**; plan for 1–2 servers and design to grow |
| 2026-10-23 | Team: **solo founder with AI agents**; first hire is infra/SRE |
| 2026-10-23 | Market: **worldwide**. Sell globally from launch; **India is the first region**; regions are first-class (ADR-0009). |
| 2026-10-23 | **Fallbacks** are part of the design: opt-in CPU or other-class placement when GPUs are unavailable, and per-request inference failover (ADR-0010) |
| 2026-10-23 | **Name:** "Tensora" is a working name only; choose and clear a distinctive name before the first deployment |
| 2026-10-23 | **Pricing is in USD** everywhere. Local-currency payment methods are converted at checkout. |
