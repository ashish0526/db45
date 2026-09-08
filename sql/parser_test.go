package sql

import "testing"

func TestTryName(t *testing.T) {
	p := NewParser("  hello_world1 from")
	name, ok := p.tryName()
	if !ok || name != "hello_world1" {
		t.Fatalf("tryName: %q ok=%v", name, ok)
	}
	if !p.tryKeyword("from") {
		t.Fatalf("expected 'from' next, pos=%d", p.pos)
	}

	p = NewParser("123abc")
	if _, ok := p.tryName(); ok {
		t.Fatal("tryName matched a digit start")
	}
	if p.pos != 0 {
		t.Fatalf("failed tryName advanced pos to %d", p.pos)
	}
}

func TestTryKeyword(t *testing.T) {
	// case-insensitive, multi-word
	p := NewParser("Create   TABLE t")
	if !p.tryKeyword("CREATE", "table") {
		t.Fatal("multi-word keyword match failed")
	}
	if name, _ := p.tryName(); name != "t" {
		t.Fatalf("after keywords, got %q", name)
	}

	// "in" must not match inside "into"
	p = NewParser("into")
	if p.tryKeyword("in") {
		t.Fatal("'in' wrongly matched the start of 'into'")
	}
	if p.pos != 0 {
		t.Fatalf("failed keyword advanced pos to %d", p.pos)
	}
	if !p.tryKeyword("into") {
		t.Fatal("'into' should match")
	}
}

func TestTryPunctuation(t *testing.T) {
	p := NewParser("a <= b")
	p.tryName()
	if !p.tryPunctuation("<=") {
		t.Fatal("'<=' not matched")
	}
	if p.tryPunctuation("=") {
		t.Fatal("'=' matched where 'b' is")
	}
}
