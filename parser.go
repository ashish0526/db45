package db

import "strings"

// Parser is a hand-written recursive-descent SQL parser. It holds the input and a
// cursor; every parsing method both reads and advances p. The "look at the next
// one or two tokens and recurse" style needs no compiler theory.
type Parser struct {
	buf string
	pos int
}

// NewParser starts a parser over a SQL string.
func NewParser(sql string) *Parser { return &Parser{buf: sql} }

// --- byte-level ASCII classifiers -------------------------------------------------

func isSpace(ch byte) bool {
	switch ch {
	case '\t', '\n', '\v', '\f', '\r', ' ':
		return true
	}
	return false
}
func isAlpha(ch byte) bool        { return 'a' <= (ch|32) && (ch|32) <= 'z' } // ch|32 lowercases ASCII
func isDigit(ch byte) bool        { return '0' <= ch && ch <= '9' }
func isNameStart(ch byte) bool    { return isAlpha(ch) || ch == '_' }
func isNameContinue(ch byte) bool { return isAlpha(ch) || isDigit(ch) || ch == '_' }

// --- primitives ----------------------------------------------------------------
//
// Every primitive follows the same shape: skip spaces, look at buf[pos:], and
// either consume (advance pos, return the token / true) or leave pos untouched
// and return false. "No side effect on failure" is what lets a caller try one
// rule, then another.

func (p *Parser) skipSpaces() {
	for p.pos < len(p.buf) && isSpace(p.buf[p.pos]) {
		p.pos++
	}
}

// tryName consumes an identifier: NameStart then NameContinue*.
func (p *Parser) tryName() (string, bool) {
	p.skipSpaces()
	start := p.pos
	if start >= len(p.buf) || !isNameStart(p.buf[start]) {
		return "", false
	}
	p.pos++
	for p.pos < len(p.buf) && isNameContinue(p.buf[p.pos]) {
		p.pos++
	}
	return p.buf[start:p.pos], true
}

// tryKeyword matches one or more keywords in sequence, case-insensitively. A
// keyword match also requires the following character to be a separator, so
// tryKeyword("in") does not match the start of "into". Rewinds fully on any miss.
func (p *Parser) tryKeyword(kws ...string) bool {
	saved := p.pos
	for _, kw := range kws {
		p.skipSpaces()
		end := p.pos + len(kw)
		if end > len(p.buf) || !strings.EqualFold(p.buf[p.pos:end], kw) {
			p.pos = saved
			return false
		}
		if end < len(p.buf) && isNameContinue(p.buf[end]) {
			p.pos = saved
			return false
		}
		p.pos = end
	}
	return true
}

// tryPunctuation matches an exact punctuation string like "=" "," "(" "<=".
func (p *Parser) tryPunctuation(pun string) bool {
	p.skipSpaces()
	end := p.pos + len(pun)
	if end > len(p.buf) || p.buf[p.pos:end] != pun {
		return false
	}
	p.pos = end
	return true
}

// atEnd reports whether only whitespace (and an optional trailing ';') remains.
func (p *Parser) atEnd() bool {
	p.skipSpaces()
	if p.pos < len(p.buf) && p.buf[p.pos] == ';' {
		p.pos++
		p.skipSpaces()
	}
	return p.pos >= len(p.buf)
}
