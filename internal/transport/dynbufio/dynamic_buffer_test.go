package dynbufio

import (
	"bytes"
	"fmt"
	"testing"
)

func TestWriter(t *testing.T) {
	var data [32768]byte

	for i := 0; i < len(data); i++ {
		data[i] = byte(' ' + i%('~'-' '))
	}

	w := new(bytes.Buffer)

	// XXX switch to anonymous structs
	nwrites := []int{1024, 2048, 4096, 8192, 16384, 32768}
	poolSizes := [][]int{{1, 10}, {100, 1000}, {1000, 10000}, {10, 1}}

	for _, sizes := range poolSizes {
		minSize := sizes[0]
		maxSize := sizes[1]
		pool := NewBufferPool(minSize, maxSize)
		for nwrite := range nwrites {
			context := fmt.Sprintf("nwrite=%d minSize=%d maxSizes=%d", nwrite, minSize, maxSize)
			w.Reset()

			buf := pool.NewWriteBuffer(w)
			n, e := buf.Write(data[0:nwrite])
			if e != nil || n != nwrite {
				t.Errorf("%s: buf.Write %d = %d, %v", context, nwrite, n, e)
				continue
			}
			if e := buf.Flush(); e != nil {
				t.Errorf("%s: buf.Flush = %v", context, e)
			}

			written := w.Bytes()
			if len(written) != nwrite {
				t.Errorf("%s: %d bytes written", context, len(written))
			}
			for l := 0; l < len(written); l++ {
				if written[l] != data[l] {
					t.Errorf("wrong bytes written")
					t.Errorf("want=%q", data[0:len(written)])
					t.Errorf("have=%q", written)
				}
			}
		}
	}
}

func TestWriterGrowShrink(t *testing.T) {
	var data [1024]byte
	for i := 0; i < len(data); i++ {
		data[i] = byte(' ' + i%('~'-' '))
	}

	minSize := 4
	maxSize := 32
	w := new(bytes.Buffer)
	pool := NewBufferPool(minSize, maxSize)
	buf := pool.NewWriteBuffer(w)

	if buf.Size() != minSize {
		t.Errorf("wrong buffer size: expected %d, got %d", buf.Size(), minSize)
	}
	buf.Write(data[0 : minSize+1])
	if buf.Size() != minSize*2 {
		t.Errorf("expected buffer to have grown to %d, but was %d", minSize*2, buf.Size())
	}
	buf.Write(data[0:minSize])
	buf.Write(data[0:minSize])
	buf.Flush() // flushing full buffer

	buf.Write(data[0:minSize])
	buf.Write(data[0:minSize])
	buf.Write(data[0:minSize])
	buf.Write(data[0 : minSize+1])
	if buf.Size() != maxSize {
		t.Errorf("expected buffer to have grown to %d, but was %d", maxSize, buf.Size())
	}

	buf.Write(data[0 : maxSize+1])
	if buf.Size() != maxSize {
		t.Errorf("expected buffer to stay at %d, but was %d", maxSize, buf.Size())
	}

	buf.Write(data[0 : maxSize/2-1])
	buf.Flush()
	if buf.Size() != maxSize/2 {
		t.Errorf("expected buffer to shrink to %d, but was %d", maxSize/2, buf.Size())
	}

	buf.Write(data[0 : maxSize/2-4])
	buf.Flush()
	if buf.Size() != maxSize/2 {
		t.Errorf("expected buffer to stay at %d, but was %d", maxSize/2, buf.Size())
	}

	buf.Write(data[0:1])
	buf.Flush()
	if buf.Size() != maxSize/4 {
		t.Errorf("expected buffer to stay at %d, but was %d", maxSize/4, buf.Size())
	}

	buf.Write(data[0:1])
	buf.Flush()
	if buf.Size() != minSize {
		t.Errorf("expected buffer to stay at %d, but was %d", minSize, buf.Size())
	}
}
