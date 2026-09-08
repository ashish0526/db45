package db

import "errors"

// An expression is a tree. Node kinds, distinguished via interface{}:
//
//	string     — a column reference
//	*Cell      — a literal constant
//	*ExprBinOp — an operator node (later also *ExprUnOp, *ExprTuple)
//
// Evaluation is a post-order walk: evaluate children, combine at the node.
type ExprBinOp struct {
	op    ExprOp
	left  interface{}
	right interface{}
}

// Precedence is one parse function per level, lowest at the top of the call
// chain: parseExpr -> parseAdd -> parseMul -> parseAtom. Because parseMul runs
// inside each operand of parseAdd, any '*'/'/' sub-expression ends up below the
// '+'/'-' node — "multiplication binds tighter". Adding a level = adding a
// function in the chain.
func (p *Parser) parseExpr() (interface{}, error) { return p.parseAdd() }

// parseBinop parses `inner ((tok) inner)*` for a set of same-precedence
// operators, left-associative.
func (p *Parser) parseBinop(toks []string, ops []ExprOp, inner func() (interface{}, error)) (interface{}, error) {
	left, err := inner()
	if err != nil {
		return nil, err
	}
	for {
		matched := false
		for i, tok := range toks {
			if p.tryPunctuation(tok) {
				right, err := inner()
				if err != nil {
					return nil, err
				}
				left = &ExprBinOp{op: ops[i], left: left, right: right}
				matched = true
				break
			}
		}
		if !matched {
			return left, nil
		}
	}
}

func (p *Parser) parseAdd() (interface{}, error) {
	return p.parseBinop([]string{"+", "-"}, []ExprOp{OP_ADD, OP_SUB}, p.parseMul)
}

func (p *Parser) parseMul() (interface{}, error) {
	return p.parseBinop([]string{"*", "/"}, []ExprOp{OP_MUL, OP_DIV}, p.parseAtom)
}

// parseAtom parses the smallest expression: a parenthesised expression (recurse
// to the top — "loop back from the bottom" is how grouping works), a column
// name, or a literal.
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
