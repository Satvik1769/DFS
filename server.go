package main

import (
	"DFS/gossip"
	"DFS/p2p"
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
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
	Ops            FileServerOpts
	peers          map[string]p2p.Peer
	pending        map[string]p2p.Peer
	peerLock       sync.Mutex // to protect access to peers map
	store          *Store
	quitch         chan struct{} // channel to signal shutdown
	membership     *gossip.Membership
	listResponseCh chan MessageListFilesResponse
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
		Ops:            ops,
		store:          store,
		quitch:         make(chan struct{}),
		peers:          make(map[string]p2p.Peer),
		peerLock:       sync.Mutex{},
		listResponseCh: make(chan MessageListFilesResponse, 10),
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

type MessageListFiles struct {
	BaseKey string
	ID      string
}

type MessageListFilesResponse struct {
	BaseKey string
	Keys    []string
}

func init() {
	gob.Register(MessageStoreFile{})
	gob.Register(MessageGetFile{})
	gob.Register(MessageDeleteFile{})
	gob.Register(MessageListFiles{})
	gob.Register(MessageListFilesResponse{})
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
		r, err := s.store.ReadDecrypt(s.Ops.EncKey, s.Ops.ID, key)
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

	// Collect peers and their channels
	type peerWithChan struct {
		addr string
		peer p2p.Peer
		ch   chan struct{}
	}

	var peersToCheck []peerWithChan
	for addr, peer := range s.peers {
		if !peer.GetEpepheral() {
			ch := peer.AddRequest()
			if ch != nil {
				peersToCheck = append(peersToCheck, peerWithChan{
					addr: addr,
					peer: peer,
					ch:   ch,
				})
			}
		}
	}

	if len(peersToCheck) == 0 {
		return nil, fmt.Errorf("no peers available to fetch file from")
	}

	// Use a goroutine to wait for first successful response
	resultCh := make(chan error, len(peersToCheck))
	var successPeer p2p.Peer
	var once sync.Once

	for _, p := range peersToCheck {
		go func(peerData peerWithChan) {
			select {
			case <-peerData.ch:
				// Try to read from this peer
				var expectedSize int64
				if err := binary.Read(peerData.peer, binary.LittleEndian, &expectedSize); err != nil {
					peerData.peer.RemoveRequest()
					resultCh <- fmt.Errorf("failed to read size from peer %s: %v", peerData.peer.RemoteAddr(), err)
					return
				}

				n, err := s.store.writeDecrypt(s.Ops.EncKey, s.Ops.ID, key, io.LimitReader(peerData.peer, expectedSize))
				if err != nil {
					peerData.peer.RemoveRequest()
					resultCh <- fmt.Errorf("failed to write from peer %s: %v", peerData.peer.RemoteAddr(), err)
					return
				}

				fmt.Printf("Received %d bytes of %s from peer %s\n", n, key, peerData.peer.RemoteAddr())
				peerData.peer.CloseStream()
				peerData.peer.RemoveRequest()

				once.Do(func() {
					successPeer = peerData.peer
				})
				resultCh <- nil // Success

			case <-time.After(7 * time.Second):
				fmt.Printf("Peer %s did not become ready in time\n", peerData.peer.ID())
				peerData.peer.RemoveRequest()
				resultCh <- fmt.Errorf("timeout waiting for peer %s", peerData.peer.RemoteAddr())
			}
		}(p)
	}

	// Wait for first successful response
	var lastErr error
	for i := 0; i < len(peersToCheck); i++ {
		err := <-resultCh
		if err == nil {
			// Success! Clean up other peers
			for _, p := range peersToCheck {
				if p.peer != successPeer {
					p.peer.RemoveRequest()
				}
			}

			_, r, readErr := s.store.Read(s.Ops.ID, key)
			return r, readErr
		}
		lastErr = err
	}

	return nil, fmt.Errorf("failed to fetch file from any peer: %v", lastErr)
}

func (s *FileServer) GetFolder(baseKey, destPath string) error {
	// 1️⃣ Try listing locally
	keys := s.store.ListKeysByPrefix(baseKey)

	// 2️⃣ If local files missing, ask peers for the list
	if len(keys) == 0 {
		fmt.Printf("⚠️ No local files for prefix %s — requesting from peers...\n", baseKey)

		msg := Message{
			Payload: MessageListFiles{
				BaseKey: baseKey,
				ID:      s.Ops.ID,
			},
		}

		if err := s.broadcast(&msg); err != nil {
			return fmt.Errorf("failed to broadcast ListFiles: %v", err)
		}

		// Wait for peers to respond (implement MessageListFilesResponse handler)
		keys = s.awaitKeysFromPeers(baseKey)
		if len(keys) == 0 {
			return fmt.Errorf("no files found with prefix %s (local or remote)", baseKey)
		}
	}

	// 3️⃣ Fetch each file — now Get() handles local/network logic
	for _, key := range keys {
		r, err := s.Get(key)
		if err != nil {
			return fmt.Errorf("failed to get key %s: %w", key, err)
		}

		// Compute local destination path
		relPath := strings.TrimPrefix(key, baseKey)
		relPath = strings.TrimPrefix(relPath, "/")
		fullPath := filepath.Join(destPath, relPath)

		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			return fmt.Errorf("failed to create directories for %s: %w", fullPath, err)
		}

		outFile, err := os.Create(fullPath)
		if err != nil {
			return fmt.Errorf("failed to create file %s: %w", fullPath, err)
		}

		_, err = io.Copy(outFile, r)
		outFile.Close()
		if err != nil {
			return fmt.Errorf("failed to write file %s: %w", fullPath, err)
		}

		fmt.Printf("✅ Retrieved file %s → %s\n", key, fullPath)
	}

	return nil
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

func (s *FileServer) StoreFolder(baseKey, folderPath string) error {
	err := filepath.WalkDir(folderPath, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(folderPath, path)
		if err != nil {
			return err
		}

		key := filepath.ToSlash(filepath.Join(baseKey, relPath))

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		fmt.Printf("📦 Storing file: %s → key: %s\n", path, key)
		if err := s.Store(key, file); err != nil {
			return fmt.Errorf("failed to store %s: %w", path, err)
		}

		return nil
	})
	if err != nil {
		return err
	}

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

	//fmt.Printf("📥 Received decoded message: %+v\n", msg)

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
	case MessageListFiles:
		return s.handleMessageListFile(from, payload)
	case MessageListFilesResponse:
		// ✅ Peer responded with file list — forward to awaitKeysFromPeers
		s.listResponseCh <- payload

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

	n, err := s.store.Write(s.Ops.ID, msg.Key, io.LimitReader(peer, int64(msg.Size)))
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

// ListKeysByPrefix simply filters the in-memory keys
func (s *Store) ListKeysByPrefix(prefix string) []string {
	var result []string
	for k := range s.keys {
		if strings.HasPrefix(k, prefix) {
			result = append(result, k)
		}
	}
	return result
}

func (s *Store) ReadDecrypt(encKey []byte, id, key string) (io.Reader, error) {
	_, r, err := s.Read(id, key)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(encKey)
	if err != nil {
		return nil, err
	}

	// Read IV from start of file
	iv := make([]byte, block.BlockSize())
	if _, err := io.ReadFull(r, iv); err != nil {
		return nil, fmt.Errorf("failed to read IV: %w", err)
	}

	// Create CTR stream reader
	stream := cipher.NewCTR(block, iv)
	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		writer := &cipher.StreamWriter{S: stream, W: pw}
		io.Copy(writer, r)
	}()
	return pr, nil
}

func (s *FileServer) awaitKeysFromPeers(baseKey string) []string {
	var allKeys []string
	timeout := time.After(5 * time.Second)

	for {
		select {
		case resp := <-s.listResponseCh:
			if resp.BaseKey == baseKey {
				allKeys = append(allKeys, resp.Keys...)
			}
		case <-timeout:
			return allKeys
		}
	}
}

func (s *FileServer) handleMessageListFile(from string, msg MessageListFiles) error {
	keys := s.store.ListKeysByPrefix(msg.BaseKey)
	resp := Message{
		Payload: MessageListFilesResponse{
			BaseKey: msg.BaseKey,
			Keys:    keys,
		},
	}

	// Send response back to sender peer (find by ID)
	s.peerLock.Lock()
	peer, ok := s.peers[from]
	s.peerLock.Unlock()
	if ok {
		peer.Send([]byte{p2p.IncomingMessage})
		buf := new(bytes.Buffer)
		if err := gob.NewEncoder(buf).Encode(&resp); err != nil {
			return fmt.Errorf("failed to encode response: %v", err)
		}
		peer.Send(buf.Bytes())
	}
	return nil
}
