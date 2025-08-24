package main

import (
	"bytes"
	"fmt"
	"testing"
)

 func TestCopyEncryptDecrypt (t *testing.T) {
	payload := []byte("This is a test data stream.");
	src := bytes.NewReader(payload);
	dst := new(bytes.Buffer);
	key := newEncryptionKey()
	_, err := copyEncrypt(key, src, dst);
	if err != nil {
		t.Fatalf("copyEncrypt failed: %v", err)
	}
	fmt.Println(dst.String())
	out := new(bytes.Buffer)
	x, err := copyDecrypt(key, dst, out)
	if err != nil {
		t.Fatalf("copyDecrypt failed: %v", err)
	}
	if x != 16 + len(payload ) {
		t.Fatalf("Unexpected number of bytes decrypted. Got: %d", x) 
		t.Fail()
	}

	if out.String() != string(payload) {
		t.Fatalf("Decrypted data does not match original. Got: %s", out.String())
	}
	fmt.Println(out.String())
}
