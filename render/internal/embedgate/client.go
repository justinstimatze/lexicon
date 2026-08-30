package embedgate

// client.go — the ollama embedding surface for the lexicon gate. Raw net/http
// on purpose; the hook is on every UserPromptSubmit so it wants zero deps and
// one tight timeout with no retries. Lifted from cupel
// (github.com/justinstimatze/cupel, cmd/cupel/client.go) — the
// Anthropic/lens half of cupel is unused here because lexicon already has
// render/internal/client for that.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// EmbedModel returns the ollama embedding model name. Defaults to
// nomic-embed-text — cupel's held-out probe found it gave the cleanest class
// separation at ~60ms warm (POS floor 0.589 > NEG ceiling 0.556). Lexicon
// should use the same model so embeddings + threshold tuning transfer.
func EmbedModel() string {
	if m := os.Getenv("LEXICON_EMBED_MODEL"); m != "" {
		return m
	}
	return "nomic-embed-text"
}

// OllamaURL returns the ollama base URL. Defaults to localhost:11434.
func OllamaURL() string {
	if u := os.Getenv("LEXICON_OLLAMA_URL"); u != "" {
		return strings.TrimRight(u, "/")
	}
	return "http://localhost:11434"
}

// embedKeepAlive returns the ollama keep_alive value sent with every embed
// request. default -1 = never unload; override with LEXICON_EMBED_KEEP_ALIVE
// (ollama duration like "5m", or seconds as a number). Without this, ollama's
// default 5-minute keep-alive unloads nomic-embed-text between calls on an
// idle host, and the next embed pays a cold-load penalty that can exceed
// HTTPTimeout — surfacing as "context deadline exceeded" and a silent
// fallback to the lexical (non-semantic) path.
func embedKeepAlive() any {
	if v := os.Getenv("LEXICON_EMBED_KEEP_ALIVE"); v != "" {
		return v
	}
	return -1
}

// HTTPTimeout bounds the embedding call. A hook must never hang a turn; on
// timeout, the gate fails open (caller falls back to lens-first flow). Default
// 60s, deliberately generous: with multiple lexicon sessions sharing one ollama
// runner, requests must QUEUE inside ollama rather than abort on the client
// side. Aborted requests log as "aborting embedding request due to client
// closing the connection" and waste work already in flight.
func HTTPTimeout() time.Duration {
	if v := os.Getenv("LEXICON_EMBED_TIMEOUT_MS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return time.Duration(n) * time.Millisecond
		}
	}
	return 60000 * time.Millisecond
}

