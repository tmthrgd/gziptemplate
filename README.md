gziptemplate
============

[![Build Status](https://travis-ci.com/tmthrgd/gziptemplate.svg?token=zvBahcneBzztKy9scr2f&branch=master)](https://travis-ci.com/tmthrgd/gziptemplate)

Simple and fast gzipped template engine for Go.

Take a look at [quicktemplate](https://github.com/valyala/quicktemplate) if you  need fast yet powerful html template engine.

*Please note that gziptemplate doesn't do any escaping on template values
unlike [html/template](https://golang.org/pkg/html/template/) do. So values
must be properly escaped before passing them to gziptemplate.*

gziptemplate is faster than [text/template](https://golang.org/pkg/text/template/),
[strings.Replace](https://golang.org/pkg/strings/#Replace),
[strings.Replacer](https://golang.org/pkg/strings/#Replacer)
and [fmt.Fprintf](https://golang.org/pkg/fmt/#Fprintf) on placeholders' substitution.

Below are benchmark* results comparing gziptemplate performance to text/template,
strings.Replace, strings.Replacer and fmt.Fprintf:

```
$ go test -bench=. -benchmem
PASS
BenchmarkFmtFprintf-8                             200000              9131 ns/op              32 B/op          0 allocs/op
BenchmarkTextTemplate-8                           200000             10689 ns/op             368 B/op         19 allocs/op
BenchmarkFastTemplateExecuteFunc-8                200000              9091 ns/op              32 B/op          0 allocs/op
BenchmarkFastTemplateExecute-8                    200000              9195 ns/op              48 B/op          1 allocs/op
BenchmarkFastTemplateExecuteTagFunc-8             200000              9494 ns/op             192 B/op          4 allocs/op
BenchmarkGzipTemplateExecuteFunc-8               5000000               242 ns/op              32 B/op          1 allocs/op
BenchmarkGzipTemplateExecute-8                   5000000               265 ns/op              48 B/op          2 allocs/op
BenchmarkGzipTemplateExecuteFuncBytes-8          3000000               416 ns/op             576 B/op          4 allocs/op
BenchmarkGzipTemplateExecuteBytes-8              3000000               426 ns/op             576 B/op          4 allocs/op
BenchmarkGzipTemplateExecuteTagFunc-8            5000000               375 ns/op             192 B/op          5 allocs/op
```


Docs
====

See https://godoc.org/github.com/tmthrgd/gziptemplate.


Usage
=====

```go
	template := "https://{{host}}/?q={{query}}&foo={{bar}}{{bar}}"
	t := gziptemplate.New(template, "{{", "}}")
	s := t.ExecuteString(map[string]interface{}{
		"host":  "google.com",
		"query": url.QueryEscape("hello=world"),
		"bar":   "foobar",
	})
	s = mustDecompress(s)
	fmt.Printf("%s", s)

	// Output:
	// https://google.com/?q=hello%3Dworld&foo=foobarfoobar
```


Advanced usage
==============

```go
	template := "Hello, [user]! You won [prize]!!! [foobar]"
	t, err := gziptemplate.NewTemplate(template, "[", "]")
	if err != nil {
		log.Fatalf("unexpected error when parsing template: %s", err)
	}
	s := t.ExecuteFuncString(func(w io.Writer, tag string) error {
		switch tag {
		case "user":
			_, err := io.WriteString(w, "John")
			return err
		case "prize":
			_, err := io.WriteString(w, "$100500")
			return err
		default:
			_, err := fmt.Fprintf(w, "[unknown tag %q]", tag)
			return err
		}
	})
	s = mustDecompress(s)
	fmt.Printf("%s", s)

	// Output:
	// Hello, John! You won $100500!!! [unknown tag "foobar"]
```
