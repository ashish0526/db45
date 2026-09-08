package db

import "errors"

// parseEqual parses `column = value` into out.
func (p *Parser) parseEqual(out *NamedCell) error {
	var ok bool
	if out.column, ok = p.tryName(); !ok {
		return errors.New("expect column")
	}
	if !p.tryPunctuation("=") {
		return errors.New("expect =")
	}
	return p.parseValue(&out.value)
}

// parseWhere parses an optional `WHERE col = val AND col = val ...` — the same
// item/separator loop as a comma list, with ',' swapped for AND.
func (p *Parser) parseWhere(out *[]NamedCell) error {
	if !p.tryKeyword("WHERE") {
		return nil
	}
	for {
		var nc NamedCell
		if err := p.parseEqual(&nc); err != nil {
			return err
		}
		*out = append(*out, nc)
		if !p.tryKeyword("AND") {
			return nil
		}
	}
}

// parseSelect parses the body of a SELECT (parseStmt has consumed no keyword yet
// in this step, so parseSelect consumes "SELECT" itself).
func (p *Parser) parseSelect(out *StmtSelect) error {
	if !p.tryKeyword("SELECT") {
		return errors.New("expect keyword SELECT")
	}
	for !p.tryKeyword("FROM") {
		if len(out.cols) > 0 && !p.tryPunctuation(",") {
			return errors.New("expect comma")
		}
		if name, ok := p.tryName(); ok {
			out.cols = append(out.cols, name)
		} else {
			return errors.New("expect column")
		}
	}
	if len(out.cols) == 0 {
		return errors.New("expect column list")
	}
	var ok bool
	if out.table, ok = p.tryName(); !ok {
		return errors.New("expect table name")
	}
	return p.parseWhere(&out.keys)
}
