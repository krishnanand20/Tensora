# 0014. Sharing idle compute: private by default, invite-only, and community pools

- **Status:** Accepted (each machine's sharing level is stored, defaulting to private; enforcement is planned, see the roadmap)
- **Date:** 2026-10-23

## Context

Many computers sit idle most of the day: a Mac Studio overnight, a gaming PC during work hours, a lab's
servers between classes. Their owners want to put that power to use, and students and developers need
free compute. Running a stranger's code on someone's personal computer is risky in both directions.

## Decision

### Sharing levels

Each machine's owner chooses one:

| Level | Who can use the machine | Default |
|---|---|---|
| **Private** | Only the owner, or their organization | ✓ |
| **Invite-only** | Named users, groups, or organizations (a team, a class, friends) | |
| **Community pool** | Anyone with an account on that instance | |

A **pool** is simply a Tensora instance with sharing turned on. Anyone can run one: a university,
a company, a community, or the project itself. Hosts join a pool by enrolling a machine with a token.

### Protecting the host

These are defaults. Owners can make them stricter, but never looser than what the trust matrix allows.

- **Share only when idle:** no keyboard or mouse input for a set time (macOS reads this from IOKit; Linux from logind), on AC power, and optionally within a schedule.
- **Resource caps:** limits on CPU, RAM, GPU memory, disk, and power.
- **Instant reclaim:** when the owner comes back, guest workloads are preempted.
- **No access** to the host's files or local network.
- **No internet access for community workloads by default.** Models arrive through the instance's model store, so a guest can't abuse the owner's home IP address.
- Guests only run through drivers that the trust matrix permits. On a Mac, community users get **managed inference** (the host runs a catalog model server and the guest only sends prompts) or a macOS VM. They never get native code execution.

### Protecting the guest (and being honest)

- A host owner has root on their own machine and can, in principle, see guest data. The dashboard says this plainly before anything is launched on someone else's machine.
- Users shouldn't send secrets or private data to community hosts. Confidential computing is out of scope.

### Fairness and abuse

- Usage accounting (ADR-0011) drives fair-share queues and per-user quotas. No money is involved.
- Abuse controls: verified accounts (email or OAuth), rate limits, reports, bans, and an acceptable use policy per pool.
- **Pool operators are responsible for their pool's rules and for complying with the law where they operate.**

### Preemption and recognition

- Workloads on shared machines can be preempted. The state machine gains a `preempted` state (planned).
- If the user opted in (ADR-0010), a preempted workload is re-queued elsewhere. Community workloads are ephemeral by default.
- Hosts can see the GPU-hours they've contributed. Leaderboards are optional per pool.

## Consequences

- New data per machine: owner, sharing level, schedule, and caps.
- New per-OS work: idle detection, preemption, and enforcing the trust matrix at admission.
- Public pools need real abuse handling, which is moderation work for whoever runs them.
- Hosts pay for their own electricity. Owners should know this before they share (see the hardware power notes).

## Alternatives considered

- **No sharing:** simpler, but it loses the idle-machine use case entirely.
- **Batch-only volunteer computing (BOINC style):** safe, but rules out interactive notebooks and endpoints.
