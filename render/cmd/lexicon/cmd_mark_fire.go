package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/justinstimatze/lexicon/render/internal/types"
)

// cmdMarkFire is the per-fire vibe-tagging companion to fires.jsonl.
// Appends one line to ~/.claude/lexicon/fire-tags.jsonl recording that
// a specific hook_event_id was useful / mixed / not-useful / autonomous.
// Pair with the hook_event_id from either ~/.claude/lexicon/hook.log
// ("event=...") or fires.jsonl. Per surfacing-function-utility-pass-1.md
// item 2 in the v0.5 instrumentation list.
//
// Usage:
//
//	lexicon mark-fire <hook_event_id> <vibe> [--notes "..."]
//
// Vibes use the same vocabulary as `lexicon log` (per-session vibe):
// useful | mixed | not-useful | autonomous. fire-tags.jsonl and
// session-log.jsonl are intentionally separate files — one is per-fire
// (what individual surfaces produced), the other is per-session (overall
// gut-check).
type fireTagRecord struct {
	Ts          string           `json:"ts"`
	HookEventID string           `json:"hook_event_id"`
	Vibe        types.SessionTag `json:"vibe"`
	Notes       string           `json:"notes,omitempty"`
}

func cmdMarkFire(renderDir string, args []string) {
	if len(args) < 2 {
		fatal("usage: lexicon mark-fire <hook_event_id> <vibe> [--notes \"...\"]\n  vibe: useful | mixed | not-useful | autonomous")
	}
	eventID := args[0]
	vibe := types.SessionTag(args[1])
	if !types.ValidSessionTag(vibe) {
		fatal("vibe must be one of: useful | mixed | not-useful | autonomous (got %q)", string(vibe))
	}
	fl := flag.NewFlagSet("mark-fire", flag.ExitOnError)
	notes := fl.String("notes", "", "optional free-form notes about why this fire was tagged this way")
	_ = fl.Parse(args[2:])

	rec := fireTagRecord{
		Ts:          time.Now().UTC().Format(time.RFC3339),
		HookEventID: eventID,
		Vibe:        vibe,
		Notes:       *notes,
	}
	home, err := os.UserHomeDir()
	if err != nil {
		fatal("cannot resolve home directory: %v", err)
	}
	dir := filepath.Join(home, ".claude", "lexicon")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		fatal("mkdir %s: %v", dir, err)
	}
	path := filepath.Join(dir, "fire-tags.jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		fatal("open %s: %v", path, err)
	}
	defer f.Close()
	data, _ := json.Marshal(rec)
	fmt.Fprintln(f, string(data))
	fmt.Printf("tagged fire %s as %s -> %s\n", eventID, vibe, path)
}
