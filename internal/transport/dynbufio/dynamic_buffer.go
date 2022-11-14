package dynbufio

import (
	"bufio"
	"errors"
	"io"
	"net"
	"os"
	"sync"
	"time"
)

const minWriteBufferSize = 1024

// XXX explain the purpose
type failingReadWriterT struct{}

var failingReadWriter failingReadWriterT

func (failingReadWriterT) Write(_ []byte) (n int, err error) {
	return 0, errors.New("write on failing writer")
}

func (failingReadWriterT) Read(_ []byte) (n int, err error) {
	return 0, errors.New("read on failing writer")
}

type BufferPool struct {
	readerPools []sync.Pool
	writerPools []sync.Pool
	maxIdx      int
}

func NewPool(minSize int, maxSize int) *BufferPool {
	if minSize <= 0 {
		minSize = minWriteBufferSize
	}
	sizes := []int{minSize}
	for minSize < maxSize {
		minSize = minSize << 1
		sizes = append(sizes, minSize)
	}

	var writerPools, readerPools []sync.Pool
	for _, s := range sizes {
		size := s
		writerPools = append(writerPools, sync.Pool{
			New: func() interface{} {
				return bufio.NewWriterSize(failingReadWriter, size)
			},
		})
		readerPools = append(readerPools, sync.Pool{
			New: func() interface{} {
				return bufio.NewReaderSize(failingReadWriter, size)
			},
		})
	}
	return &BufferPool{
		writerPools: writerPools,
		readerPools: readerPools,
		maxIdx:      len(sizes) - 1,
	}
}

type DynamicBufWriter struct {
	buf     *bufio.Writer
	w       io.Writer
	pool    *BufferPool
	poolIdx int
}

func (bp *BufferPool) NewWriteBuffer(w io.Writer) *DynamicBufWriter {
	bw := bp.writerPools[0].Get().(*bufio.Writer)
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

func (dbw *DynamicBufWriter) grow() {
	if dbw.poolIdx < dbw.pool.maxIdx {
		dbw.buf.Reset(failingReadWriter)
		dbw.pool.writerPools[dbw.poolIdx].Put(dbw.buf)
		dbw.poolIdx++
		dbw.buf = dbw.pool.writerPools[dbw.poolIdx].Get().(*bufio.Writer)
		dbw.buf.Reset(dbw.w)
	}
}

func (dbw *DynamicBufWriter) shrink() {
	if dbw.poolIdx != 0 {
		dbw.buf.Reset(failingReadWriter)
		dbw.pool.writerPools[dbw.poolIdx].Put(dbw.buf)
		dbw.poolIdx--
		dbw.buf = dbw.pool.writerPools[dbw.poolIdx].Get().(*bufio.Writer)
		dbw.buf.Reset(dbw.w)
	}
}

type DynamicBufReader struct {
	buf     *bufio.Reader
	r       io.Reader
	pool    *BufferPool
	poolIdx int
}

func (bp *BufferPool) NewReaderBuffer(r io.Reader) *DynamicBufReader {
	bw := bp.readerPools[0].Get().(*bufio.Reader)
	bw.Reset(r)
	return &DynamicBufReader{buf: bw, pool: bp, r: r}
}

func (dbr *DynamicBufReader) Read(p []byte) (n int, err error) {
	conn, ok := dbr.r.(net.Conn)
	if ok {
		if dbr.poolIdx != 0 {
			conn.SetReadDeadline(time.Now().Add(10 * time.Second)) // TODO: make that configurable or tune?
		} else {
			conn.SetReadDeadline(time.Time{})
		}
	}

	if dbr.buf.Buffered() != 0 {
		// we have data buffered, guaranteed to not trigger an underlying read,
		// so we can't learn anything about buffer sizing here.
		return dbr.buf.Read(p)
	}
	n, err = dbr.buf.Read(p)
	if errors.Is(err, os.ErrDeadlineExceeded) {
		dbr.shrink()
		return dbr.Read(p)
	}
	if n+dbr.buf.Buffered() >= dbr.buf.Size() {
		// we read more data than buf capacity
		// note that this can be because len(p) > buf.Size(), bypassing the buffer.
		dbr.grow()
	} else if n+dbr.buf.Buffered() < dbr.buf.Size()/2 {
		// we read less than half the buffer size. Shrink the buffer.
		dbr.shrink()
	}
	return n, err
}

func (dbr *DynamicBufReader) grow() {
	if dbr.poolIdx < dbr.pool.maxIdx {
		dbr.buf.Reset(failingReadWriter)
		dbr.pool.readerPools[dbr.poolIdx].Put(dbr.buf)
		dbr.poolIdx++
		dbr.buf = dbr.pool.readerPools[dbr.poolIdx].Get().(*bufio.Reader)
		dbr.buf.Reset(dbr.r)
	}
}

func (dbr *DynamicBufReader) shrink() {
	if dbr.poolIdx != 0 {
		dbr.buf.Reset(failingReadWriter)
		dbr.pool.readerPools[dbr.poolIdx].Put(dbr.buf)
		dbr.poolIdx--
		dbr.buf = dbr.pool.readerPools[dbr.poolIdx].Get().(*bufio.Reader)
		dbr.buf.Reset(dbr.r)
	}
}
