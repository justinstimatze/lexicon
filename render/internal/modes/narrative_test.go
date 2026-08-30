package modes

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/justinstimatze/lexicon/render/internal/client"
	"github.com/justinstimatze/lexicon/render/internal/types"
)

// fakeClient is a minimal client.Client substitute: records the last
// request so tests can assert on system prompt + user message
// construction without making a real API call.
type fakeClient struct {
	lastReq client.MessageRequest
	resp    client.MessageResponse
	err     error
}

func (f *fakeClient) CreateMessage(_ context.Context, req client.MessageRequest) (client.MessageResponse, error) {
	f.lastReq = req
	if f.err != nil {
		return client.MessageResponse{}, f.err
	}
	return f.resp, nil
}

var fixtureForNarrative = &types.LexEntry{
	ID:                 "lex-kebfa",
	Name:               "argument-from-expert-opinion",
	TypeIn:             "claim",
	TypeOut:            "posture",
	Tier:               "molecule",
	Evokes:             []string{"appeal-to-authority"},
	Premises:           []string{"E is expert in D", "E asserts A"},
	CriticalQuestions:  []string{"is E really an expert in D?"},
	DecomposesInto:     []string{"lex-q9asc"},
	Lineage:            []types.LineageEntry{{Source: "walton", Text: "wrm-2008", Citation: "ch.3"}},
	CanonicalInstances: []string{"Dr. X (oncology) says T helps C"},
	Status:             "under-review",
}

func TestNarrativeReturnsClientText(t *testing.T) {
	fake := &fakeClient{resp: client.MessageResponse{
		Text: "mocked output", InputTokens: 100, OutputTokens: 50,
	}}
	out, err := Narrative(context.Background(), fake, fixtureForNarrative, "user mid-bind", nil)
	if err != nil {
		t.Fatalf("Narrative: %v", err)
	}
	if out.Text != "mocked output" || out.Mode != types.ModeNarrative {
		t.Fatalf("unexpected output: %+v", out)
	}
}

func TestNarrativeIncludesTokenUsageInTrace(t *testing.T) {
	fake := &fakeClient{resp: client.MessageResponse{
		Text: "x", InputTokens: 100, OutputTokens: 50,
	}}
	out, _ := Narrative(context.Background(), fake, fixtureForNarrative, "ctx", nil)
	if !strings.Contains(out.IntrospectionTrace, "input_tokens=100") ||
		!strings.Contains(out.IntrospectionTrace, "output_tokens=50") {
		t.Fatalf("trace missing token counts: %s", out.IntrospectionTrace)
	}
}

// User-message construction is load-bearing: the LLM gets the entry
// name (so it knows what move to demonstrate) but the system prompt
// instructs it NOT to surface the name. Test that we send what we
// promise.
func TestNarrativeUserMessageIncludesEntryFields(t *testing.T) {
	fake := &fakeClient{resp: client.MessageResponse{Text: "x"}}
	_, _ = Narrative(context.Background(), fake, fixtureForNarrative, "user is stuck on expert claim", []string{"expert", "credibility"})
	checks := []string{
		"Entry name (do not surface to user): argument-from-expert-opinion",
		"E is expert in D",
		"is E really an expert in D?",
		"User's context: user is stuck on expert claim",
		"User's working vocabulary hints: expert, credibility",
	}
	for _, want := range checks {
		if !strings.Contains(fake.lastReq.UserText, want) {
			t.Errorf("user message missing %q in:\n%s", want, fake.lastReq.UserText)
		}
	}
}

// System prompt carries the deployment-mode constraints. If these
// drift, narrative output silently shifts shape (verbose, third-
// person, name-leaking) and the smoke-test catches it later.
func TestNarrativeSystemPromptCarriesConstraints(t *testing.T) {
	fake := &fakeClient{resp: client.MessageResponse{Text: "x"}}
	_, _ = Narrative(context.Background(), fake, fixtureForNarrative, "ctx", nil)
	checks := []string{"100-300 words", "Second-person", "Do NOT name the entry"}
	for _, want := range checks {
		if !strings.Contains(fake.lastReq.System, want) {
			t.Errorf("system prompt missing %q", want)
		}
	}
}

func TestNarrativePropagatesClientError(t *testing.T) {
	fake := &fakeClient{err: errors.New("upstream sad")}
	_, err := Narrative(context.Background(), fake, fixtureForNarrative, "ctx", nil)
	if err == nil || !strings.Contains(err.Error(), "upstream sad") {
		t.Fatalf("expected propagated error, got %v", err)
	}
}
