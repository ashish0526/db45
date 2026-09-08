package db

import "errors"

// An expression is a tree. Node kinds, distinguished via interface{}:
//
//	string      — a column reference
//	*Cell       — a literal constant
//	*ExprBinOp  — a binary operator node
//	*ExprUnOp   — a unary operator node (NOT, unary minus)
//	*ExprTuple  — a parenthesised list (Step 0507)
//
// Evaluation is a post-order walk: evaluate children, combine at the node.
type ExprBinOp struct {
	op    ExprOp
	left  interface{}
	right interface{}
}

type ExprUnOp struct {
	op  ExprOp
	kid interface{}
}

// The precedence ladder, lowest binding at the top of the call chain:
//
//	OR · AND · NOT · (= != <> < > <= >=) · (+ -) · (* /) · unary -
//
// Each binary level is the same loop over a (token, op) table — the readable
// cousin of a Pratt parser. Adding a level = adding a function in the chain.
func (p *Parser) parseExpr() (interface{}, error) { return p.parseOr() }

// binToken pairs a matcher (keyword or punctuation) with the op it produces.
type binToken struct {
	kw  string // matched with tryKeyword if non-empty
	pun string // matched with tryPunctuation otherwise
	op  ExprOp
}

func (p *Parser) tryBinToken(t binToken) bool {
	if t.kw != "" {
		return p.tryKeyword(t.kw)
	}
	return p.tryPunctuation(t.pun)
}

// parseBinop parses `inner ((token) inner)*`, left-associative.
func (p *Parser) parseBinop(toks []binToken, inner func() (interface{}, error)) (interface{}, error) {
	left, err := inner()
	if err != nil {
		return nil, err
	}
	for {
		matched := false
		for _, t := range toks {
			if p.tryBinToken(t) {
				right, err := inner()
				if err != nil {
					return nil, err
				}
				left = &ExprBinOp{op: t.op, left: left, right: right}
				matched = true
				break
			}
		}
		if !matched {
			return left, nil
		}
	}
}

func (p *Parser) parseOr() (interface{}, error) {
	return p.parseBinop([]binToken{{kw: "OR", op: OP_OR}}, p.parseAnd)
}

func (p *Parser) parseAnd() (interface{}, error) {
	return p.parseBinop([]binToken{{kw: "AND", op: OP_AND}}, p.parseNot)
}

// parseNot is unary and may nest: NOT NOT a.
func (p *Parser) parseNot() (interface{}, error) {
	if p.tryKeyword("NOT") {
		kid, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		return &ExprUnOp{op: OP_NOT, kid: kid}, nil
	}
	return p.parseCmp()
}

func (p *Parser) parseCmp() (interface{}, error) {
	return p.parseBinop([]binToken{
		{pun: "<=", op: OP_LE}, {pun: ">=", op: OP_GE},
		{pun: "<>", op: OP_NE}, {pun: "!=", op: OP_NE},
		{pun: "=", op: OP_EQ},
		{pun: "<", op: OP_LT}, {pun: ">", op: OP_GT},
	}, p.parseAdd)
}

func (p *Parser) parseAdd() (interface{}, error) {
	return p.parseBinop([]binToken{{pun: "+", op: OP_ADD}, {pun: "-", op: OP_SUB}}, p.parseMul)
}

func (p *Parser) parseMul() (interface{}, error) {
	return p.parseBinop([]binToken{{pun: "*", op: OP_MUL}, {pun: "/", op: OP_DIV}}, p.parseNeg)
}

// parseNeg is unary minus; it may nest.
func (p *Parser) parseNeg() (interface{}, error) {
	if p.tryPunctuation("-") {
		kid, err := p.parseNeg()
		if err != nil {
			return nil, err
		}
		return &ExprUnOp{op: OP_NEG, kid: kid}, nil
	}
	return p.parseAtom()
}

// parseAtom parses a parenthesised expression (recurse to the top — grouping),
// a column name, or a literal.
func (p *Parser) parseAtom() (interface{}, error) {
	if p.tryPunctuation("(") {
		inner, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if !p.tryPunctuation(")") {
			return nil, errors.New("expect )")
		}
		return inner, nil
	}
	if name, ok := p.tryName(); ok {
		return name, nil
	}
	cell := &Cell{}
	if err := p.parseValue(cell); err != nil {
		return nil, errors.New("expect column, value or (")
	}
	return cell, nil
}
