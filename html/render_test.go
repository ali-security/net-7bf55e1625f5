// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package html

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestRenderer(t *testing.T) {
	nodes := [...]*Node{
		0: {
			Type: ElementNode,
			Data: "html",
		},
		1: {
			Type: ElementNode,
			Data: "head",
		},
		2: {
			Type: ElementNode,
			Data: "body",
		},
		3: {
			Type: TextNode,
			Data: "0<1",
		},
		4: {
			Type: ElementNode,
			Data: "p",
			Attr: []Attribute{
				{
					Key: "id",
					Val: "A",
				},
				{
					Key: "foo",
					Val: `abc"def`,
				},
			},
		},
		5: {
			Type: TextNode,
			Data: "2",
		},
		6: {
			Type: ElementNode,
			Data: "b",
			Attr: []Attribute{
				{
					Key: "empty",
					Val: "",
				},
			},
		},
		7: {
			Type: TextNode,
			Data: "3",
		},
		8: {
			Type: ElementNode,
			Data: "i",
			Attr: []Attribute{
				{
					Key: "backslash",
					Val: `\`,
				},
			},
		},
		9: {
			Type: TextNode,
			Data: "&4",
		},
		10: {
			Type: TextNode,
			Data: "5",
		},
		11: {
			Type: ElementNode,
			Data: "blockquote",
		},
		12: {
			Type: ElementNode,
			Data: "br",
		},
		13: {
			Type: TextNode,
			Data: "6",
		},
		14: {
			Type: CommentNode,
			Data: "comm",
		},
		15: {
			Type: CommentNode,
			Data: "x-->y", // Needs escaping.
		},
		16: {
			Type: RawNode,
			Data: "7<pre>8</pre>9",
		},
	}

	// Build a tree out of those nodes, based on a textual representation.
	// Only the ".\t"s are significant. The trailing HTML-like text is
	// just commentary. The "0:" prefixes are for easy cross-reference with
	// the nodes array.
	treeAsText := [...]string{
		0:  `<html>`,
		1:  `.	<head>`,
		2:  `.	<body>`,
		3:  `.	.	"0&lt;1"`,
		4:  `.	.	<p id="A" foo="abc&#34;def">`,
		5:  `.	.	.	"2"`,
		6:  `.	.	.	<b empty="">`,
		7:  `.	.	.	.	"3"`,
		8:  `.	.	.	<i backslash="\">`,
		9:  `.	.	.	.	"&amp;4"`,
		10: `.	.	"5"`,
		11: `.	.	<blockquote>`,
		12: `.	.	<br>`,
		13: `.	.	"6"`,
		14: `.	.	"<!--comm-->"`,
		15: `.	.	"<!--x--&gt;y-->"`,
		16: `.	.	"7<pre>8</pre>9"`,
	}
	if len(nodes) != len(treeAsText) {
		t.Fatal("len(nodes) != len(treeAsText)")
	}
	var stack [8]*Node
	for i, line := range treeAsText {
		level := 0
		for line[0] == '.' {
			// Strip a leading ".\t".
			line = line[2:]
			level++
		}
		n := nodes[i]
		if level == 0 {
			if stack[0] != nil {
				t.Fatal("multiple root nodes")
			}
			stack[0] = n
		} else {
			stack[level-1].AppendChild(n)
			stack[level] = n
			for i := level + 1; i < len(stack); i++ {
				stack[i] = nil
			}
		}
		// At each stage of tree construction, we check all nodes for consistency.
		for j, m := range nodes {
			if err := checkNodeConsistency(m); err != nil {
				t.Fatalf("i=%d, j=%d: %v", i, j, err)
			}
		}
	}

	want := `<html><head></head><body>0&lt;1<p id="A" foo="abc&#34;def">` +
		`2<b empty="">3</b><i backslash="\">&amp;4</i></p>` +
		`5<blockquote></blockquote><br/>6<!--comm--><!--x--&gt;y-->7<pre>8</pre>9</body></html>`
	b := new(bytes.Buffer)
	if err := Render(b, nodes[0]); err != nil {
		t.Fatal(err)
	}
	if got := b.String(); got != want {
		t.Errorf("got vs want:\n%s\n%s\n", got, want)
	}
}

// Parsing unescapes character references, so a ">" can end up in a doctype's
// PUBLIC or SYSTEM identifier. Rendering it literally would trigger an
// abrupt-doctype-system-identifier parse error, which emits the doctype token
// early and continues in the data state, so the remainder of the identifier
// becomes markup.
func TestRenderDoctypeIdentifiers(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
	}{
		// SYSTEM only.
		{
			in:   `<!DOCTYPE html SYSTEM "&gt;&lt;script&gt;alert(1)&lt;/script&gt;">`,
			want: `<!DOCTYPE html SYSTEM "&gt;<script&gt;alert(1)</script&gt;">`,
		},
		// PUBLIC, with and without a following SYSTEM identifier.
		{
			in:   `<!DOCTYPE html PUBLIC "&gt;&lt;script&gt;alert(1)&lt;/script&gt;">`,
			want: `<!DOCTYPE html PUBLIC "&gt;<script&gt;alert(1)</script&gt;">`,
		},
		{
			in:   `<!DOCTYPE html PUBLIC "&gt;&lt;script&gt;alert(1)&lt;/script&gt;" "&gt;">`,
			want: `<!DOCTYPE html PUBLIC "&gt;<script&gt;alert(1)</script&gt;" "&gt;">`,
		},
		// An identifier containing a double quote is written with single
		// quotes; the ">" still needs escaping.
		{
			in:   `<!DOCTYPE html SYSTEM '&gt;"&lt;script&gt;alert(1)&lt;/script&gt;'>`,
			want: `<!DOCTYPE html SYSTEM '&gt;"<script&gt;alert(1)</script&gt;'>`,
		},
	} {
		doc, err := Parse(strings.NewReader(tc.in))
		if err != nil {
			t.Fatal(err)
		}
		b := new(bytes.Buffer)
		if err := Render(b, doc); err != nil {
			t.Fatal(err)
		}
		want := tc.want + `<html><head></head><body></body></html>`
		if got := b.String(); got != want {
			t.Errorf("%s: got vs want:\n%s\n%s\n", tc.in, got, want)
			continue
		}

		// Re-parsing the rendered output must not turn the identifier into
		// markup, and must render back to the same bytes.
		doc1, err := Parse(strings.NewReader(b.String()))
		if err != nil {
			t.Fatal(err)
		}
		if hasElement(doc1, "script") {
			t.Errorf("%s: doctype identifier escaped into a <script> element", tc.in)
		}
		b1 := new(bytes.Buffer)
		if err := Render(b1, doc1); err != nil {
			t.Fatal(err)
		}
		if b1.String() != b.String() {
			t.Errorf("%s: rendering is not idempotent:\n%s\n%s\n", tc.in, b.String(), b1.String())
		}
	}
}

// parseDoctype never produces an identifier containing both quote types, but a
// programmatically constructed Node can. Such a Node cannot be rendered
// unambiguously, so rendering it must fail rather than emit an identifier that
// terminates early.
func TestRenderDoctypeBothQuoteTypes(t *testing.T) {
	for _, key := range []string{"public", "system"} {
		n := &Node{
			Type: DoctypeNode,
			Data: "html",
			Attr: []Attribute{{Key: key, Val: `"'><script>alert(1)</script>`}},
		}
		b := new(bytes.Buffer)
		if err := Render(b, n); err == nil {
			t.Errorf("%s: rendered %q with no error, want error", key, b.String())
		}
	}
}

func hasElement(n *Node, name string) bool {
	if n.Type == ElementNode && n.Data == name {
		return true
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if hasElement(c, name) {
			return true
		}
	}
	return false
}

func TestRenderTextNodes(t *testing.T) {
	elements := []string{"style", "script", "xmp", "iframe", "noembed", "noframes", "plaintext", "noscript"}
	for _, namespace := range []string{
		"", // html
		"svg",
		"math",
	} {
		for _, e := range elements {
			var namespaceOpen, namespaceClose string
			if namespace != "" {
				namespaceOpen, namespaceClose = fmt.Sprintf("<%s>", namespace), fmt.Sprintf("</%s>", namespace)
			}
			doc := fmt.Sprintf(`<html><head></head><body>%s<%s>&</%s>%s</body></html>`, namespaceOpen, e, e, namespaceClose)
			n, err := Parse(strings.NewReader(doc))
			if err != nil {
				t.Fatal(err)
			}
			b := bytes.NewBuffer(nil)
			if err := Render(b, n); err != nil {
				t.Fatal(err)
			}

			expected := doc
			if namespace != "" {
				expected = strings.Replace(expected, "&", "&amp;", 1)
			}

			if b.String() != expected {
				t.Errorf("unexpected output: got %q, want %q", b.String(), expected)
			}
		}
	}
}
