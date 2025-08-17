package main

import (
	"DFS/p2p"
	"bytes"
	"encoding/gob"
	"fmt"
	"io"
	"log"
	"sync"
	"time"
)

type FileServerOpts struct {
	StorageRoot       string
	PathTransformFunc PathTransformFunc
	Transport         p2p.Transport
	BootstrapNodes    []string
}

type FileServer struct {
	Ops      FileServerOpts
	peers    map[string]p2p.Peer
	peerLock sync.Mutex // to protect access to peers map
	store    *Store
	quitch   chan struct{} // channel to signal shutdown
}

func newFileServer(ops FileServerOpts) *FileServer {
	StoreOps := StoreOps{
		Root:              ops.StorageRoot,
		PathTransformFunc: ops.PathTransformFunc,
	}
	store := NewStore(StoreOps)
	return &FileServer{
		Ops:      ops,
		store:    store,
		quitch:   make(chan struct{}),
		peers:    make(map[string]p2p.Peer),
		peerLock: sync.Mutex{},
	}
}

func (s *FileServer) bootstrapNetwork() error {
	for _, addr := range s.Ops.BootstrapNodes {
		if len(addr) == 0 {
			continue
		}
		go func(addr string) {
			if err := s.Ops.Transport.Dial(addr); err != nil {
				log.Printf("Failed to connect to bootstrap node %s: %v", addr, err)
			}
		}(addr)
	}
	return nil
}

func (s *FileServer) OnPeer(p p2p.Peer) error {
	s.peerLock.Lock()
	defer s.peerLock.Unlock()
	s.peers[p.RemoteAddr().String()] = p
	log.Printf("New peer connected: %s", p.RemoteAddr().String())
	return nil

}

type Message struct {
	Payload any
}

type MessageStoreFile struct {
	Key string 
	Size int
}

func init() {
    gob.Register(MessageStoreFile{})
}

func (s *FileServer) broadcast(msg *Message) error {
	peers := []io.Writer{}
	for _, peer := range s.peers {
		peers = append(peers, peer)
	}
	mw := io.MultiWriter(peers...)
	return gob.NewEncoder(mw).Encode(msg)

}

func (s *FileServer) StoreData(key string, r io.Reader) error {
	buf := new(bytes.Buffer)
	msg := Message{
		Payload: MessageStoreFile{
			Key: key,
			Size: int(r.(*bytes.Reader).Len()),
		},
	}

	if err := gob.NewEncoder(buf).Encode(&msg); err != nil {
		log.Printf("Failed to encode message: %v", err)
		return err
	}

	for _, peer := range s.peers {
		if err := peer.Send(buf.Bytes()); err != nil {
			log.Printf("Failed to send message to peer %s: %v", peer.RemoteAddr().String(), err)
			return err
		}
	} 

	time.Sleep(4 * time.Second)
 
	for _, peer := range s.peers {
		n, err := io.Copy(peer, r);
		if  err != nil {
			log.Printf("Failed to send message to peer %s: %v", peer.RemoteAddr().String(), err)
			return err
		}
		fmt.Printf("Received and Written %d bytes to peer %s\n", n, peer.RemoteAddr().String())
	} 

	return nil;
}

func (s *FileServer)  Start() error {

	if err := s.Ops.Transport.ListenAndAccept(); err != nil {
		return err
	}

	if len(s.Ops.BootstrapNodes) > 0 {
		s.bootstrapNetwork()
	}

	s.loop()

	return nil
}


func (s *FileServer) loop() {
	defer func() {
		log.Println("FileServer loop stopped due to user request or error.")
		s.Ops.Transport.Close()
	}()
	for {
		select {
		case rpc := <-s.Ops.Transport.Consume():
			var msg Message;
			payloadBytes := rpc.Payload
			if err := gob.NewDecoder(bytes.NewReader(payloadBytes)).Decode(&msg); err != nil {
				log.Printf("Failed to decode message: %v", err)
				return
			}


			if err := s.handleMessage(rpc.From, &msg ); err != nil {
				log.Printf("Failed to handle message: %v", err)
				return
			}

			fmt.Printf("Received message : %+v\n", msg.Payload)
 
	

		case <-s.quitch:
			return
		}
	}
}

func (s *FileServer) handleMessage(from string, msg *Message) error {
	switch payload := msg.Payload.(type) {
	case MessageStoreFile:
		s.handleMessageStorageFile(from, payload)
	default:
		log.Printf("Unknown message type: %T", payload)
		return nil
	}
	return nil
}

func (s *FileServer) handleMessageStorageFile(from string, msg MessageStoreFile) error {
			fmt.Printf("%+v\n", msg)
			peer, ok := s.peers[from]
			if !ok {
				log.Printf("Unknown peer: %s", from)
				return fmt.Errorf("unknown peer: %s", from)
			}

			if err := s.store.Write(msg.Key, io.LimitReader(peer, int64(msg.Size) )); err != nil {
				log.Printf("Failed to write to store: %v", err)
				return err
			}

			peer.(*p2p.TCPPeer).Wg.Done(); 
			return nil;

}

func (s *FileServer) Stop() {
	close(s.quitch)
}