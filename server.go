package main

import (
	"DFS/p2p"
	"fmt"
	"log"
	"sync"
)



type FileServerOpts struct {
	StorageRoot string;
	PathTransformFunc PathTransformFunc;
	Transport p2p.Transport;
	BootstrapNodes []string;
	}

type FileServer struct {
	Ops FileServerOpts;
	peers map[string]p2p.Peer;
	peerLock sync.Mutex; // to protect access to peers map
	store *Store;
	quitch chan struct{}; // channel to signal shutdown
}

func newFileServer(ops FileServerOpts) *FileServer {
	StoreOps := StoreOps{
		Root: ops.StorageRoot,
		PathTransformFunc: ops.PathTransformFunc,
	}
	store := NewStore(StoreOps);
	return &FileServer{
		Ops: ops,
		store: store,
		quitch: make(chan struct{}),
		peers: make(map[string]p2p.Peer),
		peerLock: sync.Mutex{},
	}
}

func (s *FileServer) bootstrapNetwork() error {
	for _, addr := range s.Ops.BootstrapNodes {
		if(len(addr) == 0){
			continue; 
		}
		go func (addr string)  {
			if err := s.Ops.Transport.Dial(addr); err != nil {
				log.Printf("Failed to connect to bootstrap node %s: %v", addr, err);
			}
		}(addr)
	}
	return nil;
}

func (s *FileServer) OnPeer(p p2p.Peer) error {
	s.peerLock.Lock()
	defer s.peerLock.Unlock()
	s.peers[p.RemoteAddr().String()] = p
	log.Printf("New peer connected: %s", p.RemoteAddr().String())
	return  nil;

}


func (s *FileServer) Start() error {
	if err := s.Ops.Transport.ListenAndAccept(); err != nil {
		return err;
	}

	if(len(s.Ops.BootstrapNodes) > 0){
		s.bootstrapNetwork()
	}

	
	s.loop(); 

	return nil; 
}

func (s *FileServer) loop(){
	defer func ()  {
		log.Println("FileServer loop stopped due to user request or error.");
		s.Ops.Transport.Close();
	}()
	for{
		select {
			case msg := <-s.Ops.Transport.Consume():
				fmt.Println("Received message:", msg) 
			case <- s.quitch:
				return
		}
	}
}

func (s *FileServer) Stop() {
	close(s.quitch)
}
