package assembly

import (
	"fmt"

	"github.com/justinstimatze/lexicon/render/internal/types"
)

// Diagnostic is one finding from type-check or lint.
type Diagnostic struct {
	Severity string // "error" | "warning" | "info"
	Code     string // e.g. "type-mismatch", "unresolvable-atom"
	Message  string
	EntryID  string
	Pos      int // byte offset in the assembly string; -1 if structural
}

// TypeCheck validates the assembly tree against the elements' typed
// primitives. Diagnostics are accumulated, not thrown — a single
// assembly may produce multiple findings.
//
// Rules enforced (per composition-operations.md):
//   - sequential(A, B, ...): typeOut(A) ≡ typeIn(B) for each adjacent pair.
//   - parallel(A, B, ...):   all arms share typeIn.
//   - choice(A, B, ...):     all arms share typeIn (same shape as parallel).
//   - iteration(A, ...):     each positional arg satisfies typeIn(A) ≡ typeOut(A)
//     (fixed-point shape).
//   - defeasibility-attach, conditional, scoping: no extra constraint enforced
//     in v0; child-recursion only. The defeater type is not in the bounded
//     vocabulary, and the conditional/scoping arm-shapes are still informal.
//   - classification(A, B, ...): label-molecule operator (per ROADMAP #15).
//     Members are unordered independent instances of a shared structural
//     signature; the molecule classifies them without imposing a shared
//     type-in. No type constraint enforced; type-in/type-out pass through
//     the first arm like defeasibility-attach.
//
// Missing atoms (lex-NNNN not in elements) and bare-name leaves are
// surfaced as warnings, not errors — elements accretion uses these
// as forcing functions.
func TypeCheck(node Node, host *types.LexEntry, elements map[string]*types.LexEntry) []Diagnostic {
	tc := &typechecker{elements: elements, host: host}
	tc.check(node)
	return tc.diags
}

type typechecker struct {
	elements map[string]*types.LexEntry
	host      *types.LexEntry
	diags     []Diagnostic
}

func (tc *typechecker) check(n Node) {
	if n == nil {
		return
	}
	switch v := n.(type) {
	case *OpNode:
		tc.checkOp(v)
	case *AtomLeaf:
		tc.checkAtomLeaf(v)
	case *MissingLeaf:
		tc.checkMissingLeaf(v)
	case *Ellipsis, *Predicate:
		// no-op: ellipsis ok at variadic sites; predicates are opaque
	}
}

// stepLabelOps lists operators whose direct MissingLeaf children are
// intentional tactic-internal step labels (sequence steps, classification
// members, choice alternatives), NOT unresolved elements-atom references.
// Bare-name warnings are suppressed for these children. choice() with a
// `selector=` named arg is the canonical case where the bare-name
// alternatives are internal-to-the-pattern options the selector ranges
// over (e.g. physics frameworks selected by scale regime).
var stepLabelOps = map[string]bool{
	"sequential":     true,
	"classification": true,
	"choice":         true,
}

func (tc *typechecker) checkAtomLeaf(a *AtomLeaf) {
	if _, ok := tc.elements[a.ID]; !ok {
		tc.warn("unresolvable-atom",
			fmt.Sprintf("assembly references %s but it's not in the elements", a.ID), a.Pos)
	}
}

func (tc *typechecker) checkMissingLeaf(m *MissingLeaf) {
	tc.warn("unresolvable-atom",
		fmt.Sprintf("bare-name atom %q has no lex-id (forcing function for next mining pass)", m.Name), m.Pos)
}

func (tc *typechecker) checkOp(op *OpNode) {
	suppressBareName := stepLabelOps[op.Op]
	for _, a := range op.Args {
		if suppressBareName {
			if _, ok := a.(*MissingLeaf); ok {
				continue
			}
		}
		tc.check(a)
	}
	for _, k := range op.NamedKeys {
		a := op.NamedArgs[k]
		if suppressBareName {
			if _, ok := a.(*MissingLeaf); ok {
				continue
			}
		}
		tc.check(a)
	}
	switch op.Op {
	case "sequential":
		tc.checkSequential(op)
	case "parallel":
		tc.checkParallel(op, "parallel-input-mismatch")
	case "choice":
		tc.checkParallel(op, "choice-input-mismatch")
	case "iteration":
		tc.checkIteration(op)
	case "defeasibility-attach", "conditional", "scoping", "classification":
		// child-recursion only in v0
		// classification: label-molecule operator; members are unordered
		// independent instances of a shape; no shared-type constraint.
	}
}

