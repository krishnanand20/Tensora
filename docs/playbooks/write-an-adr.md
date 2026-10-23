# Playbook: write an ADR

## When

- You're adding, replacing, or removing a platform component
- You're changing an architecture rule in `AGENTS.md`
- You're making a choice that's expensive to reverse (data model, protocol, vendor, licence)
- People keep asking "why is it like this?" about something

Don't write one for routine changes, library bumps, or anything easy to reverse.

## How

1. Copy `docs/adr/0000-template.md` to `docs/adr/NNNN-short-title.md`, where NNNN is the next number.
2. Write the **context** as facts and constraints, linking to docs and code. Write the **decision** in one paragraph.
   Be honest about the **consequences**, including the bad ones and what would make us revisit.
   List the **alternatives** you actually considered, with one line on why not.
3. Keep it short. One page is plenty.
4. Set the status to **Proposed**. It becomes **Accepted** when the PR merges with reviewer agreement.
5. Add it to the index in `docs/adr/README.md`.
6. If it changes an `AGENTS.md` rule, change the rule in the same PR and reference the ADR.

To reverse a decision, write a new ADR that supersedes the old one. Mark the old one
`Superseded by NNNN` in its status, and put a banner at the top of it saying what still applies
(see ADR-0006 for an example). Never edit history away.
