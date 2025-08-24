package main

import (
	"DFS/p2p"
	// "bytes"
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

	go func() {
		s1.Start()

	}()

	go func() {
		s2.Start()
	}()

	time.Sleep(1 * time.Second)

	for i := 0; i < 1; i++ {
		// data := bytes.NewReader([]byte("test data"))
		// s2.Store("test_key_"+fmt.Sprint(i), data)
		// time.Sleep(100 * time.Millisecond)

	// r, err := s2.Get("test_key_" + fmt.Sprint(i))
	// if err != nil {
	// 	fmt.Printf("Failed to read from store: %v", err)
	// 	return
	// }

	// b, err := io.ReadAll(r)
	// if err != nil {
	// 	fmt.Printf("Failed to read from io.Reader: %v", err)
	// 	return
	// }
	// fmt.Println(string(b))
	// time.Sleep(100 * time.Millisecond)


	} 

		// data := bytes.NewReader([]byte("test data"))
		// s2.Store("test_key", data)
		// time.Sleep(100 * time.Millisecond)

	r, err := s2.Get("test_key")
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