func (tc *typechecker) checkSequential(op *OpNode) {
	if len(op.Args) < 2 {
		return
	}
	for i := 0; i < len(op.Args)-1; i++ {
		out := tc.typeOut(op.Args[i])
		in := tc.typeIn(op.Args[i+1])
		if out == "" || in == "" {
			continue // upstream warning (missing atom) covers it
		}
		if out != in {
			pos, _ := op.Args[i+1].Span()
			tc.err("type-mismatch",
				fmt.Sprintf("sequential: %s type-out=%q ≠ %s type-in=%q",
					nodeLabel(op.Args[i]), out,
					nodeLabel(op.Args[i+1]), in), pos)
		}
	}
}

func (tc *typechecker) checkParallel(op *OpNode, code string) {
	var first string
	var firstNode Node
	for _, a := range op.Args {
		if _, ok := a.(*Ellipsis); ok {
			continue
		}
		in := tc.typeIn(a)
		if in == "" {
			continue
		}
		if first == "" {
			first = in
			firstNode = a
			continue
		}
		if in != first {
			pos, _ := a.Span()
			tc.err(code,
				fmt.Sprintf("%s arms disagree on type-in: %s=%q vs %s=%q",
					op.Op, nodeLabel(firstNode), first, nodeLabel(a), in), pos)
		}
	}
}

func (tc *typechecker) checkIteration(op *OpNode) {
	for _, a := range op.Args {
		if _, ok := a.(*Ellipsis); ok {
			continue
		}
		in := tc.typeIn(a)
		out := tc.typeOut(a)
		if in == "" || out == "" {
			continue
		}
		if in != out {
			pos, _ := a.Span()
			tc.warn("iteration-not-fixed-point",
				fmt.Sprintf("iteration arg %s has type-in=%q but type-out=%q (expected equal for fixed-point shape)",
					nodeLabel(a), in, out), pos)
		}
	}
}

// typeIn / typeOut return the inferred input/output type for any node,
// or "" when unresolvable (missing atom, predicate, ellipsis).
//
// For OpNodes, the rules mirror composition-operations.md:
//   - sequential: typeIn = first arg's typeIn; typeOut = last arg's typeOut.
//   - parallel/choice: typeIn = arms' shared typeIn; typeOut = arms' shared
//     typeOut (we report the first non-empty seen).
//   - iteration: typeIn = typeOut = arg's typeIn (assumed fixed-point).
//   - defeasibility-attach / conditional / scoping: pass through the first arm.
func (tc *typechecker) typeIn(n Node) string {
	switch v := n.(type) {
	case *AtomLeaf:
		if e, ok := tc.elements[v.ID]; ok {
			return e.TypeIn
		}
	case *OpNode:
		switch v.Op {
		case "sequential", "iteration", "defeasibility-attach", "conditional", "scoping", "classification":
			if len(v.Args) > 0 {
				return tc.typeIn(v.Args[0])
			}
		case "parallel", "choice":
			for _, a := range v.Args {
				if t := tc.typeIn(a); t != "" {
					return t
				}
			}
		}
	}
	return ""
}

func (tc *typechecker) typeOut(n Node) string {
	switch v := n.(type) {
	case *AtomLeaf:
		if e, ok := tc.elements[v.ID]; ok {
			return e.TypeOut
		}
	case *OpNode:
		switch v.Op {
		case "sequential":
			if len(v.Args) > 0 {
				return tc.typeOut(v.Args[len(v.Args)-1])
			}
		case "iteration":
			if len(v.Args) > 0 {
				return tc.typeOut(v.Args[0])
			}
		case "defeasibility-attach", "conditional", "scoping", "classification":
			if len(v.Args) > 0 {
				return tc.typeOut(v.Args[0])
			}
		case "parallel", "choice":
			for _, a := range v.Args {
				if t := tc.typeOut(a); t != "" {
					return t
				}
			}
		}
	}
	return ""
}

func (tc *typechecker) err(code, msg string, pos int) {
	tc.diags = append(tc.diags, Diagnostic{Severity: "error", Code: code, Message: msg, EntryID: tc.host.ID, Pos: pos})
}

func (tc *typechecker) warn(code, msg string, pos int) {
	tc.diags = append(tc.diags, Diagnostic{Severity: "warning", Code: code, Message: msg, EntryID: tc.host.ID, Pos: pos})
}

// nodeLabel renders a short human-readable handle for a node (id, bare
// name, or op summary) used inside diagnostic messages.
func nodeLabel(n Node) string {
	switch v := n.(type) {
	case *AtomLeaf:
		return v.ID
	case *MissingLeaf:
		return v.Name
	case *OpNode:
		return v.Op + "(...)"
	case *Predicate:
		return v.Name
	case *Ellipsis:
		return "..."
	}
	return "?"
}
