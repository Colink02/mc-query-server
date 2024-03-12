package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"log"
	"net"
)

type AnyComponent struct {
	Value interface{}
}

type query struct {
	Version     queryVersion `json:"version"`
	Players     players      `json:"players"`
	Description AnyComponent `json:"description"`
	Favicon     string       `json:"favicon"`
}
type queryVersion struct {
	Name     string `json:"name"`
	Protocol int    `json:"protocol"`
}
type players struct {
	Max    int      `json:"max"`
	Online int      `json:"online"`
	Sample []player `json:"sample,omitempty"`
}
type player struct {
	Name string `json:"name"`
	Id   string `json:"id"`
}

type handshake struct {
	protocolVersion int32
	serverAddress   string
	serverPort      uint16
	nextState       int32
}

func main() {
	// Create a new UDP connection handler
	tcpAddr, err := net.ResolveTCPAddr("tcp4", "localhost:25567")
	if err != nil {
		log.Fatal(err)
	}

	listener, err := net.ListenTCP("tcp4", tcpAddr)
	println("Server started on localhost:25567")
	if err != nil {
		log.Fatal(err)
	}
	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println("Connection error: ", err)
			return
		}
		go serve(conn)
	}
	println("Server is shutting down...")
}

func serve(conn net.Conn) {
	defer func(conn net.Conn) {
		err := conn.Close()
		if err != nil {
			log.Fatal(err)
		}
	}(conn)
	var state int32 = 0

	for {
		reader := bufio.NewReader(conn)
		_, err := ReadVarInt(reader)
		if err != nil {
			return
		}
		packetId, _ := ReadVarInt(reader)
		// Read the incoming connection into the buffer.
		if state == 0 {
			println("Received a Handshake")
			var done bool
			state, done = getHandshake(conn, state, *reader)
			if done {
				println("Done with handshake")
			}
		} else if state == 1 {
			if packetId == 0x00 {
				println("Received a Status Request")
				// Status request
				if sendStatus(conn) {
					println("Sent Status")
				}
			} else if packetId == 0x01 {
				println("Received a Ping Request")
				// ping request
				if sendPing(conn, *reader) {
					println("Sent Ping")
				}
			}
		}
	}
}

func sendPing(conn net.Conn, reader bufio.Reader) bool {

	packet := bytes.NewBuffer(make([]byte, 1024))
	err := WriteVarInt(packet, VarInt(0x01))
	if err != nil {
		println(err)
		return false
	}
	ping, _ := ReadVarLong(&reader)
	err = WriteVarLong(packet, ping)
	conn.Write(packet.Bytes())
	if err != nil {
		println(err)
		return false
	}

	return true
}

func getHandshake(conn net.Conn, state int32, reader bufio.Reader) (int32, bool) {
	var protocolVersion VarInt
	var serverAddress string
	var serverPort uint16
	var nextState VarInt
	protocolVersion, err := ReadVarInt(&reader)
	if err != nil {
		println("Error ProtocolVersion", err)
		return 0, false
	}
	println("Protocol Version: ", protocolVersion)
	serverAddress, err = ReadString(&reader)
	if err != nil {
		println("Error ServerAddress", err)
		return 0, false
	}
	println("Server Address: ", serverAddress)
	var tmp [2]byte
	if _, err = reader.Read(tmp[:2]); err != nil {
		println("Error ServerPort", err)
		return 0, false
	}
	serverPort = (uint16(tmp[1]) << 0) | (uint16(tmp[0]) << 8)
	println("Server Port: ", serverPort)
	nextState, err = ReadVarInt(&reader)
	if err != nil {
		println("Error nextState", err)
		return 0, false
	}
	println("Next State: ", nextState)

	println("Got a handshake from ", conn.RemoteAddr().String())
	state = int32(nextState)
	return state, true
}

func sendStatus(conn net.Conn) bool {
	println("Got a packet from ", conn.RemoteAddr().String())

	packet := bytes.NewBuffer(make([]byte, 1024))
	err := WriteVarInt(packet, VarInt(0))
	if err != nil {
		println(err)
		return true
	}
	testQuery := &query{
		Version: queryVersion{
			Name:     "1.19.4",
			Protocol: 762,
		},
		Players: players{
			Max:    20,
			Online: 10,
		},
		Description: AnyComponent{
			"Super Epic Overridden Server Description!",
		},
	}

	b, err := json.Marshal(&testQuery)
	if err != nil {
		log.Fatal(err)
	}

	err = WriteString(packet, string(b))
	println("String: ", string(b))
	if err != nil {
		return true
	}
	_, writeErr := conn.Write(packet.Bytes())
	if writeErr != nil {
		println(writeErr.Error())
		return true
	}
	return false
}
