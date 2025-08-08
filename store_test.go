package main

import (
	"bytes"
	"fmt"
	"io"
	"testing"
)

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

func TestStoreDeleteKey(t *testing.T) {
	opts := StoreOps{
		PathTransforFunc: CASPathTransformFunc,
	}
	s := NewStore(opts);
	key := "test_key";

	if err:=s.writeStream(key, bytes.NewReader([]byte("This is a test data stream."))); err != nil {
		t.Errorf("Failed to write stream: %v", err)
	}
	if err := s.Delete(key); err != nil {
		t.Errorf("Failed to delete key: %v", err)
	}

}

func TestStore(t *testing.T) {
	opts := StoreOps{
		PathTransforFunc: CASPathTransformFunc,
	}
	s := NewStore(opts);
	key := "test_key";
	data := []byte("This is a test data stream 2.");
	if err := s.writeStream(key, bytes.NewReader(data)); err != nil {
		t.Errorf("Failed to write stream: %v", err)
	}
	r, err := s.Read(key);
	if err != nil {
		t.Errorf("Failed to read stream: %v", err)
	}

	b, _ := io.ReadAll(r);


	if(string(b) != string(data)) {
		t.Errorf("Data mismatch: expected '%s', got '%s'", string(data), string(b))
	}
}

