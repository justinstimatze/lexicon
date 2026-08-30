// Package assembly parses and type-checks the assembly: grammar
// described in composition-operations.md. AST nodes are the parsed
// shape of one assembly string; type-checking validates the
// composition against the host elements' type-in / type-out
// signatures.
//
// Grammar (informal):
//
//	top      = expr ("→" expr)*
//	expr     = opCall | leaf
//	opCall   = opName "(" args? ")"
//	opName   = sequential | parallel | defeasibility-attach
//	         | choice | iteration | conditional | scoping
//	args     = arg (("," | ";") arg)*
//	arg      = expr | "..." | ident "=" rhs
//	rhs      = expr   if name == "defeaters"
//	         | ident  otherwise (treated as opaque Predicate)
//	leaf     = lex-NNNN | bare-ident
//
// Top-level "→" chains normalize to a single sequential(...) op so
// downstream type-flow checks treat the cosmetic glue uniformly.
package assembly

import (
	"strings"
)

// Node is one node in a parsed assembly tree.
type Node interface {
	Span() (start, end int)
	String() string
	nodeMarker()
}

// OpNode is a call: op(positional...; named=...).
type OpNode struct {
	Op        string
	Args      []Node
	NamedArgs map[string]Node
	NamedKeys []string // preserves source order for round-trip
	Pos, End  int
}

// AtomLeaf is a reference to an atom primitive by lex-NNNN id.
type AtomLeaf struct {
	ID       string
	Pos, End int
}

// MissingLeaf is a bare-name atom reference not (yet) carrying a
// lex-id. Surfaces as `unresolvable-atom` in lint output, both as a
// forcing function for the next mining pass and as a placeholder that
// participates in tree-edit distance.
type MissingLeaf struct {
	Name     string
	Pos, End int
}

// Predicate is an opaque named-arg value (e.g.
// "selector=is-similar-known-problem-recallable"). Predicates do not
// participate in type-checking; they exist as discrimination
// machinery the surfacing function consults.
type Predicate struct {
	Name     string
	Pos, End int
}

// Ellipsis is the literal "..." used at variadic call sites
// (e.g. `parallel(lex-axa6h, lex-axa6h, ...; aggregator=...)`).
type Ellipsis struct {
	Pos, End int
}

func (n *OpNode) Span() (int, int)      { return n.Pos, n.End }
func (n *AtomLeaf) Span() (int, int)    { return n.Pos, n.End }
func (n *MissingLeaf) Span() (int, int) { return n.Pos, n.End }
func (n *Predicate) Span() (int, int)   { return n.Pos, n.End }
func (n *Ellipsis) Span() (int, int)    { return n.Pos, n.End }

func (*OpNode) nodeMarker()      {}
func (*AtomLeaf) nodeMarker()    {}
func (*MissingLeaf) nodeMarker() {}
func (*Predicate) nodeMarker()   {}
func (*Ellipsis) nodeMarker()    {}

func (n *AtomLeaf) String() string    { return n.ID }
func (n *MissingLeaf) String() string { return n.Name }
func (n *Predicate) String() string   { return n.Name }
func (n *Ellipsis) String() string    { return "..." }

func (n *OpNode) String() string {
	var b strings.Builder
	b.WriteString(n.Op)
	b.WriteByte('(')
	first := true
	for _, a := range n.Args {
		if !first {
			b.WriteString(", ")
		}
		first = false
		b.WriteString(a.String())
	}
	for _, k := range n.NamedKeys {
		if !first {
			b.WriteString(", ")
		}
		first = false
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(n.NamedArgs[k].String())
	}
	b.WriteByte(')')
	return b.String()
}

// IsLexID reports whether s matches the canonical primitive id form
// `lex-xxxxx` — 5 characters from the hand-typing-safe alphabet (excludes
// 0/1/i/l/o). Migrated 2026-08-20 from the earlier 4-digit-only form.
func IsLexID(s string) bool {
	if len(s) != 9 || !strings.HasPrefix(s, "lex-") {
		return false
	}
	for _, c := range s[4:] {
		if !strings.ContainsRune("23456789abcdefghjkmnpqrstuvwxyz", c) {
			return false
		}
	}
	return true
}

// IsOpName reports whether s is one of the composition operators
// defined in composition-operations.md. V104 b added "classification"
// for label-molecules per ROADMAP #15.
func IsOpName(s string) bool {
	switch s {
	case "sequential", "parallel", "defeasibility-attach",
		"choice", "iteration", "conditional", "scoping",
		"classification":
		return true
	}
	return false
}

// isPredicateNamedArg names the named-arg keys whose RHS is parsed as
// an opaque Predicate rather than a typed expression. Defeaters is
// the one expression-shaped named-arg per composition-operations.md.
// V104 b: "shape" added for classification-operator label descriptors.
func isPredicateNamedArg(key string) bool {
	switch key {
	case "selector", "until", "if", "within", "aggregator", "shape":
		return true
	}
	return false
}

// CollectAtomIDs walks the tree and adds every lex-NNNN reference to
// out. Used by lint's decomposes-into / assembly consistency check.
func CollectAtomIDs(n Node, out map[string]bool) {
	switch v := n.(type) {
	case *AtomLeaf:
		out[v.ID] = true
	case *OpNode:
		for _, a := range v.Args {
			CollectAtomIDs(a, out)
		}
		for _, k := range v.NamedKeys {
			CollectAtomIDs(v.NamedArgs[k], out)
		}
	}
}

// CollectMissingNames walks the tree and adds every bare-name atom
// (MissingLeaf) to out. Used to surface the next mining-pass forcing
// function.
func CollectMissingNames(n Node, out map[string]bool) {
	switch v := n.(type) {
	case *MissingLeaf:
		out[v.Name] = true
	case *OpNode:
		for _, a := range v.Args {
			CollectMissingNames(a, out)
		}
		for _, k := range v.NamedKeys {
			CollectMissingNames(v.NamedArgs[k], out)
		}
	}
}
