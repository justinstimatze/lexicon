package framestatus

import (
	"strings"
	"testing"
)

// TestParseToyRegister checks the parser keys on section headers and the
// per-section column layout (mixed = 4 cols with a handle; const/nav = 3).
func TestParseToyRegister(t *testing.T) {
	const toy = `# Oracle-risk register
## A. Constitutive pool — frame-status: constitutive
| id | name | prov |
|---|---|---|
| lex-75r77 | two-truths | audit-2 |
| lex-nahg9 | upaya | sweep |

## B. Mixed pool — frame-status: mixed
| id | name | checkable handle | prov |
|---|---|---|---|
| lex-jv983 | fluency-trap | the basis-trace test: name the supporting observation | sweep |

## C. Navigational — frame-status: navigational
| id | name | prov |
|---|---|---|
| lex-2sx7q | sequential | sweep |
`
	m := parse(strings.NewReader(toy))

	if got := len(m); got != 4 {
		t.Fatalf("want 4 entries, got %d (%v)", got, m)
	}
	if e, ok := m.Lookup("lex-75r77"); !ok || e.Status != Constitutive {
		t.Errorf("lex-75r77: want constitutive, got %+v ok=%v", e, ok)
	}
	if e, ok := m.Lookup("lex-jv983"); !ok || e.Status != Mixed || !strings.Contains(e.Handle, "basis-trace") {
		t.Errorf("lex-jv983: want mixed with basis-trace handle, got %+v ok=%v", e, ok)
	}
	if e, ok := m.Lookup("lex-2sx7q"); !ok || e.Status != Navigational {
		t.Errorf("lex-2sx7q: want navigational, got %+v ok=%v", e, ok)
	}
	if _, ok := m.Lookup("lex-9999"); ok {
		t.Errorf("lex-9999: want not-found")
	}
}

// TestLoadRealRegister reconciles the parser against the committed register:
// the published distribution is 59 navigational / 218 mixed / 79 constitutive
// = 356. This doubles as a drift guard — if the register's table format
// changes in a way the runtime parser can't read, these counts move and the
// test fails loudly instead of silently dropping frame labels in production.
func TestLoadRealRegister(t *testing.T) {
	m, err := Load("../..") // renderDir = repo's render/ ; test runs from internal/framestatus
	if err != nil {
		t.Fatalf("Load real register: %v", err)
	}
	var nav, mixed, con int
	mixedNoHandle := 0
	for id, e := range m {
		switch e.Status {
		case Navigational:
			nav++
		case Mixed:
			mixed++
			if strings.TrimSpace(e.Handle) == "" {
				mixedNoHandle++
				t.Logf("mixed atom %s has empty handle", id)
			}
		case Constitutive:
			con++
		}
	}
	if nav != 59 || mixed != 218 || con != 79 {
		t.Errorf("register distribution drifted: got %d nav / %d mixed / %d constitutive (want 59 / 218 / 79). Update framestatus parser or the expected counts.", nav, mixed, con)
	}
	if total := nav + mixed + con; total != 356 {
		t.Errorf("total CQ-bearing atoms = %d, want 356", total)
	}
	if mixedNoHandle != 0 {
		t.Errorf("%d mixed atoms have empty checkable handles (register backfill incomplete)", mixedNoHandle)
	}
}
