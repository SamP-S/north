# 026 — M5: Voice (Conversational Cockpit) — DEFERRED

Was intended to implement milestone M5 of the Build Order in
[021_session-pipeline-architecture.md](021_session-pipeline-architecture.md)
(the voice-layer subsection of "Conversation Frontend" and "The Cockpit").
M1–M4 are complete (plans 022–025, all passed). M5 is **deferred** — no
North-scope deliverable shipped.

## Outcome

Two audio-routing paths were explored for giving the text cockpit (M4) a
voice mode:

1. **Self-hosted** (021's original plan): local whisper/kokoro for STT/TTS
   plus self-hosted LiveKit + a browser frontend + TLS. VoiceMode's released
   version does not ship the self-hosted LiveKit/browser path, so this would
   have meant building custom audio-routing glue ourselves — out of scope
   for the time available. Dropped.

2. **VoiceMode Connect** (hosted relay): wired into the cockpit
   (`cockpit/.mcp.json`, `.claude/settings.json`, `CLAUDE.md`) as a
   lower-effort alternative. This did not deliver the intended Connect-based
   UX in practice and has since been **reverted** — the cockpit is back to
   text-only.

**Takeaway:** VoiceMode itself is good at producing conversational Claude
Code sessions — that part works. The unresolved problem is audio routing to
the cockpit session: self-hosting is too DIY right now, and Connect didn't
work as intended. Either needs a retry (Connect, on a later VoiceMode
release) or an alternative routing approach.

Any local STT/TTS experimentation (whisper/kokoro) done during this spike
was dev-environment setup, not part of North, and is not tracked in this
repo.

## Status

**Deferred.** Revisit after the M1–M7 migration completes (see 021's Open
Questions). The cockpit role/grants are unaffected either way — voice was
always meant to be "the same session, another input modality," so resuming
this work later requires no changes to the cockpit's board access or role.

## Change history

- [2026-06-14] Plan created from 021 M5 scope; spike begun (self-hosted
  whisper/kokoro/LiveKit).
- [2026-06-14] Self-hosted LiveKit/browser/TLS path dropped (not shipped by
  released VoiceMode, custom glue out of scope). Pivoted to VoiceMode
  Connect; wired into cockpit.
- [2026-06-15] Connect did not deliver the intended UX. Cockpit wiring
  reverted (`cockpit/.mcp.json`, `.claude/settings.json`, `CLAUDE.md`);
  README voice section replaced with a "planned" note. M5 marked deferred
  in 021's Build Order and Open Questions.
