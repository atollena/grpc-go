package dynbufio

import (
	"bufio"
	"io"
	"sync"
)

const minSize = 4096

type BufferPool struct {
	// pools of buffers of size 4k -> n
	pools []sync.Pool
}

type Writer interface {
	io.Writer
	Flush() error
	Buffered() int
}

func NewBufferPool(sizes []int) *BufferPool {
	bp := BufferPool{}
	for s := range sizes {
		bp.pools = append(bp.pools, sync.Pool{
			New: func() any {
				return bufio.NewWriterSize(s)
			},
		})
	}
}

type DynamicBufWriter struct {
	err  error
	buf  []byte
	n    int
	wr   io.Writer
	pool *BufferPool
}

func (bp *BufferPool) NewWriteBuffer(w io.Writer) DynamicBufWriter {
	return DynamicBufWriter{
		buf: bp.pools[0].Get().([]byte),
		wr:  w,
	}
}

func (dbw *DynamicBufWriter) Flush() error {
	if dbw.err != nil {
		return dbw.err
	}
	if dbw.n == 0 {
		return nil
	}
	n, err := dbw.wr.Write(dbw.buf[0:dbw.n])
	if n < dbw.n && err == nil {
		err = io.ErrShortWrite
		//
	}
	if err != nil {

	}
}

func (bp *BufferPool) NewReadBuffer(r io.Reader) DynamicBufReader {

}
