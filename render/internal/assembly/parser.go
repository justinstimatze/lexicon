package assembly

import (
	"fmt"
)

// Parse parses one assembly string into a Node tree. Top-level "→"
// chains normalize to a single sequential(...) so downstream
// type-flow checks treat the glue as the spec describes (cosmetic
// shorthand, not a separate operator).
func Parse(src string) (Node, error) {
	toks, err := tokenize(src)
	if err != nil {
		return nil, err
	}
	p := &parser{toks: toks, src: src}
	n, err := p.parseTop()
	if err != nil {
		return nil, err
	}
	if p.peek().kind != tokEOF {
		t := p.peek()
		return nil, fmt.Errorf("assembly: trailing tokens after parse, next=%q at byte %d", t.val, t.pos)
	}
	return n, nil
}

// --- tokenizer ---

type tokKind int

const (
	tokEOF tokKind = iota
	tokIdent
	tokLParen
	tokRParen
	tokComma
	tokSemi
	tokEq
	tokArrow
	tokEllipsis
)

type tok struct {
	kind tokKind
	val  string
	pos  int
	end  int
}

func tokenize(s string) ([]tok, error) {
	var out []tok
	i := 0
	for i < len(s) {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == '(':
			out = append(out, tok{tokLParen, "(", i, i + 1})
			i++
		case c == ')':
			out = append(out, tok{tokRParen, ")", i, i + 1})
			i++
		case c == ',':
			out = append(out, tok{tokComma, ",", i, i + 1})
			i++
		case c == ';':
			out = append(out, tok{tokSemi, ";", i, i + 1})
			i++
		case c == '=':
			out = append(out, tok{tokEq, "=", i, i + 1})
			i++
		case c == '.':
			if i+2 < len(s) && s[i+1] == '.' && s[i+2] == '.' {
				out = append(out, tok{tokEllipsis, "...", i, i + 3})
				i += 3
				continue
			}
			return nil, fmt.Errorf("assembly: unexpected '.' at byte %d (only '...' is valid)", i)
		case c == 0xE2 && i+2 < len(s) && s[i+1] == 0x86 && s[i+2] == 0x92:
			// U+2192 RIGHTWARDS ARROW in UTF-8 = E2 86 92
			out = append(out, tok{tokArrow, "→", i, i + 3})
			i += 3
		case isIdentStart(c):
			j := i + 1
			for j < len(s) && isIdentCont(s[j]) {
				j++
			}
			out = append(out, tok{tokIdent, s[i:j], i, j})
			i = j
		default:
			return nil, fmt.Errorf("assembly: unexpected character %q at byte %d", c, i)
		}
	}
	out = append(out, tok{tokEOF, "", len(s), len(s)})
	return out, nil
}

func isIdentStart(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isIdentCont(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9') || c == '-' || c == '_'
}

// --- parser (recursive descent) ---

type parser struct {
	toks []tok
	src  string
	pos  int
}

func (p *parser) peek() tok {
	return p.toks[p.pos]
}

func (p *parser) peekAt(n int) tok {
	if p.pos+n >= len(p.toks) {
		return p.toks[len(p.toks)-1]
	}
	return p.toks[p.pos+n]
}

func (p *parser) next() tok {
	t := p.toks[p.pos]
	if p.pos < len(p.toks)-1 {
		p.pos++
	}
	return t
}

func (p *parser) parseTop() (Node, error) {
	first, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if p.peek().kind != tokArrow {
		return first, nil
	}
	args := []Node{first}
	startPos, _ := first.Span()
	endPos := startPos
	for p.peek().kind == tokArrow {
		p.next()
		next, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		args = append(args, next)
		_, endPos = next.Span()
	}
	return &OpNode{Op: "sequential", Args: args, NamedArgs: map[string]Node{}, Pos: startPos, End: endPos}, nil
}

func (p *parser) parseExpr() (Node, error) {
	t := p.peek()
	if t.kind != tokIdent {
		return nil, fmt.Errorf("assembly: expected identifier at byte %d, got %q", t.pos, t.val)
	}
	if p.peekAt(1).kind == tokLParen && IsOpName(t.val) {
		return p.parseOpCall()
	}
	return p.parseLeaf()
}

func (p *parser) parseLeaf() (Node, error) {
	t := p.next()
	if IsLexID(t.val) {
		return &AtomLeaf{ID: t.val, Pos: t.pos, End: t.end}, nil
	}
	return &MissingLeaf{Name: t.val, Pos: t.pos, End: t.end}, nil
}

func (p *parser) parseOpCall() (Node, error) {
	name := p.next()
	if p.peek().kind != tokLParen {
		return nil, fmt.Errorf("assembly: expected '(' after op %q at byte %d", name.val, p.peek().pos)
	}
	p.next() // (
	op := &OpNode{
		Op:        name.val,
		NamedArgs: map[string]Node{},
		Pos:       name.pos,
	}
	if p.peek().kind == tokRParen {
		end := p.next()
		op.End = end.end
		return op, nil
	}
	for {
		// named-arg lookahead: ident "=" ...
		if p.peek().kind == tokIdent && p.peekAt(1).kind == tokEq {
			keyTok := p.next()
			p.next() // =
			rhs, err := p.parseNamedRhs(keyTok.val)
			if err != nil {
				return nil, err
			}
			if _, dup := op.NamedArgs[keyTok.val]; dup {
				return nil, fmt.Errorf("assembly: duplicate named-arg %q at byte %d", keyTok.val, keyTok.pos)
			}
			op.NamedArgs[keyTok.val] = rhs
			op.NamedKeys = append(op.NamedKeys, keyTok.val)
		} else if p.peek().kind == tokEllipsis {
			t := p.next()
			op.Args = append(op.Args, &Ellipsis{Pos: t.pos, End: t.end})
		} else {
			arg, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			op.Args = append(op.Args, arg)
		}
		switch p.peek().kind {
		case tokComma, tokSemi:
			p.next()
			// Tolerate trailing separator before ')' — defensive but cheap.
			if p.peek().kind == tokRParen {
				end := p.next()
				op.End = end.end
				return op, nil
			}
			continue
		case tokRParen:
			end := p.next()
			op.End = end.end
			return op, nil
		default:
			t := p.peek()
			return nil, fmt.Errorf("assembly: expected ',' ';' or ')' at byte %d, got %q", t.pos, t.val)
		}
	}
}

func (p *parser) parseNamedRhs(key string) (Node, error) {
	if isPredicateNamedArg(key) {
		t := p.peek()
		if t.kind != tokIdent {
			return nil, fmt.Errorf("assembly: expected predicate identifier after %q= at byte %d", key, t.pos)
		}
		p.next()
		return &Predicate{Name: t.val, Pos: t.pos, End: t.end}, nil
	}
	return p.parseExpr()
}
