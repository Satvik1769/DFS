package main

import (
	"DFS/p2p"
	"fmt"
	"log"
)


func main(){
	fmt.Printf("hello \n");
	t := p2p.NewTcpTransport(":3000");
    if err := t.ListenAndAccept(); err != nil {
        log.Fatal(err)
    }
	select {}

}
