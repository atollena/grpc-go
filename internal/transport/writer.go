/*
 *
 * Copyright 2023 gRPC authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

package transport

import (
	"errors"
	"io"
)

// writer is the interface for bufio.Writer. Loopy flushes explicitly so this interface is necessary
// to allow using an io.Writer, such as a net.Conn, in place of a bufio.Writer.
type writer interface {
	io.Writer
	Buffered() int
	Flush() error
}

// unbufferedWriter is a wrapper for io.Writer that implements the bufio.Writer interface while not
// buffering anything.
type unbufferedWriter struct {
	io.Writer
}

func (unbufferedWriter) Buffered() int {
	return 0
}

func (unbufferedWriter) Flush() error {
	return nil
}

type ioError struct {
	error
}

func (i ioError) Unwrap() error {
	return i.error
}

// isIOError returns true if err originates from an ioWrappingWriter
func isIOError(err error) bool {
	return errors.As(err, &ioError{})
}

// ioErrorWrappingWriter is a wrapper for bufio.Writer that wraps errors into an ioError. It allows
// checking if an error originates from the underlying writer by using isIOError.
type ioErrorWrappingWriter struct {
	writer
}

func (i ioErrorWrappingWriter) Write(p []byte) (n int, err error) {
	n, err = i.writer.Write(p)
	if err != nil {
		err = ioError{error: err}
	}
	return n, err
}

func (i ioErrorWrappingWriter) Flush() error {
	err := i.writer.Flush()
	if err != nil {
		err = ioError{error: err}
	}
	return err
}
