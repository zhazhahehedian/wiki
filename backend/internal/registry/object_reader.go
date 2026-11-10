package registry

import (
	"context"
	"errors"
	"io"

	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
)

// storageError marks a failure to read the stored object, as opposed to an
// archive that is malformed. Callers must not treat it as a bad package.
type storageError struct{ err error }

func (e storageError) Error() string { return "read stored bundle: " + e.err.Error() }
func (e storageError) Unwrap() error { return e.err }

// isArchiveError reports whether err came from the archive content rather
// than from reading storage or cancellation.
func isArchiveError(err error) bool {
	var s storageError
	return !errors.As(err, &s) && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)
}

// windowReader serves ReadAt from one cached window of a stored object, so
// a sequential pass over a large ZIP costs about size/window range requests
// and constant memory. It is not safe for concurrent use.
type windowReader struct {
	ctx     context.Context
	read    func(ctx context.Context, offset, length int64) (io.ReadCloser, error)
	size    int64
	window  int64
	buf     []byte
	start   int64
	fetched int64
	// failure keeps the last storage error: archive readers may report it as
	// a malformed archive, which must not be mistaken for a bad package.
	failure error
}

func (w *windowReader) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 {
		return 0, errors.New("negative offset")
	}
	n := 0
	for n < len(p) {
		pos := off + int64(n)
		if pos >= w.size {
			return n, io.EOF
		}
		if pos < w.start || pos >= w.start+int64(len(w.buf)) {
			if err := w.fill(pos); err != nil {
				return n, err
			}
		}
		n += copy(p[n:], w.buf[pos-w.start:])
	}
	return n, nil
}

func (w *windowReader) fill(pos int64) error {
	length := min(w.window, w.size-pos)
	if int64(cap(w.buf)) < length {
		w.buf = make([]byte, length)
	}
	w.buf = w.buf[:0]
	body, err := w.read(w.ctx, pos, length)
	if err != nil {
		w.failure = storageError{err}
		return w.failure
	}
	defer body.Close()
	buf := w.buf[:length]
	if _, err = io.ReadFull(body, buf); err != nil {
		if w.ctx.Err() != nil {
			return w.ctx.Err()
		}
		w.failure = storageError{err}
		return w.failure
	}
	w.buf, w.start = buf, pos
	w.fetched += length
	return nil
}

// objectReader opens a stored object for ranged reads. Storage without range
// support (test doubles) falls back to skipping a full read.
func (s *Service) objectReader(ctx context.Context, key string, size, window int64) (*windowReader, error) {
	ranged, ok := s.storage.(ports.RangeReader)
	if size <= 0 {
		var err error
		if size, err = s.objectSize(ctx, key); err != nil {
			return nil, err
		}
	}
	read := func(ctx context.Context, offset, length int64) (io.ReadCloser, error) {
		if ok {
			return ranged.GetRange(ctx, key, offset, length)
		}
		body, err := s.storage.Get(ctx, key)
		if err != nil {
			return nil, err
		}
		if _, err = io.CopyN(io.Discard, body, offset); err != nil {
			_ = body.Close()
			return nil, err
		}
		return struct {
			io.Reader
			io.Closer
		}{io.LimitReader(body, length), body}, nil
	}
	return &windowReader{ctx: ctx, read: read, size: size, window: window}, nil
}

func (s *Service) objectSize(ctx context.Context, key string) (int64, error) {
	if ranged, ok := s.storage.(ports.RangeReader); ok {
		return ranged.ObjectSize(ctx, key)
	}
	body, err := s.storage.Get(ctx, key)
	if err != nil {
		return 0, err
	}
	defer body.Close()
	return io.Copy(io.Discard, body)
}