// MaxInFlight bounds concurrent ollama embed calls FROM THIS PROCESS. With
// ~8 concurrent claude sessions each running its own lexicon MCP, a per-process
// cap of 1 keeps the host runner from being dogpiled. Override with
// LEXICON_EMBED_MAX_INFLIGHT; the cross-session bound is OLLAMA_NUM_PARALLEL
// on the ollama server itself (recommend ≤4 on an oversubscribed box).
func MaxInFlight() int {
	if v := os.Getenv("LEXICON_EMBED_MAX_INFLIGHT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 1
}

// embedSem caps concurrent in-flight ollama embed calls per process. Lazy-init
// on first use so MaxInFlight env override is honored.
var (
	embedSemOnce sync.Once
	embedSem     chan struct{}
)

func acquireEmbed(ctx context.Context) error {
	embedSemOnce.Do(func() {
		embedSem = make(chan struct{}, MaxInFlight())
	})
	select {
	case embedSem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func releaseEmbed() { <-embedSem }

// embedFlight collapses concurrent identical embed batches into one round trip.
// Two callers in the same process with the same (model, texts) wait on one
// ollama call. Most direct win for the hot path of repeated identical prompts
// within a session.
var embedFlight singleflight.Group

func embedKey(texts []string) string {
	h := sha256.New()
	h.Write([]byte(EmbedModel()))
	h.Write([]byte{0})
	for _, t := range texts {
		h.Write([]byte(t))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

type embedReq struct {
	Model     string   `json:"model"`
	Input     []string `json:"input"`
	KeepAlive any      `json:"keep_alive,omitempty"`
}
type embedResp struct {
	Embeddings [][]float64 `json:"embeddings"`
}

// batchSize bounds how many inputs go in one ollama request. Hand-tested:
// nomic-embed-text on a warm model handles ~50 inputs in ~2.5s; 247 in one
// shot exceeds ollama's per-request limits. The timeout is per-batch so total
// wall time = (N/batchSize) * per-batch-ceiling.
const batchSize = 50

// EmbedTexts returns one L2-normalized vector per input (so cosine reduces to
// a plain dot product downstream). Internally chunked into batches of
// batchSize; one ollama round trip per chunk.
func EmbedTexts(ctx context.Context, texts []string) ([][]float64, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	out := make([][]float64, 0, len(texts))
	for i := 0; i < len(texts); i += batchSize {
		end := i + batchSize
		if end > len(texts) {
			end = len(texts)
		}
		vecs, err := embedBatch(ctx, texts[i:end])
		if err != nil {
			return nil, err
		}
		out = append(out, vecs...)
	}
	return out, nil
}

// embedBatch issues one ollama call for up to batchSize inputs. Wrapped in
// singleflight (collapse duplicate concurrent calls) + a bounded semaphore
// (cap per-process in-flight ollama hits). Both are defenses against the
// multi-session dogpile pattern where N claude sessions burst-fire identical
// embeds against a single nomic-embed-text runner.
func embedBatch(ctx context.Context, texts []string) ([][]float64, error) {
	key := embedKey(texts)
	v, err, _ := embedFlight.Do(key, func() (any, error) {
		return embedBatchInner(ctx, texts)
	})
	if err != nil {
		return nil, err
	}
	return v.([][]float64), nil
}

func embedBatchInner(ctx context.Context, texts []string) ([][]float64, error) {
	if err := acquireEmbed(ctx); err != nil {
		return nil, err
	}
	defer releaseEmbed()
	ctx, cancel := context.WithTimeout(ctx, HTTPTimeout())
	defer cancel()
	body, _ := json.Marshal(embedReq{Model: EmbedModel(), Input: texts, KeepAlive: embedKeepAlive()})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		OllamaURL()+"/api/embed", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	// A dedicated client with its own Timeout, not http.DefaultClient —
	// this is a backstop independent of ctx cancellation. fires.jsonl
	// history has real embed calls clocking 226172ms and 78886ms despite
	// the ctx-composed deadline above (HTTPTimeout(), 60s default) that
	// was supposed to bound them; something about how that composition
	// reaches the request isn't airtight. A client-level Timeout doesn't
	// share that failure mode — it fires regardless of what ctx did.
	// Reuses http.DefaultTransport's connection pool (nil Transport
	// falls back to it), so this costs nothing per call.
	httpClient := &http.Client{Timeout: HTTPTimeout()}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama embed: status %d", resp.StatusCode)
	}
	var er embedResp
	if err := json.NewDecoder(resp.Body).Decode(&er); err != nil {
		return nil, err
	}
	if len(er.Embeddings) != len(texts) {
		return nil, fmt.Errorf("ollama embed: got %d vectors for %d inputs", len(er.Embeddings), len(texts))
	}
	for i := range er.Embeddings {
		normalize(er.Embeddings[i])
	}
	return er.Embeddings, nil
}

func normalize(v []float64) {
	var n float64
	for _, x := range v {
		n += x * x
	}
	n = math.Sqrt(n)
	if n == 0 {
		return
	}
	for i := range v {
		v[i] /= n
	}
}

// Cosine of two already-normalized vectors (plain dot product).
func Cosine(a, b []float64) float64 {
	if len(a) != len(b) {
		return -1
	}
	var dot float64
	for i := range a {
		dot += a[i] * b[i]
	}
	return dot
}

// BestCosine returns the highest cosine similarity between query and any of
// an atom's cached instance vectors — an atom's overall matchability isn't
// capped by whichever canonical-instance happens to embed closest to how
// this particular prompt is phrased. Exported so cmd_calib.go and
// cmd_replay_fires.go can reproduce Score's exact scoring logic.
func BestCosine(query []float64, vectors [][]float64) float64 {
	best := -2.0 // below Cosine's own [-1,1] range, so any real vector wins
	for _, v := range vectors {
		if s := Cosine(query, v); s > best {
			best = s
		}
	}
	return best
}
