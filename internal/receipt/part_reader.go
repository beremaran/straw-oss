// Package receipt implements durable receipt-and-check body transport.
package receipt

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/beremaran/straw-oss/internal/objectstore"
)

// Receipt directions and lifecycle states are stable public values.
type partReader struct {
	ctx     context.Context
	store   objectstore.Store
	keys    []string
	index   int
	current io.ReadCloser
}

func (r *partReader) Read(p []byte) (int, error) {
	for {
		if r.current == nil {
			if r.index >= len(r.keys) {
				return 0, io.EOF
			}

			var err error

			r.current, _, err = r.store.Open(r.ctx, r.keys[r.index])
			if err != nil {
				return 0, fmt.Errorf("open receipt part: %w", err)
			}

			r.index++
		}

		n, err := r.current.Read(p)
		if errors.Is(err, io.EOF) {
			_ = r.current.Close()
			r.current = nil

			if n > 0 {
				return n, nil
			}

			continue
		}

		if err != nil {
			return n, fmt.Errorf("read receipt part: %w", err)
		}

		return n, nil
	}
}

func (r *partReader) Close() error {
	if r.current != nil {
		err := r.current.Close()
		if err != nil {
			return fmt.Errorf("close receipt part: %w", err)
		}
	}

	return nil
}
