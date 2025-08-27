package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"io"
)

func copyEncrypt(key []byte, src io.Reader, dst io.Writer) (int, error) {
	block, err := aes.NewCipher(key);
	 if err != nil {
		return 0, err
	 }

	 iv := make([]byte, block.BlockSize())
	 if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		 return 0, err  
	 }

	 // prepend iv to file
	 if _, err := dst.Write(iv); err != nil {
		 return 0, err
	 }

	var (
		stream = cipher.NewCTR(block, iv)
		x = block.BlockSize()
	)
	return copyStream(stream, x, src, dst)  
}

func copyDecrypt(key []byte, src io.Reader, dst io.Writer) (int, error) {
	block, err := aes.NewCipher(key);
	if err != nil {
		return 0, err
	}

	iv := make([]byte, block.BlockSize())
	if _, err := src.Read(iv); err != nil {
		return 0, err
	}

	var (
		stream = cipher.NewCTR(block, iv)
		x = block.BlockSize()
	)
	return copyStream(stream, x, src, dst) 
}

func newEncryptionKey() []byte {
	keyBuf := make([]byte, 32)  
	if _, err := io.ReadFull(rand.Reader, keyBuf); err != nil {
		return nil
	}
	return keyBuf
}

func copyStream(stream cipher.Stream, blockSize int, src io.Reader, dst io.Writer) (int, error) {
	var (
		buf = make([]byte, 32*1024)
		x   = blockSize 
	)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			stream.XORKeyStream(buf, buf[:n])
			y, err := dst.Write(buf[:n])
			if err != nil {
				return 0, err
			}
			x += y
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, err
		}
	}
	return x, nil
}

func hashKey(key string) string {
	h := md5.Sum([]byte(key))
	return hex.EncodeToString(h[:])
} 

func generateId() string {
	id := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, id); err != nil {
		return ""
	}
	return hex.EncodeToString(id)
}