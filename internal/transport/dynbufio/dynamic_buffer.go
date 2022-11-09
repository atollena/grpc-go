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

func (emptyWriterT) Write(p []byte) (n int, err error) {
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
	*bufio.Writer
	w       io.Writer
	pool    *BufferPool
	poolIdx int
}

func (bp *BufferPool) NewWriteBuffer(w io.Writer) *DynamicBufWriter {
	bw := bp.pools[0].Get().(*bufio.Writer)
	bw.Reset(w)
	return &DynamicBufWriter{Writer: bw, pool: bp, w: w}
}

func (dbw *DynamicBufWriter) Write(p []byte) (int, error) {
	shouldGrow := false
	if len(p) > dbw.Available() && dbw.poolIdx < dbw.pool.maxIdx {
		shouldGrow = true
	}
	n, err := dbw.Writer.Write(p)
	dbw.pool.pools[dbw.poolIdx].Put(dbw.Writer)
	if shouldGrow {
		dbw.poolIdx++
		dbw.Writer = dbw.pool.pools[dbw.poolIdx].Get().(*bufio.Writer)
		dbw.Writer.Reset(dbw.w)
	}
	return n, err
}

func (dbw *DynamicBufWriter) Flush() error {
	// only shrink here
	shouldGrow := false
	shouldShrink := false
	buffered := dbw.Buffered()
	size := dbw.Size()
	if buffered == size && dbw.poolIdx < dbw.pool.maxIdx {
		shouldGrow = true
	} else if buffered < size/2 && dbw.poolIdx != 0 {
		shouldShrink = true
	}

	err := dbw.Writer.Flush()
	if err != nil {
		return err
	}
	if !shouldGrow && !shouldShrink {
		return nil
	}
	dbw.pool.pools[dbw.poolIdx].Put(dbw.Writer)
	if shouldGrow {
		dbw.poolIdx++
	} else {
		// should shrink
		dbw.poolIdx--
	}
	dbw.Writer = dbw.pool.pools[dbw.poolIdx].Get().(*bufio.Writer)
	dbw.Writer.Reset(dbw.w)
	return nil
}
