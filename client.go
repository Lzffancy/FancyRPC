package FancyRPC

import (
	"FancyRPC/codec"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
)

type Call struct {
	Seq uint64

	Args          interface{}
	Rely          interface{}
	Error         error
	Done          chan *Call
	ServiceMethod string
}

// 为了支持异步调用，Call 结构体中添加了一个字段 Done，
// Done 的类型是 chan *Call，当调用结束时，会调用 call.done() 通知调用方。
func (call *Call) done() {
	call.Done <- call
}

type Client struct {
	cc      codec.Codec //cc 是消息的编解码器，和服务端类似，用来序列化将要发送出去的请求，以及反序列化接收到的响应。
	opt     *Option
	sending sync.Mutex   //sending 是一个互斥锁，和服务端类似，为了保证请求的有序发送，即防止出现多个请求报文混淆。
	header  codec.Header //header 是每个请求的消息头，header 只有在请求发送时才需要，
	// 而请求发送是互斥的，因此每个客户端只需要一个，声明在 Client 结构体中可以复用。
	mu       sync.Mutex //为了保护pending map
	pending  map[uint64]*Call
	closing  bool
	shutdown bool
	seq      uint64
}

var _ io.Closer = (*Client)(nil)

var ErrShutdown = errors.New("connection is shut down")

func (client *Client) Close() error {
	//client.mu.Lock()
	//defer client.mu.Unlock()
	if client.closing {
		return ErrShutdown
	}
	client.closing = true
	return client.cc.Close()

}

func (client *Client) IsAvailable() bool {
	//client.mu.Lock()
	//defer client.mu.Unlock()

	return !client.shutdown && !client.closing

}

// ----------------------------
func (client *Client) registerCall(call *Call) (uint64, error) {
	client.mu.Lock() //锁为了保护修改client.pending[call.Seq] map
	defer client.mu.Unlock()
	if client.closing || client.shutdown {
		return 0, ErrShutdown

	}
	call.Seq = client.seq
	client.pending[call.Seq] = call
	client.seq++
	return call.Seq, nil

}
func (client *Client) removeCall(seq uint64) *Call {
	client.mu.Lock()
	defer client.mu.Unlock()
	call := client.pending[seq]
	delete(client.pending, seq)
	return call // 这里为什么还返回call，这是removeCall方法
}

func (client *Client) terminateCalls(err error) {
	client.sending.Lock()
	defer client.sending.Unlock()
	client.mu.Lock()
	defer client.mu.Unlock()
	client.shutdown = true

	for _, call := range client.pending {
		call.Error = err
		call.done()
	}
}

func (client *Client) receive() {
	var err error
	for err == nil {
		var h codec.Header
		if err = client.cc.ReadHeader(&h); err != nil {
			log.Printf("receive ReadHeader err : %s", err)
			break
		}
		call := client.removeCall(h.Seq)
		switch {
		case call == nil:
			err = client.cc.ReadBody(nil)

		case h.Error != "":
			call.Error = fmt.Errorf(h.Error)
			err = client.cc.ReadBody(nil)
			call.done()
		default:
			err = client.cc.ReadBody(call.Rely)
			if err != nil {
				call.Error = errors.New("reading body" + err.Error())
			}
			call.done()
		}
	}
	client.terminateCalls(err)
}

func NewClient(conn net.Conn, opt *Option) (*Client, error) {

	f := codec.NewCodecFuncMap[opt.CodecType]
	log.Printf("client.go.NewClient %s \n", opt.CodecType)
	if f == nil {
		err := fmt.Errorf("invalid codec type %s", opt.CodecType)
		log.Println("rpc client :codec error:", err)
		return nil, err

	}
	if err := json.NewEncoder(conn).Encode(opt); err != nil {
		log.Println("rpc client options error: ", err)
		_ = conn.Close()
		return nil, err
	}
	log.Println("NewClient ok!")
	return newClientCodec(f(conn), opt), nil
}

func newClientCodec(cc codec.Codec, opt *Option) *Client {
	var client = &Client{
		seq:     1,
		cc:      cc,
		opt:     opt,
		pending: make(map[uint64]*Call),
	}
	go client.receive()
	return client
}

func parseOptions(opts ...*Option) (*Option, error) {
	if len(opts) == 0 || opts[0] == nil {
		return DefaultOption, nil
	}

	if len(opts) != 1 {
		return nil, errors.New("number of options is more than 1")
	}
	opt := opts[0]
	opt.MagicNumber = DefaultOption.MagicNumber
	if opt.CodecType == "" {
		opt.CodecType = DefaultOption.CodecType
	}
	log.Println("11111111111111111111111111111111111")
	log.Printf("parseOptions %s\n", opt.CodecType)
	return opt, nil
}

func Dial(network, address string, opts ...*Option) (client *Client, err error) {
	opt, err := parseOptions(opts...)
	if err != nil {
		return nil, err
	}
	conn, err := net.Dial(network, address)
	if err != nil {
		return nil, err
	}

	defer func() {
		if client == nil {
			_ = conn.Close()
		}
	}()

	return NewClient(conn, opt)
}

func (client *Client) send(call *Call) {
	client.sending.Lock()
	defer client.sending.Unlock()

	seq, err := client.registerCall(call)
	if err != nil {
		call.Error = err
		call.done()
		return
	}

	client.header.ServerMethod = call.ServiceMethod
	client.header.Seq = seq
	client.header.Error = ""

	if err := client.cc.Write(&client.header, call.Args); err != nil {
		call := client.removeCall(seq)
		if call != nil {
			call.Error = err
			call.done()
		}
	}

}

// Go 暴露给客户但调用,异步接口
func (client *Client) Go(serviceMethod string, args, reply interface{}, done chan *Call) *Call {

	if done == nil {
		done = make(chan *Call, 10)

	} else if cap(done) == 0 {
		log.Println("rpc client done channel is unbuffered")

	}
	call := &Call{
		ServiceMethod: serviceMethod,
		Args:          args,
		Rely:          reply,
		Done:          done,
	}
	client.send(call)
	return call

}

// Call 暴露给客户但调用,同步接口
func (client *Client) Call(serviceMethod string, args, reply interface{}) error {
	call := <-client.Go(serviceMethod, args, reply, make(chan *Call, 1)).Done
	return call.Error
}
