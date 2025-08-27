package main

import (
	"bytes"
	"fmt"
	"io"
	"testing"
)

func newStore() *Store {
	opts := StoreOps{
		PathTransformFunc: CASPathTransformFunc,
	}
	return NewStore(opts)
}

func teardown(t *testing.T, s *Store) {
	if err := s.Clear(); err != nil {
		t.Errorf("Failed to clear store: %v", err)
	}
}

func TestPathTransformFunc(t *testing.T) {
	key:= "test2_key";
	path_name := CASPathTransformFunc(key);
	fmt.Printf("Path for key '%s': %s\n", key, path_name);
	expected := "e25614c68403cf628010bf113b045f943916f280";
	expectedPathName := "e2561/4c684/03cf6/28010/bf113/b045f/94391/6f280";
	if path_name.Pathname != expectedPathName {
		t.Errorf("Expected path '%s', got '%s'", expectedPathName, path_name.Pathname);
	}
	if path_name.Filename != expected {
		t.Errorf("Expected original '%s', got '%s'", expected, path_name.Filename);
	}
}

func TestStore(t *testing.T) {
	
	s := newStore();
	id := generateId();

	defer teardown(t, s);
	key := "test_key";
	data := []byte("This is a test data stream 2.");

	n, err := s.writeStream(id, key, bytes.NewReader(data));

	if  err != nil {
		t.Errorf("Failed to write stream: %v", err)
	}


	if ok  := s.Has(id, key); !ok {
		t.Errorf("Key '%s' should exist after writing", key)
	}
	_,  r, err := s.Read(id, key);
	if err != nil {
		t.Errorf("Failed to read stream: %v", err)
	}

	b, _ := io.ReadAll(r);


	if(string(b) != string(data)) {
		t.Errorf("Data mismatch: expected '%s', got '%s'", string(data), string(b))
	}
	if err := s.Delete(id, key); err != nil {
		t.Errorf("Failed to delete key: %v", err)
	}
	if ok := s.Has(id, key); ok {
		t.Errorf("Key '%s' should not exist after deletion", key)
	}
	fmt.Printf("TestStore passed, wrote %d bytes for key %s\n", n, key)

}

