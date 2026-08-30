// Package modes contains the per-mode renderers (one file per mode).
// Modes are pure functions over an entry plus optional context — they
// never reach for global state, so each can be tested in isolation
// with a small literal LexEntry fixture.
//
// Modes that need to look up sibling entries (visual, introspection)
// take an entry-pool map as a parameter rather than calling the
// loader. Keeps tests fast and decouples render-shape from
// elements-discovery.
package modes

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/justinstimatze/lexicon/render/internal/types"
)

// Algebraic renders the raw elements as YAML. The "algebraic" name
// (per molecule-representation-design.md) is meant for the LLM matcher
// or the maintainer drilling in — not for end-user reading.
func Algebraic(e *types.LexEntry) types.RenderOutput {
	out, err := yaml.Marshal(e)
	if err != nil {
		// LexEntry is yaml.v3-marshalable by construction; the only
		// way this fails is OOM, which is a process-level problem.
		// Surface as text rather than panic so the CLI can keep going.
		return types.RenderOutput{
			PrimitiveID: e.ID,
			Mode:        types.ModeAlgebraic,
			Text:        fmt.Sprintf("# %s %s — algebraic (marshal error)\n%v\n", e.ID, e.Name, err),
		}
	}
	text := fmt.Sprintf("# %s %s — algebraic (raw elements)\n\n%s", e.ID, e.Name, string(out))
	return types.RenderOutput{
		PrimitiveID: e.ID,
		Mode:        types.ModeAlgebraic,
		Text:        text,
	}
}
