// Package client wraps the Anthropic Go SDK behind a small interface
// so test code can substitute a fake without importing the real
// SDK's types.
//
// The interface is deliberately minimal: one method, mirroring
// anthropic.Messages.New, parameterized only by the fields lexicon's
// narrative renderer actually uses. Adding fields (e.g., for a future
// streaming mode) means widening this interface, not the consumer's
// imports.
package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// Model is the canonical render-function model id (used by narrative,
// introspection, etc — quality-sensitive paths). Pinned here so per-mode
// renderers don't each pick a different one.
const Model = "claude-sonnet-4-6"

// LensModel is the fast/cheap model used by the production hook's lens
// layer. Latency-sensitive (sub-second on every UserPromptSubmit), so
// Haiku not Sonnet. V12: added when the lens layer was wired in to fix
// token-collision noise per fires-jsonl-analysis-pass-1.md.
const LensModel = "claude-haiku-4-5-20251001"

// MessageRequest is what render modes hand to a Client. Model is
// optional — empty defaults to Model (Sonnet). Set to LensModel for
// the hook's filter call.
//
// CachedSystem is V13: when non-empty, sent as a SECOND system block
// with cache_control: ephemeral (5min TTL). Use for large static
// content (e.g., the lens elements index) so subsequent calls within
// the TTL pay cache-read price (~10% of input) instead of full input.
// The block goes at the end so the cache prefix covers System +
// CachedSystem together. Anthropic requires the cached block to be
// at least ~1024 tokens; smaller blocks silently won't cache.
type MessageRequest struct {
	System       string
	CachedSystem string
	UserText     string
	MaxTokens    int
	Model        string
}

// MessageResponse is the small slice of the SDK response lexicon
// surfaces. Text is the first text block content; token counts
// populate the --why introspection trace and `lexicon doctor`'s exact
// cost rollup. CacheReadTokens / CacheCreationTokens are the V13
// prompt-cache accounting (zero when no caching is in play).
type MessageResponse struct {
	Text                string
	InputTokens         int64
	OutputTokens        int64
	CacheReadTokens     int64
	CacheCreationTokens int64
}

// Client is the interface render modes consume. Production code uses
// New(); test code substitutes a fake.
type Client interface {
	CreateMessage(ctx context.Context, req MessageRequest) (MessageResponse, error)
}

type sdkClient struct {
	inner *anthropic.Client
}

// New returns a real Anthropic client wired with the API key from
// ANTHROPIC_API_KEY. The .env loader runs in main() before this is
// called; we don't reach into the filesystem here.
func New() (Client, error) {
	key := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		return nil, errors.New(
			"ANTHROPIC_API_KEY not set. Easiest path: edit render/.env " +
				"(gitignored) and add ANTHROPIC_API_KEY=sk-ant-... " +
				"— see render/.env.example for shape",
		)
	}
	// hardHTTPTimeout is a backstop independent of the ctx callers pass to
	// CreateMessage. The lens layer's own budgets are 8s inner (lens.go's
	// DefaultTimeout) / 12s outer (cmd_hook.go's lensBudget) — both meant
	// to be the binding constraint in the normal case. This exists for
	// when they aren't: fires.jsonl history has a real lens call clocking
	// 309124ms (5.15min) despite that 12s backstop, which only ctx
	// composition enforced — this adds a second, client-level mechanism
	// that doesn't depend on cancellation propagating correctly through
	// the SDK's retry logic to catch it.
	const hardHTTPTimeout = 20 * time.Second
	c := anthropic.NewClient(
		option.WithAPIKey(key),
		option.WithHTTPClient(&http.Client{Timeout: hardHTTPTimeout}),
	)
	return &sdkClient{inner: &c}, nil
}

func (s *sdkClient) CreateMessage(ctx context.Context, req MessageRequest) (MessageResponse, error) {
	max := int64(req.MaxTokens)
	if max == 0 {
		max = 1024
	}
	model := req.Model
	if model == "" {
		model = Model
	}
	systemBlocks := []anthropic.TextBlockParam{
		{Text: req.System},
	}
	if req.CachedSystem != "" {
		// Second block carries cache_control: ephemeral. Anthropic caches
		// the prefix UP TO this block, so [System + CachedSystem] all gets
		// cached together. Subsequent calls within the 5min TTL with the
		// same prefix pay cache-read price (~10% of input).
		systemBlocks = append(systemBlocks, anthropic.TextBlockParam{
			Text:         req.CachedSystem,
			CacheControl: anthropic.NewCacheControlEphemeralParam(),
		})
	}
	resp, err := s.inner.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: max,
		System:    systemBlocks,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(req.UserText)),
		},
	})
	if err != nil {
		return MessageResponse{}, fmt.Errorf("client: anthropic: %w", err)
	}
	var text string
	for _, block := range resp.Content {
		if block.Type == "text" {
			text = block.Text
			break
		}
	}
	if text == "" {
		return MessageResponse{}, errors.New("client: anthropic response had no text block")
	}
	return MessageResponse{
		Text:                text,
		InputTokens:         resp.Usage.InputTokens,
		OutputTokens:        resp.Usage.OutputTokens,
		CacheReadTokens:     resp.Usage.CacheReadInputTokens,
		CacheCreationTokens: resp.Usage.CacheCreationInputTokens,
	}, nil
}

// LoadDotEnv reads KEY=VALUE pairs from .env and sets them in the
// process environment if not already set. Tiny inline implementation —
// no godotenv dep for this trivial format.
func LoadDotEnv(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("client: read %s: %w", path, err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, val)
		}
	}
	return nil
}
