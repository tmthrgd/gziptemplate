// Package gziptemplate implements simple and fast gzipped template library.
//
// gziptemplate is faster than text/template, strings.Replace
// and strings.Replacer.
//
// gziptemplate ideally fits for fast and simple placeholders' substitutions.
//
// Unlike other solutions, gziptemplate compresses templates once ahead of time.
package gziptemplate

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"strings"

	"github.com/tmthrgd/gzipbuilder"
)

// These constants are copied from the flate package, so that code that imports
// this package does not also have to import "compress/flate".
const (
	NoCompression      = gzipbuilder.NoCompression
	BestSpeed          = gzipbuilder.BestSpeed
	BestCompression    = gzipbuilder.BestCompression
	DefaultCompression = gzipbuilder.DefaultCompression
	HuffmanOnly        = gzipbuilder.HuffmanOnly
)

// Template implements simple template engine, which can be used for fast
// tags' (aka placeholders) substitution.
type Template struct {
	level    int
	template []byte
	texts    []*gzipbuilder.PrecompressedData
	tags     []string
}

// New parses the given template using the given startTag and endTag
// as tag start and tag end.
//
// The returned template can be executed by concurrently running goroutines
// using Execute* methods.
//
// New panics if the given template cannot be parsed. Use NewTemplate instead
// if template may contain errors.
func New(template, startTag, endTag string, level int) *Template {
	t, err := NewTemplate(template, startTag, endTag, level)
	if err != nil {
		panic(err)
	}
	return t
}

// NewTemplate parses the given template using the given startTag and endTag
// as tag start and tag end.
//
// The returned template can be executed by concurrently running goroutines
// using Execute* methods.
func NewTemplate(template, startTag, endTag string, level int) (*Template, error) {
	if len(startTag) == 0 {
		panic("gziptemplate: startTag cannot be empty")
	}
	if len(endTag) == 0 {
		panic("gziptemplate: endTag cannot be empty")
	}

	t := &Template{
		level: level,
	}

	tagsCount := strings.Count(template, startTag)
	if tagsCount == 0 {
		var buf bytes.Buffer
		gw, err := gzip.NewWriterLevel(&buf, level)
		if err != nil {
			return nil, err
		}

		if _, err := gw.Write([]byte(template)); err != nil {
			return nil, err
		}

		if err := gw.Close(); err != nil {
			return nil, err
		}

		t.template = buf.Bytes()
		return t, nil
	}

	t.texts = make([]*gzipbuilder.PrecompressedData, 0, tagsCount+1)
	t.tags = make([]string, 0, tagsCount)

	w := gzipbuilder.NewPrecompressedWriter(level)

	s := []byte(template)
	st := template

	for {
		if len(t.texts) > 0 {
			w.Reset()
		}

		n := strings.Index(st, startTag)
		ni := n
		if n < 0 {
			ni = len(st)
		}

		w.Write(s[:ni])
		d, err := w.Data()
		if err != nil {
			return nil, err
		}

		t.texts = append(t.texts, d)
		if n < 0 {
			break
		}

		s = s[n+len(startTag):]
		st = st[n+len(startTag):]

		n = strings.Index(st, endTag)
		if n < 0 {
			return nil, fmt.Errorf("gziptemplate: missing end tag=%q in template=%q starting from %q", endTag, template, st)
		}

		t.tags = append(t.tags, st[:n])

		s = s[n+len(endTag):]
		st = st[n+len(endTag):]
	}

	return t, nil
}

// TagFunc can be used as a substitution value in the map passed to Execute*.
// Execute* functions pass tag (placeholder) name in 'tag' argument.
//
// TagFunc must write contents to w and be safe to call from concurrently
// running goroutines.
type TagFunc func(w io.Writer, tag string) error

// ExecuteFunc calls f on each template tag (placeholder) occurrence.
func (t *Template) ExecuteFunc(w io.Writer, f TagFunc) error {
	n := len(t.texts) - 1
	if n == -1 {
		_, err := w.Write(t.template)
		return err
	}

	gw := gzipbuilder.NewWriter(w, t.level)
	uw := gw.UncompressedWriter()

	for i := 0; i < n; i++ {
		gw.AddPrecompressedData(t.texts[i])

		if err := f(uw, t.tags[i]); err != nil {
			return err
		}
	}

	gw.AddPrecompressedData(t.texts[n])
	return gw.Close()
}

// Execute substitutes template tags (placeholders) with the corresponding
// values from the map m and writes the result to the given writer w.
//
// Substitution map m may contain values with the following types:
//   * []byte - the fastest value type
//   * string - convenient value type
//   * TagFunc - flexible value type
func (t *Template) Execute(w io.Writer, m map[string]interface{}) error {
	return t.ExecuteFunc(w, func(w io.Writer, tag string) error {
		return stdTagFunc(w, tag, m)
	})
}

// ExecuteFuncBytes calls f on each template tag (placeholder) occurrence
// and substitutes it with the data written to TagFunc's w.
//
// Returns the resulting byte slice.
func (t *Template) ExecuteFuncBytes(f TagFunc) []byte {
	n := len(t.texts) - 1
	if n == -1 {
		return append([]byte(nil), t.template...)
	}

	b := gzipbuilder.NewBuilder(t.level)
	uw := b.UncompressedWriter()

	for i := 0; i < n; i++ {
		b.AddPrecompressedData(t.texts[i])

		if err := f(uw, t.tags[i]); err != nil {
			panic(fmt.Sprintf("gziptemplate: unexpected error from TagFunc: %s", err))
		}
	}

	b.AddPrecompressedData(t.texts[n])
	return b.BytesOrPanic()
}

// ExecuteBytes substitutes template tags (placeholders) with the corresponding
// values from the map m and returns the result.
//
// Substitution map m may contain values with the following types:
//   * []byte - the fastest value type
//   * string - convenient value type
//   * TagFunc - flexible value type
func (t *Template) ExecuteBytes(m map[string]interface{}) []byte {
	return t.ExecuteFuncBytes(func(w io.Writer, tag string) error {
		return stdTagFunc(w, tag, m)
	})
}

func stdTagFunc(w io.Writer, tag string, m map[string]interface{}) error {
	v := m[tag]
	if v == nil {
		return nil
	}
	switch value := v.(type) {
	case []byte:
		_, err := w.Write(value)
		return err
	case string:
		_, err := w.Write([]byte(value))
		return err
	case TagFunc:
		return value(w, tag)
	default:
		panic(fmt.Sprintf("gziptemplate: tag=%q contains unexpected value type=%#v", tag, v))
	}
}
