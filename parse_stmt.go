package db

import "errors"

// ParseStmt parses one complete SQL statement. It consumes the leading
// keyword(s) and dispatches; the per-statement parsers must not re-consume them.
func (p *Parser) ParseStmt() (interface{}, error) {
	var out interface{}
	var err error
	switch {
	case p.tryKeyword("SELECT"):
		s := &StmtSelect{}
		err, out = p.parseSelect(s), s
	case p.tryKeyword("CREATE", "TABLE"):
		s := &StmtCreatTable{}
		err, out = p.parseCreateTable(s), s
	case p.tryKeyword("INSERT", "INTO"):
		s := &StmtInsert{}
		err, out = p.parseInsert(s), s
	case p.tryKeyword("UPDATE"):
		s := &StmtUpdate{}
		err, out = p.parseUpdate(s), s
	case p.tryKeyword("DELETE", "FROM"):
		s := &StmtDelete{}
		err, out = p.parseDelete(s), s
	default:
		return nil, errors.New("unknown statement")
	}
	if err != nil {
		return nil, err
	}
	if !p.atEnd() {
		return nil, errors.New("trailing tokens after statement")
	}
	return out, nil
}

func (p *Parser) tableName(dst *string) error {
	name, ok := p.tryName()
	if !ok {
		return errors.New("expect table name")
	}
	*dst = name
	return nil
}

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

// parseWhere parses an optional `WHERE col = val AND col = val ...`.
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

// commaList calls item until a ',' no longer follows. Requires at least one.
func (p *Parser) commaList(item func() error) error {
	for {
		if err := item(); err != nil {
			return err
		}
		if !p.tryPunctuation(",") {
			return nil
		}
	}
}

// parseSelect parses the body after the SELECT keyword. Output columns are now
// full expressions (parseExpr), not bare names.
func (p *Parser) parseSelect(out *StmtSelect) error {
	for !p.tryKeyword("FROM") {
		if len(out.cols) > 0 && !p.tryPunctuation(",") {
			return errors.New("expect comma")
		}
		expr, err := p.parseExpr()
		if err != nil {
			return err
		}
		out.cols = append(out.cols, expr)
	}
	if len(out.cols) == 0 {
		return errors.New("expect column list")
	}
	if err := p.tableName(&out.table); err != nil {
		return err
	}
	return p.parseWhere(&out.keys)
}

// parseCreateTable parses `t (a int64, b string, primary key (a, b))`.
func (p *Parser) parseCreateTable(out *StmtCreatTable) error {
	if err := p.tableName(&out.table); err != nil {
		return err
	}
	if !p.tryPunctuation("(") {
		return errors.New("expect (")
	}
	err := p.commaList(func() error {
		if p.tryKeyword("PRIMARY", "KEY") {
			if !p.tryPunctuation("(") {
				return errors.New("expect ( after primary key")
			}
			e := p.commaList(func() error {
				name, ok := p.tryName()
				if !ok {
					return errors.New("expect pkey column")
				}
				out.pkey = append(out.pkey, name)
				return nil
			})
			if e != nil {
				return e
			}
			if !p.tryPunctuation(")") {
				return errors.New("expect ) after primary key list")
			}
			return nil
		}
		name, ok := p.tryName()
		if !ok {
			return errors.New("expect column name")
		}
		var typ CellType
		switch {
		case p.tryKeyword("int64"):
			typ = TypeI64
		case p.tryKeyword("string"):
			typ = TypeStr
		default:
			return errors.New("expect column type (int64 | string)")
		}
		out.cols = append(out.cols, Column{Name: name, Type: typ})
		return nil
	})
	if err != nil {
		return err
	}
	if !p.tryPunctuation(")") {
		return errors.New("expect )")
	}
	if len(out.cols) == 0 {
		return errors.New("table has no columns")
	}
	if len(out.pkey) == 0 {
		return errors.New("table has no primary key")
	}
	return nil
}

// parseInsert parses `t values (1, 'x')`.
func (p *Parser) parseInsert(out *StmtInsert) error {
	if err := p.tableName(&out.table); err != nil {
		return err
	}
	if !p.tryKeyword("VALUES") {
		return errors.New("expect VALUES")
	}
	if !p.tryPunctuation("(") {
		return errors.New("expect (")
	}
	err := p.commaList(func() error {
		var c Cell
		if err := p.parseValue(&c); err != nil {
			return err
		}
		out.value = append(out.value, c)
		return nil
	})
	if err != nil {
		return err
	}
	if !p.tryPunctuation(")") {
		return errors.New("expect )")
	}
	return nil
}

// parseUpdate parses `t set a=1, b='x' where k=2`.
func (p *Parser) parseUpdate(out *StmtUpdate) error {
	if err := p.tableName(&out.table); err != nil {
		return err
	}
	if !p.tryKeyword("SET") {
		return errors.New("expect SET")
	}
	err := p.commaList(func() error {
		name, ok := p.tryName()
		if !ok {
			return errors.New("expect column")
		}
		if !p.tryPunctuation("=") {
			return errors.New("expect =")
		}
		expr, err := p.parseExpr()
		if err != nil {
			return err
		}
		out.value = append(out.value, ExprAssign{column: name, expr: expr})
		return nil
	})
	if err != nil {
		return err
	}
	return p.parseWhere(&out.keys)
}

// parseDelete parses `t where k=2` (the DELETE FROM keywords are already eaten).
func (p *Parser) parseDelete(out *StmtDelete) error {
	if err := p.tableName(&out.table); err != nil {
		return err
	}
	return p.parseWhere(&out.keys)
}
