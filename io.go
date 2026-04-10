package mkot

import (
	"fmt"
	"io"
	"os"
	"sync"
)

func NopCloser(w io.Writer) io.WriteCloser {
	return nopCloser{w}
}

type nopCloser struct {
	io.Writer
}

func (nopCloser) Close() error {
	return nil
}

type WriterOpenFunc func() (io.WriteCloser, error)

func NewSharedWriter(open WriterOpenFunc) WriterOpenFunc {
	v := &sharedWriter{}
	return func() (io.WriteCloser, error) {
		v.mu.Lock()
		defer v.mu.Unlock()

		v.n++
		if v.n > 1 {
			return v, nil
		}

		w, err := open()
		if err != nil {
			v.n--
			return nil, err
		}

		v.w = w
		return v, nil
	}
}

type sharedWriter struct {
	mu sync.Mutex
	w  io.WriteCloser
	n  int // num of writers
}

func (s *sharedWriter) Write(p []byte) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.w.Write(p)
}

func (s *sharedWriter) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.n--
	if s.n == 0 {
		return s.w.Close()
	}
	return nil
}

type MultiWriteCloser []io.WriteCloser

func (m MultiWriteCloser) Write(p []byte) (n int, err error) {
	for _, w := range m {
		n, err = w.Write(p)
		if err != nil {
			return
		}
		if n != len(p) {
			err = io.ErrShortWrite
			return
		}
	}
	return len(p), nil
}

func (m MultiWriteCloser) Close() error {
	var err error
	for _, w := range m {
		if e := w.Close(); e != nil && err == nil {
			err = e
		}
	}
	return err
}

var Outputs = OutputStore{
	"stdout": NewSharedWriter(func() (io.WriteCloser, error) {
		return NopCloser(os.Stdout), nil
	}),
	"stderr": NewSharedWriter(func() (io.WriteCloser, error) {
		return NopCloser(os.Stderr), nil
	}),
}

type OutputStore map[string]WriterOpenFunc

func (o OutputStore) Open(p string) (io.WriteCloser, error) {
	open, ok := o[p]
	if !ok {
		open = func() (io.WriteCloser, error) {
			return os.OpenFile(p, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
		}
		o[p] = open
	}

	return open()
}

func (o OutputStore) OpenAll(paths []string) (MultiWriteCloser, error) {
	ws := MultiWriteCloser{}
	for _, p := range paths {
		w, err := o.Open(p)
		if err != nil {
			ws.Close()
			return nil, fmt.Errorf("open %q: %w", p, err)
		}
		ws = append(ws, w)
	}

	return ws, nil
}
