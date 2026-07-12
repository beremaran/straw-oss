package objectstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	privateDirectoryMode = 0o700
	privateFileMode      = 0o600
	copyBufferBytes      = 64 << 10
)

// Local stores objects under a private directory. It is intended for the
// single-Control development profile or a shared filesystem operators manage.
type Local struct{ Root string }

// Put atomically writes an object beneath the configured root.
func (s Local) Put(ctx context.Context, key string, body io.Reader, size int64, metadata map[string]string) error {
	clean, err := s.cleanKey(key)
	if err != nil {
		return err
	}

	path := filepath.Join(s.Root, filepath.FromSlash(clean))

	err = os.MkdirAll(filepath.Dir(path), privateDirectoryMode)
	if err != nil {
		return fmt.Errorf("create object directory: %w", err)
	}

	tmpName, err := writeTemporaryObject(ctx, filepath.Dir(path), body, size)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmpName) }()

	err = os.Rename(tmpName, path)
	if err != nil {
		return fmt.Errorf("commit object: %w", err)
	}

	return writeObjectMetadata(path, metadata)
}

func writeTemporaryObject(ctx context.Context, directory string, body io.Reader, size int64) (string, error) {
	tmp, err := os.CreateTemp(directory, ".straw-object-*")
	if err != nil {
		return "", fmt.Errorf("create temporary object: %w", err)
	}

	tmpName := tmp.Name()

	err = tmp.Chmod(privateFileMode)
	if err == nil {
		var written int64

		written, err = copyContext(ctx, tmp, body)
		if err == nil && size >= 0 && written != size {
			err = fmt.Errorf("%w: got %d, want %d", errObjectSize, written, size)
		}
	}

	closeErr := tmp.Close()
	if err == nil {
		err = closeErr
	}

	if err != nil {
		_ = os.Remove(tmpName)

		return "", fmt.Errorf("write object: %w", err)
	}

	return tmpName, nil
}

func writeObjectMetadata(path string, metadata map[string]string) error {
	metaRaw, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("encode object metadata: %w", err)
	}

	err = os.WriteFile(path+".metadata.json", metaRaw, privateFileMode)
	if err != nil {
		return fmt.Errorf("write object metadata: %w", err)
	}

	return nil
}

// Open opens an object for streaming reads.
func (s Local) Open(_ context.Context, key string) (io.ReadCloser, Object, error) {
	root, clean, err := s.openRoot(key)
	if errors.Is(err, os.ErrNotExist) {
		return nil, Object{}, ErrNotFound
	}

	if err != nil {
		return nil, Object{}, err
	}

	defer func() { _ = root.Close() }()

	file, err := root.Open(clean)
	if errors.Is(err, os.ErrNotExist) {
		return nil, Object{}, ErrNotFound
	}

	if err != nil {
		return nil, Object{}, fmt.Errorf("open object: %w", err)
	}

	info, err := file.Stat()
	if err != nil {
		_ = file.Close()

		return nil, Object{}, fmt.Errorf("stat object: %w", err)
	}

	metadata := map[string]string{}

	raw, readErr := root.ReadFile(clean + ".metadata.json")
	if readErr == nil {
		_ = json.Unmarshal(raw, &metadata)
	}

	return file, Object{Key: key, Size: info.Size(), LastModified: info.ModTime(), Metadata: metadata}, nil
}

// Delete removes an object and its metadata. Missing objects are ignored.
func (s Local) Delete(_ context.Context, key string) error {
	root, clean, err := s.openRoot(key)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	if err != nil {
		return err
	}

	defer func() { _ = root.Close() }()

	err = root.Remove(clean)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete object: %w", err)
	}

	_ = root.Remove(clean + ".metadata.json")

	return nil
}

// List returns objects with the requested key prefix.
func (s Local) List(_ context.Context, prefix string) ([]Object, error) {
	_, err := s.cleanKey(prefix)
	if err != nil {
		return nil, err
	}

	root, err := os.OpenRoot(s.Root)
	if errors.Is(err, os.ErrNotExist) {
		return []Object{}, nil
	}

	if err != nil {
		return nil, fmt.Errorf("open object root: %w", err)
	}

	defer func() { _ = root.Close() }()

	objects := []Object{}

	err = fs.WalkDir(root.FS(), ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("walk object path: %w", walkErr)
		}

		if entry.IsDir() || strings.HasSuffix(path, ".metadata.json") {
			return nil
		}

		if !strings.HasPrefix(path, prefix) {
			return nil
		}

		info, infoErr := entry.Info()
		if infoErr != nil {
			return fmt.Errorf("inspect object: %w", infoErr)
		}

		objects = append(objects, Object{Key: path, Size: info.Size(), LastModified: info.ModTime()})

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list objects: %w", err)
	}

	return objects, nil
}

func (s Local) openRoot(key string) (*os.Root, string, error) {
	clean, err := s.cleanKey(key)
	if err != nil {
		return nil, "", err
	}

	root, err := os.OpenRoot(s.Root)
	if err != nil {
		return nil, "", fmt.Errorf("open object root: %w", err)
	}

	return root, clean, nil
}

func (s Local) cleanKey(key string) (string, error) {
	if s.Root == "" {
		return "", errLocalRootRequired
	}

	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(key)))
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", errInvalidKey
	}

	return clean, nil
}

func copyContext(ctx context.Context, dst io.Writer, src io.Reader) (int64, error) {
	buf := make([]byte, copyBufferBytes)

	var total int64

	for {
		err := ctx.Err()
		if err != nil {
			return total, fmt.Errorf("copy object context: %w", err)
		}

		n, readErr := src.Read(buf)
		if n > 0 {
			written, writeErr := dst.Write(buf[:n])
			total += int64(written)

			if writeErr != nil {
				return total, fmt.Errorf("write object data: %w", writeErr)
			}

			if written != n {
				return total, io.ErrShortWrite
			}
		}

		if errors.Is(readErr, io.EOF) {
			return total, nil
		}

		if readErr != nil {
			return total, fmt.Errorf("read object data: %w", readErr)
		}
	}
}
