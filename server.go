package main

import (
	"DFS/p2p"
	"fmt"
	"log"
)



type FileServerOpts struct {
	StorageRoot string;
	PathTransformFunc PathTransformFunc;
	Transport p2p.Transport;
	}

type FileServer struct {
	Ops FileServerOpts;
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
	}
}

func (s *FileServer) Start() error {
	if err := s.Ops.Transport.ListenAndAccept(); err != nil {
		return err;
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
