package main

import (
	"DFS/p2p"
	"fmt"
	"runtime"
	"strings"
	"time"
)

func OnPeer(peer p2p.Peer) error {
	fmt.Println("logic to handle new peer connection")
	return nil
}

type Room struct {
	Code  string
	Peers []string
}

var rooms = make(map[string]*Room)

func createRoom(listenAddr string, roomCode string) *FileServer {
	rooms[roomCode] = &Room{
		Code:  roomCode,
		Peers: []string{listenAddr},
	}
	fmt.Printf("Room created with code: %s\n", roomCode)
	return makeServer(listenAddr)
}

func generateRoomCode() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func joinRoom(roomCode string, listenAddr string) *FileServer {
	room, exists := rooms[roomCode]
	if !exists {
		fmt.Printf("Room with code %s does not exist\n", roomCode)
		return nil
	}
	room.Peers = append(room.Peers, listenAddr)
	fmt.Printf("Joined room %s with peers: %v\n", roomCode, room.Peers)
	return makeServer(listenAddr, room.Peers...)
}

func leaveRoom(roomCode string, listenAddr string, server *FileServer) {
	room, exists := rooms[roomCode]
	if !exists {
		fmt.Printf("Room with code %s does not exist\n", roomCode)
		return
	}

	// Remove the peer from the room
	for i, peer := range room.Peers {
		if peer == listenAddr {
			room.Peers = append(room.Peers[:i], room.Peers[i+1:]...)
			break
		}
	}

	fmt.Printf("Peer %s left room %s. Remaining peers: %v\n", listenAddr, roomCode, room.Peers)

	// Check if the server is not nil before stopping it
	if server != nil {
		server.Stop()
	} else {
		fmt.Printf("Server for peer %s is nil, cannot stop\n", listenAddr)
	}
}

func makeServer(listenAddr string, nodes ...string) *FileServer {
	tcpTransport := p2p.NewTcpTransport(p2p.TCPTransportOps{
		ListenAddr:    listenAddr,
		HandshakeFunc: p2p.NOPHandTransport,
		Decoder:       p2p.DefaultDecoder{},
	})
	if runtime.GOOS == "windows" {
		listenAddr = strings.ReplaceAll(listenAddr, ":", "_")
	}
	fileServerOpts := FileServerOpts{
		EncKey:            newEncryptionKey(),
		StorageRoot:       listenAddr + "_network",
		PathTransformFunc: CASPathTransformFunc,
		Transport:         tcpTransport,
		BootstrapNodes:    nodes,
	}
	return newFileServer(fileServerOpts)
}

func main() {
	roomCode := generateRoomCode()
	s1 := createRoom("127.0.0.1:3000", roomCode)

	// Join the room
	s2 := joinRoom(roomCode, "127.0.0.1:4000")
	s3 := joinRoom(roomCode, "127.0.0.1:5000")

	go func() {
		s1.Start()
	}()

	go func() {
		s2.Start()
	}()

	go func() {
		s3.Start()
	}()
	//
	time.Sleep(1 * time.Second)
	key := "test_key"
	//
	////leaveRoom(roomCode, "127.0.0.1:4000", s2)
	//
	//data := bytes.NewReader([]byte("test data"))
	//s3.Store("test_key", data)
	//time.Sleep(100 * time.Millisecond)
	//
	//if err := s3.store.Delete(s3.Ops.ID, key); err != nil {
	//	fmt.Printf("Failed to delete from store: %v", err)
	//	return
	//}
	//
	//r, err := s3.Get(key)
	//if err != nil {
	//	fmt.Printf("Failed to read from store: %v", err)
	//	return
	//}
	//
	//b, err := io.ReadAll(r)
	//if err != nil {
	//	fmt.Printf("Failed to read from io.Reader: %v", err)
	//	return
	//}
	//fmt.Println(string(b))
	//err = s3.DeleteFromEveryServer(key)
	//if err != nil {
	//	fmt.Printf("Failed to delete from every server: %v", err)
	//	return
	//}
	//time.Sleep(1 * time.Second)

	s3.StoreFolder(key, "./p2p")

	time.Sleep(1 * time.Second)

	err := s2.GetFolder(key, "./downloaded_p2p")
	if err != nil {
		fmt.Printf("Failed to read from store: %v", err)
		return
	}

	leaveRoom(roomCode, "127.0.0.1:5000", s3)

	leaveRoom(roomCode, "127.0.0.1:4000", s2)

	leaveRoom(roomCode, "127.0.0.1:3000", s1)

}
