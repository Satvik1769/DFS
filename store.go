package main

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

type PathTransformFunc func(string) PathKey

const DefaultRoot = "store"

type StoreOps struct {
	// Root is the root directory  for the store
	Root              string
	PathTransformFunc PathTransformFunc
}

type Store struct {
	StoreOps
	keys map[string]string
}

type PathKey struct {
	Pathname string
	Filename string
}

var DefaultPathTransformFunc = func(key string) PathKey {
	return PathKey{
		Pathname: key,
		Filename: key,
	}
}

func NewStore(opts StoreOps) *Store {

	if opts.PathTransformFunc == nil {
		opts.PathTransformFunc = DefaultPathTransformFunc
	}

	if len(opts.Root) == 0 {
		opts.Root = DefaultRoot
	}

	return &Store{
		StoreOps: opts,
		keys:     make(map[string]string),
	}
}

func (s *Store) AddKey(logicalKey string, pathName PathKey) {
	if s.keys == nil {
		s.keys = make(map[string]string)
	}
	s.keys[logicalKey] = pathName.FullPathName()
}

func CASPathTransformFunc(key string) PathKey {
	hash := sha1.Sum([]byte(key))
	hashStr := hex.EncodeToString(hash[:])
	blockSize := 5
	sliceLen := len(hashStr) / blockSize
	paths := make([]string, sliceLen)

	for i := 0; i < sliceLen; i++ {
		from, to := i*blockSize, (i+1)*blockSize
		paths[i] = hashStr[from:to]
	}

	return PathKey{
		Pathname: strings.Join(paths, "/"),
		Filename: hashStr,
	}
}

func (p PathKey) FirstPathName() string {
	paths := strings.Split(p.Pathname, "/")
	if len(paths) > 0 {
		return paths[0]
	}
	return ""
}

func (p PathKey) FullPathName() string {
	return fmt.Sprintf("%s/%s", p.Pathname, p.Filename)
}

func (s *Store) PathTransformFunc(key string) PathKey {
	return s.StoreOps.PathTransformFunc(key)
}

func (s *Store) readStream(id, key string) (int64, io.ReadCloser, error) {
	actualPath, ok := s.keys[key]
	if !ok {
		return 0, nil, fmt.Errorf("no file found for key %s", key)
	}

	fullPathWithRoot := filepath.Join(s.Root, id, actualPath)
	file, err := os.Open(fullPathWithRoot)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to open file: %v", err)
	}

	fi, err := file.Stat()
	if err != nil {
		return 0, nil, fmt.Errorf("failed to stat file: %v", err)
	}

	return fi.Size(), file, nil
}
func (s *Store) Read(id string, key string) (int64, io.Reader, error) {
	return s.readStream(id, key)
}

func (s *Store) Delete(id string, key string) error {
	pathKey := s.PathTransformFunc(key)

	defer func() {
		log.Printf("Deleted %s from disk", pathKey.Filename)

	}()
	if err := os.RemoveAll(pathKey.FullPathName()); err != nil {
		return err
	}
	var FirstPathNameWithRoot = s.Root + "/" + id + "/" + pathKey.FirstPathName()

	// Remove the key from the in-memory registry to sync with disk deletion
	delete(s.keys, key)

	return os.RemoveAll(FirstPathNameWithRoot)
}

func (s *Store) Clear() error {
	return os.RemoveAll(s.Root)
}

func (s *Store) Has(id, key string) bool {
	if s.keys == nil {
		return false
	}
	actualPath, ok := s.keys[key]
	if !ok {
		return false
	}

	// Verify file actually exists on disk, not just in memory
	fullPathWithRoot := filepath.Join(s.Root, id, actualPath)
	_, err := os.Stat(fullPathWithRoot)
	return err == nil
}

func (s *Store) Write(id string, key string, r io.Reader) (int64, error) {

	return s.writeStream(id, key, r)
}

func (s *Store) writeDecrypt(encKey []byte, id string, key string, r io.Reader) (int64, error) {
	f, err := s.openFileForWriting(id, key)
	if err != nil {
		return 0, err
	}

	n, err := copyDecrypt(encKey, r, f)

	return int64(n), err
}

func (s *Store) writeStream(id string, key string, r io.Reader) (int64, error) {
	f, err := s.openFileForWriting(id, key)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	return io.Copy(f, r)
}

func (s *Store) openFileForWriting(id string, key string) (*os.File, error) {
	pathName := s.PathTransformFunc(key)
	pathNameWithRoot := s.Root + "/" + id + "/" + pathName.Pathname
	if err := os.MkdirAll(pathNameWithRoot, os.ModePerm); err != nil {
		return nil, err
	}

	fullPath := pathName.FullPathName()

	s.AddKey(key, pathName)
	fullPathWithRoot := s.Root + "/" + id + "/" + fullPath
	return os.Create(fullPathWithRoot)

}
