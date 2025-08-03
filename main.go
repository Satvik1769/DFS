package main

import (
	"DFS/p2p"
	"fmt"
	"log"
)

func OnPeer(peer p2p.Peer) error {
	peer.Close() // Close the peer connection immediately for this example
	fmt.Println("logic to handle new peer connection");
	return nil;
		}

func main(){
	fmt.Printf("hello \n");
	tcpOpts := p2p.TCPTransportOps{
		ListenAddr:    ":3000",
		HandshakeFunc: p2p.NOPHandTransport,
		Decoder: p2p.DefaultDecoder{},
		OnPeer: OnPeer,
	}
	t := p2p.NewTcpTransport(tcpOpts);

	go func() {
		for {
			msg := <-t.Consume()
			fmt.Println("Received message:", msg)
			fmt.Printf("%+v \n", msg)
		}
	}()
	
    if err := t.ListenAndAccept(); err != nil {
        log.Fatal(err)
    }
	select {}

}
