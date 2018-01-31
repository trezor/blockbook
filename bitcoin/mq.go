package bitcoin

import (
	"encoding/binary"

	"github.com/golang/glog"
	zmq "github.com/pebbe/zmq4"
)

// MQ is message queue listener handle
type MQ struct {
	context   *zmq.Context
	socket    *zmq.Socket
	isRunning bool
	finished  chan bool
}

// MQMessage contains data received from Bitcoind message queue
type MQMessage struct {
	Topic    string
	Sequence uint32
	Body     []byte
}

// New creates new Bitcoind ZeroMQ listener
// callback function receives messages
func New(binding string, callback func(*MQMessage)) (*MQ, error) {
	context, err := zmq.NewContext()
	if err != nil {
		return nil, err
	}
	socket, err := context.NewSocket(zmq.SUB)
	if err != nil {
		return nil, err
	}
	socket.SetSubscribe("hashblock")
	socket.SetSubscribe("hashtx")
	// for now do not use raw subscriptions - we would have to handle skipped/lost notifications from zeromq
	// on each notification we do sync or syncmempool respectively
	// socket.SetSubscribe("rawblock")
	// socket.SetSubscribe("rawtx")
	socket.Connect(binding)
	glog.Info("MQ listening to ", binding)
	mq := &MQ{context, socket, true, make(chan bool)}
	go mq.run(callback)
	return mq, nil
}

func (mq *MQ) run(callback func(*MQMessage)) {
	mq.isRunning = true
	for {
		msg, err := mq.socket.RecvMessageBytes(0)
		if err != nil {
			if zmq.AsErrno(err) == zmq.Errno(zmq.ETERM) {
				close(mq.finished)
				glog.Info("MQ loop terminated")
				break
			}
			glog.Error("MQ RecvMessageBytes error ", err)
		}
		if msg != nil && len(msg) >= 3 {
			sequence := uint32(0)
			if len(msg[len(msg)-1]) == 4 {
				sequence = binary.LittleEndian.Uint32(msg[len(msg)-1])
			}
			m := &MQMessage{
				Topic:    string(msg[0]),
				Sequence: sequence,
				Body:     msg[1],
			}
			callback(m)
		}
	}
	mq.isRunning = false
}

// Shutdown stops listening to the ZeroMQ and closes the connection
func (mq *MQ) Shutdown() error {
	glog.Info("MQ server shutdown")
	if mq.isRunning {
		// if errors in socket.Close or context.Term, let it close ungracefully
		if err := mq.socket.Close(); err != nil {
			return err
		}
		if err := mq.context.Term(); err != nil {
			return err
		}
		_, _ = <-mq.finished
		glog.Info("MQ server shutdown finished")
	}
	return nil
}
