package main

import (
	"DFS/gossip"
	"DFS/p2p"
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"
)

type FileServerOpts struct {
	ID                string
	EncKey            []byte
	StorageRoot       string
	PathTransformFunc PathTransformFunc
	Transport         p2p.Transport
	BootstrapNodes    []string
}

type FileServer struct {
	Ops        FileServerOpts
	peers      map[string]p2p.Peer
	pending    map[string]p2p.Peer
	peerLock   sync.Mutex // to protect access to peers map
	store      *Store
	quitch     chan struct{} // channel to signal shutdown
	membership *gossip.Membership
}

func (s *FileServer) AddPeer(peerID string, p p2p.Peer) {
	s.peerLock.Lock()
	defer s.peerLock.Unlock()

	if _, exists := s.peers[peerID]; exists {
		fmt.Printf("Peer %s already exists, skipping add\n", peerID)
		return
	}

	if !p.GetEpepheral() {
		s.peers[peerID] = p
		fmt.Printf("Peer %s added successfully\n", peerID)
	}
}

func (s *FileServer) RemovePeer(remoteAddr string) {
	s.peerLock.Lock()
	defer s.peerLock.Unlock()
	delete(s.peers, remoteAddr)
}

func newFileServer(ops FileServerOpts) *FileServer {
	StoreOps := StoreOps{
		Root:              ops.StorageRoot,
		PathTransformFunc: ops.PathTransformFunc,
	}
	store := NewStore(StoreOps)

	if len(ops.ID) == 0 {
		ops.ID = generateId()
	}

	s := &FileServer{
		Ops:      ops,
		store:    store,
		quitch:   make(chan struct{}),
		peers:    make(map[string]p2p.Peer),
		peerLock: sync.Mutex{},
	}

	if tcpT, ok := ops.Transport.(*p2p.TCPTransport); ok {
		tcpT.OnPeer = s.OnPeer
	}

	return s
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

func (s *FileServer) OnPeer(peer p2p.Peer) error {
	peerID := peer.ID()
	s.peerLock.Lock()
	defer s.peerLock.Unlock()

	if _, exists := s.peers[peerID]; exists {
		fmt.Printf("Peer %s already exists, skipping add\n", peerID)
		return nil
	}

	if peer.GetEpepheral() {
		s.pending[peerID] = peer
		fmt.Printf("Ephemeral peer %s stored in pending\n", peerID)
		return nil
	}

	// Move from pending to ready peers
	delete(s.pending, peerID)
	s.peers[peerID] = peer
	fmt.Printf("Peer %s added successfully\n", peerID)
	return nil
}

type Message struct {
	Payload any
}

type MessageStoreFile struct {
	Key  string
	Size int64
	ID   string
}

type MessageGetFile struct {
	Key string
	ID  string
}

type MessageDeleteFile struct {
	Key string
	ID  string
}

func init() {
	gob.Register(MessageStoreFile{})
	gob.Register(MessageGetFile{})
	gob.Register(MessageDeleteFile{})
}

func (s *FileServer) broadcast(msg *Message) error {
	buf := new(bytes.Buffer)

	if err := gob.NewEncoder(buf).Encode(&msg); err != nil {
		log.Printf("Failed to encode message: %v", err)
		return err
	}

	for _, peer := range s.peers {
		peer.Send([]byte{p2p.IncomingMessage})
		if err := peer.Send(buf.Bytes()); err != nil {
			log.Printf("Failed to send message to peer %s: %v", peer.RemoteAddr().String(), err)
			return err
		}
	}
	return nil
}

func (s *FileServer) Get(key string) (io.Reader, error) {
	if s.store.Has(s.Ops.ID, key) {
		_, r, err := s.store.Read(s.Ops.ID, key)
		return r, err
	}

	fmt.Printf("Fetching file %s from network\n", key)
	msg := Message{
		Payload: MessageGetFile{
			Key: key,
			ID:  s.Ops.ID,
		},
	}

	if err := s.broadcast(&msg); err != nil {
		return nil, fmt.Errorf("failed to broadcast message: %v", err)
	}

	// Create request channels for all non-ephemeral peers BEFORE broadcasting
	peerChannels := make(map[string]chan struct{})
	for addr, peer := range s.peers {
		if !peer.GetEpepheral() {
			ch := peer.AddRequest()
			if ch != nil {
				peerChannels[addr] = ch
			}
		}
	}

	if len(peerChannels) == 0 {
		return nil, fmt.Errorf("no peers available to fetch file from")
	}

	var lastErr error
	success := false

	// Wait for ANY peer to respond
	for addr, readyCh := range peerChannels {
		peer := s.peers[addr]

		select {
		case <-readyCh:
			// Peer is ready, start reading
			var expectedSize int64
			if err := binary.Read(peer, binary.LittleEndian, &expectedSize); err != nil {
				lastErr = fmt.Errorf("failed to read size from peer %s: %v", peer.RemoteAddr(), err)
				peer.RemoveRequest()
				continue
			}

			n, err := s.store.writeDecrypt(s.Ops.EncKey, s.Ops.ID, key, io.LimitReader(peer, expectedSize))
			if err != nil {
				lastErr = fmt.Errorf("failed to write from peer %s: %v", peer.RemoteAddr(), err)
				peer.RemoveRequest()
				continue
			}

			fmt.Printf("Received %d bytes of %s from peer %s\n", n, key, peer.RemoteAddr())
			peer.CloseStream()
			peer.RemoveRequest()
			success = true

			// Clean up remaining peer requests
			for otherAddr, otherPeer := range s.peers {
				if otherAddr != addr {
					otherPeer.RemoveRequest()
				}
			}
			goto Done

		case <-time.After(7 * time.Second):
			fmt.Printf("Peer %s did not become ready in time\n", peer.ID())
			lastErr = fmt.Errorf("timeout waiting for peer %s", peer.RemoteAddr())
			peer.RemoveRequest()
		}
	}

Done:
	if !success {
		return nil, fmt.Errorf("failed to fetch file from any peer: %v", lastErr)
	}

	_, r, err := s.store.Read(s.Ops.ID, key)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (s *FileServer) DeleteFromEveryServer(key string) error {
	if !s.store.Has(s.Ops.ID, key) {
		return fmt.Errorf("do not have the file ")
	}
	msg := Message{
		Payload: MessageDeleteFile{
			Key: key,
			ID:  s.Ops.ID,
		},
	}

	if err := s.broadcast(&msg); err != nil {
		log.Printf("Failed to broadcast message: %v", err)
		return err
	}
	s.store.Delete(s.Ops.ID, key)

	return nil
}

func (s *FileServer) Store(key string, r io.Reader) error {
	fileBuffer := new(bytes.Buffer)
	tee := io.TeeReader(r, fileBuffer)
	size, err := s.store.Write(s.Ops.ID, key, tee)
	if err != nil {
		log.Printf("Failed to write to store: %v", err)
		return err
	}
	fmt.Printf("Stored %d bytes for key %s\n", size, key)

	msg := Message{
		Payload: MessageStoreFile{
			Key:  key,
			Size: size + 16,
			ID:   s.Ops.ID,
		},
	}

	if err := s.broadcast(&msg); err != nil {
		log.Printf("Failed to broadcast message: %v", err)
		return err
	}

	time.Sleep(2 * time.Millisecond)

	// stream the file to all peers
	peers := []io.Writer{}
	for _, peer := range s.peers {
		peers = append(peers, peer)
	}

	mw := io.MultiWriter(peers...)
	mw.Write([]byte{p2p.IncomingStream})
	n, err := copyEncrypt(s.Ops.EncKey, fileBuffer, mw)
	if err != nil {
		log.Printf("Failed to copy file to peers: %v", err)
		return err
	}

	fmt.Printf(" %s Received and Written %d bytes to peers\n", s.Ops.Transport.Addr(), n)
	return nil
}

func (s *FileServer) Start() error {

	if err := s.Ops.Transport.ListenAndAccept(); err != nil {
		return err
	}

	// Parse P2P listen addr
	bindHost := "127.0.0.1"
	bindPort := 0
	if addr := s.Ops.Transport.Addr(); addr != "" {
		host, portStr, err := net.SplitHostPort(addr)
		if err == nil {
			if host != "" {
				bindHost = host
			}
			fmt.Sscanf(portStr, "%d", &bindPort)
		} else {
			if addr[0] == ':' {
				fmt.Sscanf(addr[1:], "%d", &bindPort)
			} else {
				fmt.Sscanf(addr, "%d", &bindPort)
			}
		}
	}

	// 👇 Membership now uses n.Name (the P2P addr) to dial peers
	s.membership = gossip.New(bindHost, bindPort,
		func(peerID string) {
			// peerID already contains IP:storePort
			fmt.Printf("membership: node joined: %s\n", peerID)
			if err := s.Ops.Transport.Dial(peerID); err != nil {
				fmt.Printf("failed to dial new node %s: %v\n", peerID, err)
			}
		},
		func(peerID string) {
			fmt.Printf("membership: node left: %s\n", peerID)
			s.RemovePeer(peerID)
		},
	)

	if len(s.Ops.BootstrapNodes) > 0 {
		gossipAddrs := []string{}
		for _, addr := range s.Ops.BootstrapNodes {
			host, portStr, err := net.SplitHostPort(addr)
			if err != nil {
				fmt.Printf("invalid bootstrap node %q: %v\n", addr, err)
				continue
			}

			var port int
			if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil {
				fmt.Printf("invalid port in bootstrap node %q: %v\n", addr, err)
				continue
			}

			// ✅ Join gossip, not P2P
			gossipAddr := fmt.Sprintf("%s:%d", host, port+1200)
			gossipAddrs = append(gossipAddrs, gossipAddr)
		}

		if err := s.membership.Join(gossipAddrs); err != nil {
			fmt.Printf("failed to join cluster: %v\n", err)
		}
	}

	//if len(s.Ops.BootstrapNodes) > 0 {
	//	s.bootstrapNetwork()
	//}

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
	peer, ok := s.peers[from]
	if !ok {
		return fmt.Errorf("unknown peer: %s", from)
	}

	if !s.store.Has(msg.ID, msg.Key) {
		return fmt.Errorf("%s need to get file %s from disk and it doesn't exist", s.Ops.Transport.Addr(), msg.Key)
	}

	fmt.Printf("%s serving file %s over the network\n", s.Ops.Transport.Addr(), msg.Key)

	fileSize, r, err := s.store.Read(msg.ID, msg.Key)
	if err != nil {
		fmt.Printf("Failed to read file %s from store: %v\n", msg.Key, err)
		return err
	}

	if rc, ok := r.(io.ReadCloser); ok {
		defer rc.Close()
	}

	// Send the file data immediately - no need to track requests on sender side
	peer.Send([]byte{p2p.IncomingStream})
	if err := binary.Write(peer, binary.LittleEndian, fileSize); err != nil {
		return err
	}

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
		return s.handleMessageStorageFile(from, payload)
	case MessageGetFile:
		return s.handleMessageGetFile(from, payload)
	case MessageDeleteFile:
		return s.handleMessageDeleteFile(from, payload)
	default:
		log.Printf("Unknown message type: %T", payload)
		return nil
	}

}

func (s *FileServer) handleMessageStorageFile(from string, msg MessageStoreFile) error {
	fmt.Printf("%+v\n", msg)
	peer, ok := s.peers[from]
	if !ok {
		log.Printf("Unknown peer: %s", from)
		return fmt.Errorf("unknown peer: %s", from)
	}

	n, err := s.store.Write(msg.ID, msg.Key, io.LimitReader(peer, int64(msg.Size)))
	if err != nil {
		log.Printf("Failed to write to store: %v", err)
		return err
	}
	fmt.Printf("Stored %d bytes for key %s from peer %s\n", n, msg.Key, s.Ops.Transport.Addr())

	peer.CloseStream()

	return nil

}

func (s *FileServer) handleMessageDeleteFile(from string, msg MessageDeleteFile) error {
	if !s.store.Has(msg.ID, msg.Key) {
		return fmt.Errorf("%s need to delete file %s from disk and it doesn't exist", s.Ops.Transport.Addr(), msg.Key)
	}

	fmt.Printf("%s deleting file %s over the network\n", s.Ops.Transport.Addr(), msg.Key)

	if err := s.store.Delete(msg.ID, msg.Key); err != nil {
		fmt.Printf("Failed to delete file %s from store: %v\n", msg.Key, err)
		return err
	}

	fmt.Printf("%s Deleted file %s from store\n", s.Ops.Transport.Addr(), msg.Key)
	return nil
}

func (s *FileServer) Stop() {
	close(s.quitch)
	s.Ops.Transport.Close() // stop TCP listener
	if s.membership != nil {
		s.membership.Leave(time.Second * 5)
		s.membership.Shutdown()
	}
}
