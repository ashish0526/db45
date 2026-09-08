package db

import "errors"

// An expression is a tree. Three node kinds, distinguished via interface{}:
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

// parseAtom parses the smallest expression: a column name or a literal.
func (p *Parser) parseAtom() (interface{}, error) {
	if name, ok := p.tryName(); ok {
		return name, nil
	}
	cell := &Cell{}
	if err := p.parseValue(cell); err != nil {
		return nil, errors.New("expect column or value")
	}
	return cell, nil
}

// parseAdd parses `atom (('+'|'-') atom)*`, folding each new term into the left
// side — a for loop that yields left-associative ((a-b)-c), the correct
// associativity for '-'. Precedence (so '*' binds tighter) arrives in Step 0503.
func (p *Parser) parseAdd() (interface{}, error) {
	left, err := p.parseAtom()
	if err != nil {
		return nil, err
	}
	for {
		var op ExprOp
		switch {
		case p.tryPunctuation("+"):
			op = OP_ADD
		case p.tryPunctuation("-"):
			op = OP_SUB
		default:
			return left, nil
		}
		right, err := p.parseAtom()
		if err != nil {
			return nil, err
		}
		left = &ExprBinOp{op: op, left: left, right: right}
	}
}
