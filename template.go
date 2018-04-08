// Package fasttemplate implements simple and fast template library.
//
// Fasttemplate is faster than text/template, strings.Replace
// and strings.Replacer.
//
// Fasttemplate ideally fits for fast and simple placeholders' substitutions.
package fasttemplate

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"strings"
)

// zero-length type 0 block
var syncFlushFooter = []byte{0x00, 0x00, 0x00, 0xff, 0xff}

// These constants are copied from the flate package, so that code that imports
// this package does not also have to import "compress/flate".
const (
	NoCompression      = flate.NoCompression
	BestSpeed          = flate.BestSpeed
	BestCompression    = flate.BestCompression
	DefaultCompression = flate.DefaultCompression
	HuffmanOnly        = flate.HuffmanOnly
)

type segment struct {
	bytes []byte
	size  int
	crc   uint32
}

// Template implements simple template engine, which can be used for fast
// tags' (aka placeholders) substitution.
type Template struct {
	template []byte
	texts    []segment
	tags     []string
	size     uint32
	gzipHdr  [10]byte
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
	var t Template
	err := t.Reset(template, startTag, endTag, level)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// TagFunc can be used as a substitution value in the map passed to Execute*.
// Execute* functions pass tag (placeholder) name in 'tag' argument.
//
// TagFunc must be safe to call from concurrently running goroutines.
//
// TagFunc must write contents to w and return the number of bytes written.
type TagFunc func(w io.Writer, tag string) (int, error)

// Reset resets the template t to new one defined by
// template, startTag and endTag.
//
// Reset allows Template object re-use.
//
// Reset may be called only if no other goroutines call t methods at the moment.
func (t *Template) Reset(template, startTag, endTag string, level int) error {
	t.texts = t.texts[:0]
	t.tags = t.tags[:0]
	t.size = 0

	const (
		gzipID1     = 0x1f
		gzipID2     = 0x8b
		gzipDeflate = 8
	)
	t.gzipHdr = [10]byte{
		0: gzipID1, 1: gzipID2, 2: gzipDeflate,
		9: 255, // unknown OS
	}

	if level == BestCompression {
		t.gzipHdr[8] = 2
	} else if level == BestSpeed {
		t.gzipHdr[8] = 4
	}

	if len(startTag) == 0 {
		panic("startTag cannot be empty")
	}
	if len(endTag) == 0 {
		panic("endTag cannot be empty")
	}

	tagsCount := strings.Count(template, startTag)
	if tagsCount == 0 {
		var buf bytes.Buffer
		gw, err := gzip.NewWriterLevel(&buf, level)
		if err != nil {
			return err
		}

		if _, err := gw.Write([]byte(template)); err != nil {
			return err
		}

		if err := gw.Close(); err != nil {
			return err
		}

		t.template = buf.Bytes()
		return nil
	}

	if tagsCount+1 > cap(t.texts) {
		t.texts = make([]segment, 0, tagsCount+1)
	}
	if tagsCount > cap(t.tags) {
		t.tags = make([]string, 0, tagsCount)
	}

	fw, err := flate.NewWriter(nil, level)
	if err != nil {
		return err
	}

	st := template

	for {
		var buf bytes.Buffer
		fw.Reset(&buf)

		n := strings.Index(st, startTag)
		ni := n
		if n < 0 {
			ni = len(st)
		}

		si := []byte(st[:ni])
		if _, err := fw.Write(si); err != nil {
			return err
		}

		var err error
		if n < 0 {
			err = fw.Close()
		} else {
			err = fw.Flush()
		}
		if err != nil {
			return err
		}

		t.size += uint32(ni)
		t.texts = append(t.texts, segment{
			bytes: bytes.TrimSuffix(buf.Bytes(), syncFlushFooter),
			size:  ni,
			crc:   crc32.ChecksumIEEE(si),
		})
		if n < 0 {
			break
		}

		st = st[n+len(startTag):]

		n = strings.Index(st, endTag)
		if n < 0 {
			return fmt.Errorf("Cannot find end tag=%q in the template=%q starting from %q", endTag, template, st)
		}

		t.tags = append(t.tags, st[:n])

		st = st[n+len(endTag):]
	}

	return nil
}

var crc32Mat = precomputeCRC32(crc32.IEEE)

// ExecuteFunc calls f on each template tag (placeholder) occurrence.
//
// Returns the number of bytes written to w.
func (t *Template) ExecuteFunc(w io.Writer, f TagFunc) (int64, error) {
	var nn int64

	n := len(t.texts) - 1
	if n == -1 {
		ni, err := w.Write(t.template)
		return int64(ni), err
	}

	ni, err := w.Write(t.gzipHdr[:])
	nn += int64(ni)
	if err != nil {
		return nn, err
	}

	zw := &typeZeroWriter{
		w: w,

		size: t.size,
		crc:  t.texts[0].crc,
	}

	for i := 0; i < n; i++ {
		ti := &t.texts[i]

		ni, err := w.Write(ti.bytes)
		nn += int64(ni)
		if err != nil {
			return nn, err
		}
		if i > 0 {
			zw.crc = combineCRC32(crc32Mat, zw.crc, ti.crc, int64(ti.size))
		}

		zw.wrote = false

		ni, err = f(zw, t.tags[i])
		nn += int64(ni)
		if err != nil {
			return nn, err
		}

		if !zw.wrote {
			ni, err := w.Write(syncFlushFooter)
			nn += int64(ni)
			if err != nil {
				return nn, err
			}
		}
	}

	tn := &t.texts[n]

	ni, err = w.Write(tn.bytes)
	nn += int64(ni)
	if err != nil {
		return nn, err
	}
	digest := combineCRC32(crc32Mat, zw.crc, tn.crc, int64(tn.size))

	var buf [8]byte
	binary.LittleEndian.PutUint32(buf[:4], digest)
	binary.LittleEndian.PutUint32(buf[4:], zw.size)

	ni, err = w.Write(buf[:])
	nn += int64(ni)
	return nn, err
}

// Execute substitutes template tags (placeholders) with the corresponding
// values from the map m and writes the result to the given writer w.
//
// Substitution map m may contain values with the following types:
//   * []byte - the fastest value type
//   * string - convenient value type
//   * TagFunc - flexible value type
//
// Returns the number of bytes written to w.
func (t *Template) Execute(w io.Writer, m map[string]interface{}) (int64, error) {
	return t.ExecuteFunc(w, func(w io.Writer, tag string) (int, error) { return stdTagFunc(w, tag, m) })
}

// ExecuteFuncString calls f on each template tag (placeholder) occurrence
// and substitutes it with the data written to TagFunc's w.
//
// Returns the resulting string.
func (t *Template) ExecuteFuncString(f TagFunc) string {
	var sb strings.Builder
	sb.Grow(len(t.template))
	if _, err := t.ExecuteFunc(&sb, f); err != nil {
		panic(fmt.Sprintf("unexpected error: %s", err))
	}
	return sb.String()
}

// ExecuteString substitutes template tags (placeholders) with the corresponding
// values from the map m and returns the result.
//
// Substitution map m may contain values with the following types:
//   * []byte - the fastest value type
//   * string - convenient value type
//   * TagFunc - flexible value type
func (t *Template) ExecuteString(m map[string]interface{}) string {
	return t.ExecuteFuncString(func(w io.Writer, tag string) (int, error) { return stdTagFunc(w, tag, m) })
}

func stdTagFunc(w io.Writer, tag string, m map[string]interface{}) (int, error) {
	v := m[tag]
	if v == nil {
		return 0, nil
	}
	switch value := v.(type) {
	case []byte:
		return w.Write(value)
	case string:
		return w.Write([]byte(value))
	case TagFunc:
		return value(w, tag)
	default:
		panic(fmt.Sprintf("tag=%q contains unexpected value type=%#v. Expected []byte, string or TagFunc", tag, v))
	}
}

type typeZeroWriter struct {
	w io.Writer

	size uint32
	crc  uint32

	hdrBuf [5]byte

	wrote bool
}

func (w *typeZeroWriter) Write(p []byte) (n int, err error) {
	w.wrote = true

	const maxLength = ^uint16(0)
	for len(p) > int(maxLength) {
		ni, err := w.Write(p[:maxLength])
		n += ni
		if err != nil {
			return n, err
		}

		p = p[maxLength:]
		if len(p) == 0 {
			return n, nil
		}
	}

	w.size += uint32(len(p))
	w.crc = crc32.Update(w.crc, crc32.IEEETable, p)

	/* The following code is equivalent to:
	 *  hbw := newHuffmanBitWriter(w.w)
	 *
	 *  if hbw.writeStoredHeader(len(p), false); hbw.err != nil {
	 *          return 0, hbw.err
	 *  }
	 *
	 *  hbw.writeBytes(p)
	 *  return len(p), hbw.err
	 */

	w.hdrBuf[0] = 0
	binary.LittleEndian.PutUint16(w.hdrBuf[1:], uint16(len(p)))
	binary.LittleEndian.PutUint16(w.hdrBuf[3:], ^uint16(len(p)))

	if _, err = w.w.Write(w.hdrBuf[:]); err != nil {
		return
	}

	ni, err := w.w.Write(p)
	n += ni
	return n, err
}
