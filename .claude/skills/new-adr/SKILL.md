---
name: new-adr
description: Record an architecture decision as a new ADR in docs/adr. Use when a change adds or replaces a platform component, or changes an architecture rule in AGENTS.md.
---

Follow [docs/playbooks/write-an-adr.md](../../../docs/playbooks/write-an-adr.md).

Decision to record: $ARGUMENTS

1. Find the next number: `ls docs/adr/` gives the highest `NNNN-` prefix, plus one.
2. Copy `docs/adr/0000-template.md` to `docs/adr/NNNN-short-kebab-title.md`.
3. Fill in context, decision, consequences, and alternatives from the conversation and the code. Keep it under about 60 lines.
   If the context is unclear, ask the user rather than inventing reasons.
4. Set the status to **Proposed** unless the user says it's accepted.
5. Add a row to the index in `docs/adr/README.md`.
6. If the decision changes an architecture rule in `AGENTS.md`, update the rule in the same change
   and link the ADR next to it.
