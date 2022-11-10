package dynbufio

import (
	"bufio"
	"errors"
	"io"
	"sync"
)

const minWriteBufferSize = 4096

// XXX explain the purpose
type emptyWriterT struct{}

var emptyWriter emptyWriterT

type BufferPool struct {
	// pools of buffers of size 4k -> n
	pools  []sync.Pool
	maxIdx int
}

func (emptyWriterT) Write(_ []byte) (n int, err error) {
	return 0, errors.New("write on empty writer")
}

func NewBufferPool(minSize int, maxSize int) *BufferPool {
	if minSize <= 0 {
		minSize = minWriteBufferSize
	}
	sizes := []int{minSize}
	for minSize < maxSize {
		minSize = minSize << 1
		sizes = append(sizes, minSize)
	}

	var pools []sync.Pool
	for _, s := range sizes {
		size := s
		pools = append(pools, sync.Pool{
			New: func() interface{} {
				return bufio.NewWriterSize(emptyWriter, size)
			},
		})
	}
	return &BufferPool{
		pools:  pools,
		maxIdx: len(sizes) - 1,
	}
}

type DynamicBufWriter struct {
	buf     *bufio.Writer // TODO: this should be private and explicitly forward Write([]byte), Flush(), and Buffered()
	w       io.Writer
	pool    *BufferPool
	poolIdx int
}

func (bp *BufferPool) NewWriteBuffer(w io.Writer) *DynamicBufWriter {
	bw := bp.pools[0].Get().(*bufio.Writer)
	bw.Reset(w)
	return &DynamicBufWriter{buf: bw, pool: bp, w: w}
}

func (dbw *DynamicBufWriter) Buffered() int {
	return dbw.buf.Buffered()
}

func (dbw *DynamicBufWriter) Size() int {
	return dbw.buf.Size()
}

func (dbw *DynamicBufWriter) Write(p []byte) (int, error) {
	shouldGrow := false
	if len(p) > dbw.buf.Available() {
		shouldGrow = true
	}
	n, err := dbw.buf.Write(p)

	if shouldGrow {
		dbw.grow()
	}
	return n, err
}

func (dbw *DynamicBufWriter) grow() {
	if dbw.poolIdx < dbw.pool.maxIdx {
		dbw.buf.Reset(emptyWriter)
		dbw.pool.pools[dbw.poolIdx].Put(dbw.buf)
		dbw.poolIdx++
		dbw.buf = dbw.pool.pools[dbw.poolIdx].Get().(*bufio.Writer)
		dbw.buf.Reset(dbw.w)
	}
}

func (dbw *DynamicBufWriter) shrink() {
	if dbw.poolIdx != 0 {
		dbw.buf.Reset(emptyWriter)
		dbw.pool.pools[dbw.poolIdx].Put(dbw.buf)
		dbw.poolIdx--
		dbw.buf = dbw.pool.pools[dbw.poolIdx].Get().(*bufio.Writer)
		dbw.buf.Reset(dbw.w)
	}
}

func (dbw *DynamicBufWriter) Flush() error {
	shouldGrow := false
	shouldShrink := false
	buffered := dbw.buf.Buffered()
	size := dbw.buf.Size()
	if buffered == size && dbw.poolIdx < dbw.pool.maxIdx {
		shouldGrow = true
	} else if buffered < size/2 {
		shouldShrink = true
	}

	err := dbw.buf.Flush()
	if err != nil {
		return err
	}
	if shouldGrow {
		dbw.grow()
	} else if shouldShrink {
		dbw.shrink()
	}
	return nil
}
