package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"log"
	"net"
	"os"
	"time"

	"github.com/tailscale/hujson"
)

type Config struct {
	Verbose  bool `json:"verbose"`
	Listener struct {
		Host string `json:"host"`
	} `json:"listener"`
	Query query `json:"query"`
}

type query struct {
	Version struct {
		Name     string `json:"name"`
		Protocol int    `json:"protocol"`
	} `json:"version"`
	Players struct {
		Max    int `json:"max"`
		Online int `json:"online"`
		Sample []struct {
			Name string `json:"name"`
			Id   string `json:"id"`
		} `json:"sample,omitempty"`
	} `json:"players"`
	Description interface{} `json:"description"`
	Favicon     string      `json:"favicon"`
}

type Handshake struct {
	ProtocolVersion VarInt
	ServerAddress   string
	ServerPort      uint16
	NextState       VarInt
}

var config = readConfig()

func main() {

	// Create a new UDP connection handler
	tcpAddr, err := net.ResolveTCPAddr("tcp4", config.Listener.Host)
	if err != nil {
		log.Fatal("Error resolving TCP address: ", err.Error())
	}
	listener, err := net.ListenTCP("tcp4", tcpAddr)
	if err != nil {
		log.Fatal("Error listening on TCP address: ", err.Error())
	}
	log.Println("Server started on ", listener.Addr().String())
	defer func(listener *net.TCPListener) {
		err := listener.Close()
		if err != nil {
			log.Println("Error closing listener: ", err.Error())
			return
		}
	}(listener)

	for {
		conn, err := listener.Accept()
		if err != nil {
			err := conn.SetDeadline(time.Now().Add(2 * time.Second))
			if err != nil {
				log.Println("Error setting deadline: ", err.Error())
				return
			}
			log.Println("Connection error: ", err.Error())
			return
		}
		go serve(conn)
	}
}

func serve(conn net.Conn) {
	defer func(conn net.Conn) {
		err := conn.Close()
		if err != nil {
			log.Println("Error closing connection: ", err.Error())
			return
		}
	}(conn)
	var state int32 = 0

	for {
		reader := bufio.NewReader(conn)
		//Read the packet length
		packetLength, err := ReadVarInt(reader)
		if err != nil {
			return
		}
		//Read the packet id
		packetId, _ := ReadVarInt(reader)
		if config.Verbose {
			log.Println("Packet Length is: ", packetLength)
			log.Println("Packet Id is: ", packetId)
		}
		// Read the incoming connection into the buffer.
		if state == 0 {
			if packetId == 0x00 {
				if config.Verbose {
					log.Println("Received a Handshake")
				}
				var done bool
				state, done = getHandshake(conn, *reader)
				if done {
					log.Println("State is now: ", state)
				}
			}
		} else if state == 1 {
			if packetId == 0x00 {
				println("Received a Status Request")
				// Status request
				if sendStatus(conn) {
					if config.Verbose {
						log.Println("Sent Status to ", conn.RemoteAddr().String())
					}
				}
			} else if packetId == 0x01 {
				println("Received a Ping Request")
				// ping request
				if sendPing(conn, *reader) {
					if config.Verbose {
						log.Println("Sent Ping to ", conn.RemoteAddr().String())
					}
				}
			} else if packetId == 0xFE {
				log.Println("Received a Legacy Ping Request")
			}
		}
	}
}

func getHandshake(conn net.Conn, reader bufio.Reader) (int32, bool) {
	var handshake Handshake
	var err error
	// Read Protocol Version
	handshake.ProtocolVersion, err = ReadVarInt(&reader)
	if err != nil {
		log.Println("Error ProtocolVersion: ", err.Error())
		return 0, false
	}
	// Read Server Address
	handshake.ServerAddress, err = ReadString(&reader)
	if err != nil {
		log.Println("Error ServerAddress: ", err.Error())
		return 0, false
	}
	// Read Server Port
	var tmp [2]byte
	if _, err = reader.Read(tmp[:2]); err != nil {
		log.Println("Error ServerPort: ", err.Error())
		return 0, false
	}
	handshake.ServerPort = (uint16(tmp[1]) << 0) | (uint16(tmp[0]) << 8)
	// Read Next State
	handshake.NextState, err = ReadVarInt(&reader)
	if err != nil {
		log.Println("Error nextState: ", err.Error())
		return 0, false
	}

	if config.Verbose {
		log.Println("Got a handshake from ", conn.RemoteAddr().String())
	}
	sendStatus(conn)
	return int32(handshake.NextState), true
}

func sendStatus(conn net.Conn) bool {
	var packetData bytes.Buffer
	var packet bytes.Buffer

	// Write Packet ID (0x00 for status response)
	err := WriteVarInt(&packetData, VarInt(0))
	if err != nil {
		log.Println("Error writing VarInt to buffer", err.Error())
		return false
	}
	// Parse json from query struct
	b, err := json.Marshal(config.Query)
	if err != nil {
		log.Println("Error Parsing query to JSON: ", err.Error())
		return false
	}
	// Write JSON string to packet buffer
	err = WriteString(&packetData, string(b))
	if err != nil {
		log.Println("Error writing query string: ", err.Error())
		return false
	}
	// Write packetData size to packet buffer
	err = WriteVarInt(&packet, VarInt(packetData.Len()))
	if err != nil {
		log.Println("Error writing packetData size: ", err.Error())
		return false
	}
	// Write packetData to packet buffer so that the size is before the rest of the data
	_, err = packetData.WriteTo(&packet)
	if err != nil {
		log.Println("Error writing packetData to the packet: ", err.Error())
		return false
	}
	// Write packet to connection
	_, writeErr := conn.Write(packet.Bytes())
	if writeErr != nil {
		log.Println("Error writing packet to connection: ", writeErr.Error())
		return false
	}
	return true
}

func sendPing(conn net.Conn, reader bufio.Reader) bool {
	var ping int64
	var packet bytes.Buffer
	var packetData bytes.Buffer
	var err error
	//Write packet id to packetData
	err = WriteVarInt(&packetData, VarInt(1))
	if err != nil {
		log.Println("Error writing PacketId to buffer", err.Error())
		return false
	}
	//Read ping long from client
	err = binary.Read(&reader, binary.BigEndian, &ping)
	if err != nil {
		log.Println("Error reading ping from client", err.Error())
		return false
	}
	//Write ping data from client to packetData
	err = binary.Write(&packetData, binary.BigEndian, &ping)
	if err != nil {
		log.Println("Error writing ping to packetData", err.Error())
		return false
	}
	//Set packetData size
	err = WriteVarInt(&packet, VarInt(packetData.Len()))
	if err != nil {
		return false
	}
	//Write packetData to packetData
	_, err = packetData.WriteTo(&packet)
	if err != nil {
		log.Println("Error writing packetData to packet", err.Error())
		return false
	}
	//Write packet to connection
	write, err := conn.Write(packet.Bytes())
	if config.Verbose {
		log.Println("Wrote ", write, " Bytes to ", conn.RemoteAddr().String())
	}
	if err != nil {
		log.Println("Error writing packet to connection: ", err.Error())
		return false
	}
	return true
}

func readConfig() Config {
	f, err := os.ReadFile("config.jsonc")
	if err != nil {
		log.Fatal("Error opening config file: ", err.Error())
	}

	var cfg Config
	parse, err := hujson.Parse(f)
	if err != nil {
		return Config{}
	}
	parse.Standardize()
	err = json.Unmarshal(parse.Pack(), &cfg)
	if err != nil {
		log.Fatal("Error decoding config file: ", err.Error())
	}
	return cfg
}
