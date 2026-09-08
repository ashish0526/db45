package db

import (
	"errors"
	"strconv"
)

// parseValue parses a literal into out, dispatching on the first character —
// small fixed lookahead, no backtracking.
func (p *Parser) parseValue(out *Cell) error {
	p.skipSpaces()
	if p.pos >= len(p.buf) {
		return errors.New("expect value")
	}
	switch ch := p.buf[p.pos]; {
	case ch == '"' || ch == '\'':
		return p.parseString(out)
	case isDigit(ch) || ch == '-' || ch == '+':
		return p.parseInt(out)
	default:
		return errors.New("expect value")
	}
}

// parseInt parses an optional +/- sign followed by digits into a TypeI64 cell.
func (p *Parser) parseInt(out *Cell) error {
	p.skipSpaces()
	start := p.pos
	if p.pos < len(p.buf) && (p.buf[p.pos] == '+' || p.buf[p.pos] == '-') {
		p.pos++
	}
	digits := p.pos
	for p.pos < len(p.buf) && isDigit(p.buf[p.pos]) {
		p.pos++
	}
	if p.pos == digits {
		p.pos = start
		return errors.New("expect integer")
	}
	n, err := strconv.ParseInt(p.buf[start:p.pos], 10, 64)
	if err != nil {
		p.pos = start
		return err
	}
	*out = Cell{Type: TypeI64, I64: n}
	return nil
}

// parseString parses a quoted string into a TypeStr cell. It opens with ' or "
// and closes with the same quote; inside, \' \" \\ are escapes. Real SQL would
// also want \n, \xFF, unicode escapes — deliberately stopped here.
func (p *Parser) parseString(out *Cell) error {
	p.skipSpaces()
	if p.pos >= len(p.buf) {
		return errors.New("expect string")
	}
	quote := p.buf[p.pos]
	if quote != '\'' && quote != '"' {
		return errors.New("expect string")
	}
	start := p.pos
	p.pos++

	var sb []byte
	for p.pos < len(p.buf) {
		ch := p.buf[p.pos]
		switch {
		case ch == '\\':
			if p.pos+1 >= len(p.buf) {
				p.pos = start
				return errors.New("dangling escape")
			}
			esc := p.buf[p.pos+1]
			if esc != '\'' && esc != '"' && esc != '\\' {
				p.pos = start
				return errors.New("unknown escape")
			}
			sb = append(sb, esc)
			p.pos += 2
		case ch == quote:
			p.pos++
			*out = Cell{Type: TypeStr, Str: sb}
			return nil
		default:
			sb = append(sb, ch)
			p.pos++
		}
	}
	p.pos = start
	return errors.New("unterminated string")
}
