package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"strings"
)


type PathTransforFunc func(string) PathKey;

type StoreOps struct {
	PathTransforFunc PathTransforFunc;
}

type Store struct {
	StoreOps;
}

type PathKey struct {
	Pathname string;
	Filename string;
}

var DefaultPathTransformFunc = func(key string) PathKey {
	return PathKey{
		Pathname: key,
		Filename: key,
	}
}

func NewStore(opts StoreOps) *Store {
	return &Store{
		StoreOps: opts,
	}
}

func CASPathTransformFunc(key string) PathKey {
	hash := sha1.Sum([]byte(key));
	hashStr := hex.EncodeToString(hash[:]);
	blockSize := 5;
	sliceLen := len(hashStr)/ blockSize;
	paths := make([] string, sliceLen);

	for i:= 0; i < sliceLen; i++ {
		from, to := i*blockSize, (i+1)*blockSize;
		paths[i] = hashStr[from:to];
	}
	return PathKey{
		Pathname: strings.Join(paths, "/"),
		Filename: hashStr,
	}
}



func (p PathKey) FullPathName() string {
	return fmt.Sprintf("%s/%s", p.Pathname, p.Filename);
}

func (s *Store) PathTransformFunc(key string) PathKey {
	return s.StoreOps.PathTransforFunc(key);
}
func (s *Store) readStream(key string) (io.ReadCloser, error) {
	pathName := s.PathTransforFunc(key);
	return  os.Open(pathName.FullPathName());
	
}

func (s * Store) Read(key string) (io.Reader, error) {
	f, err := s.readStream(key);
	if err != nil {
		return nil, err;
	}
	defer f.Close();
	buf := new(bytes.Buffer);
	_, err = io.Copy(buf, f);

	return buf, err;
}

func (s *Store) Delete(key string) error {
	pathKey := s.PathTransformFunc(key);

	defer func(){
		log.Printf("Deleted %s from disk", pathKey.Filename);

	}();
	if err := os.RemoveAll(pathKey.FullPathName()); err != nil {
		return  err;
	}
	return os.RemoveAll(pathKey.Pathname);
}

func (s *Store) Has(Key string) bool {
	PathKey := s.PathTransformFunc(Key);
	_, err := os.Stat(PathKey.FullPathName());
	if(err != nil && err == fs.ErrNotExist){
		return false;
	}
	return  true;

}

func (s *Store) writeStream(key string, r io.Reader) error {
	pathName := s.PathTransforFunc(key);

	if err := os.MkdirAll(pathName.Pathname, os.ModePerm); err != nil {
		return  err;
	}

	fullPath := pathName.FullPathName();
	f, err := os.Create(fullPath);
	if err != nil {
		return err;
	}

	n, err := io.Copy(f,r);
	if err != nil {
		return err;
	}
	fmt.Printf("Wrote %d bytes to %s\n", n, fullPath);

	return nil;
}