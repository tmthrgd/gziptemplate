gziptemplate
============

Simple and fast gzipped template engine for Go.

Take a look at [quicktemplate](https://github.com/valyala/quicktemplate) if you  need fast yet powerful html template engine.

*Please note that gziptemplate doesn't do any escaping on template values
unlike [html/template](http://golang.org/pkg/html/template/) do. So values
must be properly escaped before passing them to gziptemplate.*

gziptemplate is faster than [text/template](http://golang.org/pkg/text/template/),
[strings.Replace](http://golang.org/pkg/strings/#Replace),
[strings.Replacer](http://golang.org/pkg/strings/#Replacer)
and [fmt.Fprintf](https://golang.org/pkg/fmt/#Fprintf) on placeholders' substitution.

Below are benchmark* results comparing gziptemplate performance to text/template,
strings.Replace, strings.Replacer and fmt.Fprintf:

```
$ go test -bench=. -benchmem
PASS
BenchmarkFmtFprintf-8                           10000000               215 ns/op               0 B/op          0 allocs/op
BenchmarkStringsReplace-8                        2000000               791 ns/op            1952 B/op         14 allocs/op
BenchmarkStringsReplacer-8                       2000000               892 ns/op            2256 B/op         23 allocs/op
BenchmarkTextTemplate-8                          1000000              1111 ns/op             336 B/op         19 allocs/op
BenchmarkFastTemplateExecuteFunc-8              20000000                93.5 ns/op             0 B/op          0 allocs/op
BenchmarkFastTemplateExecute-8                  10000000               122 ns/op              16 B/op          1 allocs/op
BenchmarkFastTemplateExecuteFuncString-8        10000000               174 ns/op             176 B/op          2 allocs/op
BenchmarkFastTemplateExecuteString-8            10000000               181 ns/op             176 B/op          2 allocs/op
BenchmarkFastTemplateExecuteTagFunc-8            5000000               262 ns/op             160 B/op          4 allocs/op
```

*Benchmarks do not yet cover gzip compression.


Docs
====

See http://godoc.org/github.com/tmthrgd/gziptemplate.


Usage
=====

```go
	template := "http://{{host}}/?q={{query}}&foo={{bar}}{{bar}}"
	t := gziptemplate.New(template, "{{", "}}")
	s := t.ExecuteString(map[string]interface{}{
		"host":  "google.com",
		"query": url.QueryEscape("hello=world"),
		"bar":   "foobar",
	})
	s = mustDecompress(s)
	fmt.Printf("%s", s)

	// Output:
	// http://google.com/?q=hello%3Dworld&foo=foobarfoobar
```


Advanced usage
==============

```go
	template := "Hello, [user]! You won [prize]!!! [foobar]"
	t, err := gziptemplate.NewTemplate(template, "[", "]")
	if err != nil {
		log.Fatalf("unexpected error when parsing template: %s", err)
	}
	s := t.ExecuteFuncString(func(w io.Writer, tag string) (int, error) {
		switch tag {
		case "user":
			return w.Write([]byte("John"))
		case "prize":
			return w.Write([]byte("$100500"))
		default:
			return w.Write([]byte(fmt.Sprintf("[unknown tag %q]", tag)))
		}
	})
	s = mustDecompress(s)
	fmt.Printf("%s", s)

	// Output:
	// Hello, John! You won $100500!!! [unknown tag "foobar"]
```
