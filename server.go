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
	Key  string
	Size int64
}

type MessageGetFile struct {
	Key string
}

func init() {
	gob.Register(MessageStoreFile{})
	gob.Register(MessageGetFile{})
}

func (s *FileServer) broadcast(msg *Message) error {
	buf := new(bytes.Buffer)

	if err := gob.NewEncoder(buf).Encode(&msg); err != nil {
		log.Printf("Failed to encode message: %v", err)
		return err
	}

	for _, peer := range s.peers {
		peer.Send([]byte{p2p.IncomingMessage} )
		if err := peer.Send(buf.Bytes()); err != nil {
			log.Printf("Failed to send message to peer %s: %v", peer.RemoteAddr().String(), err)
			return err
		}
	}
	return nil
}

func (s *FileServer) Get(key string) (io.Reader, error) {
	if s.store.Has(key) {
		return s.store.Read(key)
	}

	fmt.Printf("don't have file %s locally, fetching from network \n", key)
	msg := Message{
		Payload: MessageGetFile{
			Key: key,
		},
	}

	if err := s.broadcast(&msg); err != nil {
		return nil, fmt.Errorf("failed to broadcast message: %v", err)
	}

	time.Sleep(500 * time.Millisecond)  
	// this must be called

	for _, peer := range s.peers {
		fileBuffer := new(bytes.Buffer)
		n, err := io.CopyN(fileBuffer, peer, 9) // read up to 1MB
		if err != nil {
			return nil, fmt.Errorf("failed to read file from peer %s: %v", peer.RemoteAddr().String(), err)
		}

		fmt.Printf("%s Received %d bytes from peer %s\n", s.Ops.Transport.Addr(), n, peer.RemoteAddr().String());
		fmt.Println(fileBuffer.String())
		peer.CloseStream()
		return fileBuffer, nil
	}
	 return nil, fmt.Errorf("file not found on any peer")
}

func (s *FileServer) Store(key string, r io.Reader) error {
	fileBuffer := new(bytes.Buffer)
	tee := io.TeeReader(r, fileBuffer)
	size, err := s.store.Write(key, tee)
	if err != nil {
		log.Printf("Failed to write to store: %v", err)
		return err
	}
	fmt.Printf("Stored %d bytes for key %s\n", size, key)

	msg := Message{
		Payload: MessageStoreFile{
			Key:  key,
			Size: size,
		},
	}

	if err := s.broadcast(&msg); err != nil {
		log.Printf("Failed to broadcast message: %v", err)
		return err
	}

	time.Sleep(100 * time.Millisecond) 

	// stream the file to all peers
	for _, peer := range s.peers {
		peer.Send([]byte{p2p.IncomingStream})
		n, err := io.Copy(peer, fileBuffer)
		if err != nil {
			log.Printf("Failed to send message to peer %s: %v", peer.RemoteAddr().String(), err)
			return err
		}
		fmt.Printf("Received and Written %d bytes to peer %s\n", n, peer.RemoteAddr().String())
	}

	return nil
}

func (s *FileServer) Start() error {

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
			var msg Message
			payloadBytes := rpc.Payload
			if err := gob.NewDecoder(bytes.NewReader(payloadBytes)).Decode(&msg); err != nil {
				log.Printf("Failed to decode message: %v", err)
				return
			}

			if err := s.handleMessage(rpc.From, &msg); err != nil {
				log.Printf("Failed to handle message: %v", err)
				return
			}

			fmt.Printf("Received message : %+v\n", msg.Payload)

		case <-s.quitch:
			return
		}
	}
}

func (s *FileServer) handleMessageGetFile(from string, msg MessageGetFile) error {
	if !s.store.Has(msg.Key) {
		return fmt.Errorf("%s need to get file %s from disk and it doesn't exist", s.Ops.Transport.Addr(), msg.Key)
	}

	fmt.Printf("%s serving file %s over the network\n", s.Ops.Transport.Addr(), msg.Key)

	r, err := s.store.Read(msg.Key)
	if err != nil {
		fmt.Printf("Failed to read file %s from store: %v\n", msg.Key, err)
		return err
	}

	peer, ok := s.peers[from]
	if !ok {
		fmt.Printf("Unknown peer: %s\n", from)
		return fmt.Errorf("unknown peer: %s", from)
	}
	peer.Send([]byte{p2p.IncomingStream})
	 
	n, err := io.Copy(peer, r)
	if err != nil {
		return err
	}

	fmt.Printf("%s Sent %d bytes of file %s to peer %s\n", s.Ops.Transport.Addr(), n, msg.Key, from)


	return nil
}

func (s *FileServer) handleMessage(from string, msg *Message) error {
	switch payload := msg.Payload.(type) {
	case MessageStoreFile:
		s.handleMessageStorageFile(from, payload)
	case MessageGetFile:
		s.handleMessageGetFile(from, payload)
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

	n, err := s.store.Write(msg.Key, io.LimitReader(peer, int64(msg.Size)))
	if err != nil {
		log.Printf("Failed to write to store: %v", err)
		return err
	}
	fmt.Printf("Stored %d bytes for key %s from peer %s\n", n, msg.Key, s.Ops.Transport.Addr())

	peer.CloseStream()
	 
	return nil

}

func (s *FileServer) Stop() {
	close(s.quitch)
}
