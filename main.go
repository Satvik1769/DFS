package main

import (
	"DFS/p2p"
	"fmt"
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
	s2.Start()

	// go func ()  {
	// 	time.Sleep(time.Second * 3);
	// 	s.Stop()
	// }()

	if err := s1.Start(); err != nil {
		fmt.Println("Error starting file server:", err)
	}

}
