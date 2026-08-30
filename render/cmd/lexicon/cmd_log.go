package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"path/filepath"

	"github.com/justinstimatze/lexicon/render/internal/session"
	"github.com/justinstimatze/lexicon/render/internal/types"
)

func cmdLog(renderDir string, args []string) {
	if len(args) < 1 {
		fatal("missing vibe positional (one of: useful | mixed | not-useful | autonomous)")
	}
	vibe := types.SessionTag(args[0])
	fl := flag.NewFlagSet("log", flag.ExitOnError)
	count := fl.Int("count", 0, "render call count for this session")
	notes := fl.String("notes", "", "optional free-form notes")
	// Skip positional (args[0]); stdlib flag stops at first non-flag.
	_ = fl.Parse(args[1:])

	if !types.ValidSessionTag(vibe) {
		fatal("vibe must be one of: useful | mixed | not-useful | autonomous (got %q)", string(vibe))
	}

	entry := session.MakeEntry(vibe, *count, *notes)
	logPath := filepath.Join(renderDir, session.DefaultLogPath)
	if err := session.Append(logPath, entry); err != nil {
		fatal("%v", err)
	}
	data, _ := json.Marshal(entry)
	fmt.Printf("logged: %s\n", string(data))
}
