package main

import (
	"DFS/p2p"
	"bytes"
	"fmt"
	"io"
	"time"
)

func OnPeer(peer p2p.Peer) error {
	fmt.Println("logic to handle new peer connection")
	return nil
}

func makeServer(listenAddr string, nodes ...string) *FileServer {

	tcpTransport := p2p.NewTcpTransport(p2p.TCPTransportOps{
		ListenAddr:    listenAddr,
		HandshakeFunc: p2p.NOPHandTransport,
		Decoder:       p2p.DefaultDecoder{},
	})
	fileServerOpts := FileServerOpts{
		EncKey:  newEncryptionKey(),
		StorageRoot:       listenAddr + "_network",
		PathTransformFunc: CASPathTransformFunc,
		Transport:         tcpTransport,
		BootstrapNodes:    nodes,
	}
	s := newFileServer(fileServerOpts)
	tcpTransport.OnPeer = s.OnPeer
	return s

}

func main() {
	s1 := makeServer(":3000", "")
	s2 := makeServer(":4000", ":3000")
	s3 := makeServer(":5000", ":3000", ":4000")

	go func() {
		s1.Start()

	}()

	go func() {
		s2.Start()
	}()

		go func() {
		s3.Start()
	}()

	time.Sleep(1 * time.Second)
	key := "test_key"

		data := bytes.NewReader([]byte("test data"))
		s3.Store("test_key", data)
		time.Sleep(100 * time.Millisecond)

		if err := s3.store.Delete(key); err != nil {
			fmt.Printf("Failed to delete from store: %v", err)
			return
		} 

	r, err := s3.Get(key) 
	if err != nil {
		fmt.Printf("Failed to read from store: %v", err)
		return
	}

	b, err := io.ReadAll(r)
	if err != nil {
		fmt.Printf("Failed to read from io.Reader: %v", err)
		return
	}
	fmt.Println(string(b)) 

}
