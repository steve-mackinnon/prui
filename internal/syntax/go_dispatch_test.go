package syntax

import (
	"math/rand"
	"reflect"
	regexsyntax "regexp/syntax"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

func TestASCIIFirstCandidates(t *testing.T) {
	for _, tc := range []struct {
		pattern      string
		first        rune
		match, empty bool
	}{
		{`abc`, 'a', true, false}, {`abc`, 'z', false, false}, {`a|bc`, 'b', true, false},
		{`a?b`, 'b', true, false}, {`a?b`, 'c', false, false}, {`a*b`, 'a', true, false},
		{`a+`, 'z', false, false}, {`a?`, 'z', false, true}, {`a{0}`, 'a', false, true},
		{`a{2,5}b`, 'a', true, false}, {`(ab)?`, 'a', true, true}, {`(?:|a){2}`, 'z', false, true},
		{`[A-Z]`, 'a', false, false}, {`[A-Z]`, 'Z', true, false}, {`[^\W\d]`, '_', true, false},
		{`[^\W\d]`, '3', false, false}, {`\s+`, ' ', true, false}, {`(?m:^\b)keyword`, 'k', true, false},
		{`(?m:^\b)keyword`, 'x', false, false}, {`.`, '\n', false, false}, {`(?s:.)`, '\n', true, false},
		{`[^\s\S]`, 'a', false, false}, {``, 'z', false, true}, {`(?i:k)`, 'K', true, false},
	} {
		t.Run(tc.pattern+string(tc.first), func(t *testing.T) {
			re, err := regexsyntax.Parse(tc.pattern, regexsyntax.Perl)
			if err != nil {
				t.Fatal(err)
			}
			match, empty := asciiFirst(re, tc.first)
			if match != tc.match || empty != tc.empty {
				t.Fatalf("first(%q,%q) = (%v,%v), want (%v,%v)", tc.pattern, tc.first, match, empty, tc.match, tc.empty)
			}
		})
	}
}

func TestGoDispatchTokenEquivalence(t *testing.T) {
	inputs := []string{
		"package p\nfunc f() { println(`{{.x}}`); /* multiline\ncomment */ }\n",
		"import (\"fmt\")\nvar x = int(4)\nconst Value = true\ntype T struct{}\n",
		"a\v b\n", "aé(1)\n", "λ_value(42)\n", "//not terminated", "`unfinished", "\x00\r\nλb()",
		"0b101_1 0xFF 077 0o77 1.5e-2 .5 3i 1.5e+2i 1e2i\n",
		"'x' '\\n' '\\u0041' \"escaped\\\"quote\" `{{if .x}}{{ .Name }}{{end}}`\n",
		"func\t\nname ( ) { a <<= 2; b &= 4; c &^= 8; x := a... }\n",
	}
	for c := range 128 {
		inputs = append(inputs, string(rune(c))+"name(0x2) // tail\n")
	}
	rng := rand.New(rand.NewSource(1))
	alphabet := []rune("abcdefghijklmnopqrstuvwxyz_()0123456789;/'\"`=+*-{}[]! \\\n\t\r\vλé\x00")
	for range 1000 {
		r := make([]rune, rng.Intn(200))
		for i := range r {
			r[i] = alphabet[rng.Intn(len(alphabet))]
		}
		inputs = append(inputs, string(r))
	}
	optimized := tokenLexer(lexers.Go)
	if optimized == lexers.Go {
		t.Fatal("pinned Go optimization inactive")
	}
	for _, input := range inputs {
		compareGoTokens(t, optimized, input, false)
		compareGoTokens(t, optimized, input, true)
	}
}

func compareGoTokens(t testing.TB, optimized chroma.Lexer, input string, nested bool) {
	t.Helper()
	options := &chroma.TokeniseOptions{State: "root", EnsureLF: false, Nested: nested}
	want, err := chroma.Tokenise(lexers.Go, options, input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := chroma.Tokenise(optimized, options, input)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Go tokens differ for %q\ngot %v\nwant %v", input, got, want)
	}
}

func FuzzGoDispatchTokens(f *testing.F) {
	for _, input := range []string{"", "package p\nfunc f() { println(`{{.x}}`) }\n", "\vλé\x00", "/* unfinished", "// no newline", "a\n(1)"} {
		f.Add(input)
	}
	optimized := tokenLexer(lexers.Go)
	f.Fuzz(func(t *testing.T, input string) {
		if len(input) > 512 || !utf8.ValidString(input) {
			return
		}
		compareGoTokens(t, optimized, input, false)
	})
}

func TestGoDispatchFallbackAndOriginalIsolation(t *testing.T) {
	original := lexers.Go.(*chroma.RegexLexer)
	originalRules, err := original.Rules()
	if err != nil {
		t.Fatal(err)
	}
	config := *original.Config()
	count := len(lexers.GlobalLexerRegistry.Lexers)
	for _, reason := range []string{"pattern", "mutator", "state", "case", "dotall", "multiline", "name"} {
		t.Run(reason, func(t *testing.T) {
			rules := originalRules.Clone()
			local := config
			switch reason {
			case "pattern":
				rules["root"][0].Pattern = "other"
			case "mutator":
				rules["root"][0].Mutator = chroma.Push("root")
			case "state":
				rules["other"] = []chroma.Rule{{Pattern: "x", Type: chroma.Text}}
			case "case":
				local.CaseInsensitive = true
			case "dotall":
				local.DotAll = true
			case "multiline":
				local.NotMultiline = true
			case "name":
				local.Name = "Other"
			}
			custom, err := chroma.NewLexer(&local, func() chroma.Rules { return rules })
			if err != nil {
				t.Fatal(err)
			}
			if buildGoDispatcher(custom) != custom {
				t.Fatal("unsupported Go shape optimized")
			}
			if tokenLexer(custom) != custom {
				t.Fatal("custom lexer replaced")
			}
		})
	}
	optimized := tokenLexer(original).(*chroma.RegexLexer)
	if optimized == original {
		t.Fatal("pinned Go optimization inactive")
	}
	fastRules, err := optimized.Rules()
	if err != nil {
		t.Fatal(err)
	}
	compiledCount := 0
	for _, rules := range fastRules {
		compiledCount += len(rules)
	}
	if compiledCount != len(originalRules["root"])+2 || len(fastRules) > 32 {
		t.Fatal("compiled regex/state bound changed")
	}
	for _, rule := range originalRules["root"] {
		if rule.Mutator != nil {
			t.Fatal("original Go rules mutated")
		}
	}
	if !reflect.DeepEqual(*original.Config(), config) || len(lexers.GlobalLexerRegistry.Lexers) != count {
		t.Fatal("original registry or config changed")
	}
	if lexer := lexers.Get("python"); tokenLexer(lexer) != lexer {
		t.Fatal("other language replaced")
	}
}

func TestGoDispatchCompiledSharing(t *testing.T) {
	root := []*chroma.CompiledRule{{Rule: chroma.Rule{Pattern: "dispatcher"}}, {Rule: chroma.Rule{Pattern: "a"}}, {Rule: chroma.Rule{Pattern: "b"}}}
	fallback := &chroma.CompiledRule{Rule: chroma.Rule{Pattern: "fallback"}}
	rules := chroma.CompiledRules{"root": root, "unmatched": {fallback}}
	installGoDispatch(rules, map[string]uint64{"none": 0, "a": 1, "both": 3})
	for name, want := range map[string][]*chroma.CompiledRule{"none": {fallback}, "a": {root[1], fallback}, "both": {root[1], root[2], fallback}} {
		got := rules[name]
		if len(got) != len(want) {
			t.Fatal("wrong candidate count")
		}
		for i := range got {
			if got[i] != want[i] {
				t.Fatal("rule copied, reordered or omitted")
			}
		}
	}
	if len(rules["root"]) != 3 || rules["root"][0] != root[0] {
		t.Fatal("root changed")
	}
}

func TestGoDispatchConcurrentCapture(t *testing.T) {
	input := "package p\nfunc f() { println(`{{.x}}`) }\n"
	var workers sync.WaitGroup
	for range 16 {
		workers.Go(func() {
			for range 20 {
				compareGoTokens(t, tokenLexer(lexers.Go), input, false)
			}
		})
	}
	workers.Wait()
}
