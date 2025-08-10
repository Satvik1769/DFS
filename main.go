package main

import (
	"DFS/p2p"
	"fmt"
	"time"
)

func OnPeer(peer p2p.Peer) error {
	peer.Close() // Close the peer connection immediately for this example
	fmt.Println("logic to handle new peer connection");
	return nil;
		}

func main(){
	fileServerOpts := FileServerOpts{
		StorageRoot: "3000_network",
		PathTransformFunc: CASPathTransformFunc,
		Transport: p2p.NewTcpTransport(p2p.TCPTransportOps{
			ListenAddr:    ":3000",
			HandshakeFunc: p2p.NOPHandTransport,
			Decoder:       p2p.DefaultDecoder{},

			// onPeer function to handle new peer connections (still needs implementation)
		}),
	}
	s := newFileServer(fileServerOpts);

	go func ()  {
		time.Sleep(time.Second * 3);
		s.Stop()
	}()

	if err:= s.Start(); err != nil {
		fmt.Println("Error starting file server:", err)
	}


}
