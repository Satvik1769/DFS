package main

import (
 	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
)

type PathTransformFunc func(string) PathKey

const DefaultRoot = "store"

type StoreOps struct {
	// Root is the root directory for the store
	Root              string
	PathTransformFunc PathTransformFunc
}

type Store struct {
	StoreOps
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
	}
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
func (s *Store) readStream(key string) (int64, io.ReadCloser, error) {
	pathName := s.PathTransformFunc(key)
	fullPathWithRoot := s.Root + "/" + pathName.FullPathName()

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

func (s *Store) Read(key string) (int64, io.Reader, error) {
	return s.readStream(key)
}

func (s *Store) Delete(key string) error {
	pathKey := s.PathTransformFunc(key)

	defer func() {
		log.Printf("Deleted %s from disk", pathKey.Filename)

	}()
	if err := os.RemoveAll(pathKey.FullPathName()); err != nil {
		return err
	}
	var FirstPathNameWithRoot = s.Root + "/" + pathKey.FirstPathName()
	return os.RemoveAll(FirstPathNameWithRoot)
}

func (s *Store) Clear() error {
	return os.RemoveAll(s.Root)
}

func (s *Store) Has(Key string) bool {
	pathKey := s.PathTransformFunc(Key)
	fullPathWithRoot := s.Root + "/" + pathKey.FullPathName()

	_, err := os.Stat(fullPathWithRoot)
	return !errors.Is(err, os.ErrNotExist)
}

func (s *Store) Write(key string, r io.Reader) (int64, error) {

	return s.writeStream(key, r)
}

func (s *Store)  writeDecrypt(encKey []byte, key string,  r io.Reader) (int64, error) {
	f, err := s.openFileForWriting(key )
	if err != nil {
		return 0,err
	}

	n, err := copyDecrypt(encKey, r, f);

	return int64(n), err
}


func (s *Store) writeStream(key string, r io.Reader) (int64, error) {
	f, err := s.openFileForWriting(key)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	return  io.Copy(f, r)
}

func (s *Store)  openFileForWriting(key string) (*os.File, error) {
	pathName := s.PathTransformFunc(key)
	pathNameWithRoot := s.Root + "/" + pathName.Pathname
	if err := os.MkdirAll(pathNameWithRoot, os.ModePerm); err != nil {
		return nil,err
	}

	fullPath := pathName.FullPathName()

	fullPathWithRoot := s.Root + "/" + fullPath
	return  os.Create(fullPathWithRoot)
	

}
