package bitcoin

import (
	"encoding/binary"
	"encoding/hex"
	"log"

	zmq "github.com/pebbe/zmq4"
)

func ZeroMQ(binding string) {
	context, err := zmq.NewContext()
	if err != nil {
		log.Fatal(err)
	}
	socket, err := context.NewSocket(zmq.SUB)
	if err != nil {
		log.Fatal(err)
	}
	socket.SetSubscribe("hashblock")
	socket.SetSubscribe("hashtx")
	socket.SetSubscribe("rawblock")
	socket.SetSubscribe("rawtx")
	socket.Connect(binding)
	defer socket.Close()
	for i := 0; i < 101; i++ {
		msg, err := socket.RecvMessageBytes(0)
		if err != nil {
			log.Fatal(err)
		}
		topic := string(msg[0])
		body := hex.EncodeToString(msg[1])
		sequence := uint32(0)
		if len(msg[len(msg)-1]) == 4 {
			sequence = binary.LittleEndian.Uint32(msg[len(msg)-1])
		}
		log.Printf("%s-%d (%v)  %s", topic, sequence, msg[len(msg)-1], body)
	}
}
