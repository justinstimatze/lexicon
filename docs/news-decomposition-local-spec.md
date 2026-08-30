# News-decomposition tool — V1 spec (LOCAL / PRIVATE)

Status: sketch (2026-05-28). Not built, and uncommitted pending a decision on whether it lives in-repo or in a future spinout. Public deploy and domain are explicitly **out of scope** for V1.

## Goal

Forward an article (URL or text) from a phone → get back which lexicon **reactions / molecules / atoms** it fires, with the firing entries' catalyst / inhibitor slots (the "where's the lever" view). The predict/intervene reaction-tier vision, applied to real content.

## Hard constraints

- **Probe-only, NO LLM.** Deterministic embedding-match has zero instruction-following surface, so a forwarded (untrusted) article is pure data — prompt injection is impossible by construction. Do not add an LLM layer in V1. (If ever added later: article as fenced data, output-schema-bound, no tools, no secrets, no outbound — contain blast radius, don't trust the fence.)
- **Stateless.** The elements/ set is files baked into the binary. No DB (Supabase unneeded until/unless we log decompositions or add accounts).
- **Source-agnostic framing.** These reactions fire on MSNBC as readily as Fox; it's a decomposition lens, not a partisan-debunk tool.
- **Local-only networking for V1.** No public exposure, no domain, no TLS. Phone hits the box on home Wi-Fi over plain HTTP to a LAN IP. (iOS Shortcuts allow http:// to local IPs; they reject self-signed certs, so it's plain-HTTP-LAN now, real cert only when going public.)

## Architecture

```
iOS share sheet ──(POST url|text)──▶ Go HTTP endpoint ──▶ render/internal/probe ──▶ JSON matches ──▶ Shortcut renders
                                          │
                                          └─(if url) fetch + readability-extract → text
```

### Component 1 — Go probe endpoint (new, small)

- New command, e.g. `cmd/probe-server` (or a `lexicon serve` subcommand), wrapping the existing `render/internal/probe` package — the same matcher the UserPromptSubmit hook uses.
- `POST /decompose`  body: `{"text": "..."}` OR `{"url": "https://..."}`
- For `url`: fetch + readability-extract to plain text (V1: basic fetch; **accept pasted `text` to sidestep** paywalls / JS-rendered pages — URL extraction is the one genuinely fiddly part, harden later).
- Response:
  ```json
  {
    "matches": [
      {
        "id": "lex-r3w25",
        "name": "confirmation-filtered-tribal-perception",
        "tier": "reaction",
        "confidence": 0.81,
        "reactants": ["lex-hbgcb"],
        "catalysts": ["lex-t9fs7", "homogeneous information-streams"],
        "inhibitors": ["high-base-rate survey of actual in-group views"],
        "products": ["a perceived consensus that is a bias artifact"]
      }
    ]
  }
  ```
  (Reactions return their slots; molecules return `assembly`/`decomposes-into`; atoms return name + confidence. Slots come straight from the elements YAML.)
- Bind to LAN only (`0.0.0.0:PORT` on the home network, or localhost + the phone tethered); takes no actions beyond returning JSON.

### Component 2 — iOS Shortcut

1. *Receive* URLs and Text *from Share Sheet*.
2. *Get Contents of URL* → `POST http://<home-ip>:<PORT>/decompose`, body `{"url": <shared url>}` (or `{"text": <shared text>}`).
3. *Get Dictionary from Input* → iterate `matches` → format as a list (name · tier · confidence, then a line each for catalysts/inhibitors).
4. *Show Result* (Quick Look or a simple text sheet).

No app to build, no app store, no chat platform.

## Deferred to "going public" (NOT V1)

- Deploy the Go binary to **Fly.io** → free `*.fly.dev` hostname + TLS (no domain purchase). 
- **Cloudflare** in front only if a custom domain is wanted.
- Android surface: PWA with Web Share Target.
- Optional capability-constrained LLM gloss layer.

## Open questions

- URL → clean article text quality (paywalls, JS-rendered pages). V1 leans on pasted text + basic fetch.
- Probe ranking / threshold tuning for article-length input (the probe was calibrated on shorter prompts — may need a length-aware threshold or top-N cap).
- Compact mobile rendering of reaction slots (catalysts/inhibitors) — what's the minimal useful view on a phone screen.
