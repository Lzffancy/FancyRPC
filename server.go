package FancyRPC

import (
	"FancyRPC/codec"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"reflect"
	"sync"
)

//body 的格式和长度通过 header 中的 Content-Type 和 Content-Length 指定
//
//服务端通过解析 header 就能够知道如何从 body 中读取需要的信息。对于 RPC 协议来说，这部分协商是需要自主设计的。
//为了提升性能，一般在报文的最开始会规划固定的字节
//来协商相关的信息。比如第1个字节用来表示序列化方式，第2个字节表示压缩方式，第3-6字节表示 header 的长度，7-10 字节表示 body 的长度。

const MagicNumber = 0x3befc

type Option struct {
	MagicNumber int
	CodecType   string
}

var DefaultOption = &Option{
	MagicNumber: MagicNumber,
	CodecType:   codec.GobType,
}

//涉及协议协商的这部分信息，需要设计固定的字节来传输的
//客户端固定采用 JSON 编码 Option，后续的 header 和 body 的编码方式由 Option 中的 CodeType 指定，
//服务端首先使用 JSON 解码 Option，然后通过 Option 的 CodeType 解码剩余的内容。即报文将以这样的形式发送
//| Option | Header1 | Body1 | Header2 | Body2 | ...

// Server 以上为协议相关内容
// 以下为server
type Server struct {
}

func NewServer() *Server {
	return &Server{}
}

var DefaultServer = NewServer()

// socket 过程
// 它定义了一个名为Accept的方法，该方法属于Server类型，并且接收一个net.Listener类型的参数lis。

// Accept 在Go语言中，方法可以定义在结构体类型上，也可以定义在结构体类型的指针类型上。
// 使用指针类型作为接收者类型可以避免在方法调用时进行值拷贝，从而提高程序的性能和效率。
func (server *Server) Accept(lis net.Listener) {
	// Accept accepts connections on the listener and serves requests
	// for each incoming connection.
	for {
		conn, err := lis.Accept()
		if err != nil {
			log.Println("rpc server accept error", err)
			return
		}
		go server.ServerConn(conn)
	}
}

func Accept(lis net.Listener) {
	DefaultServer.Accept(lis)

}

// ServerConn ServeConn runs the server on a single connection.
// ServeConn blocks, serving the connection until the client hangs up. 服务会阻塞直到客户端挂起
func (server *Server) ServerConn(conn io.ReadWriteCloser) {
	defer func() { _ = conn.Close() }()
	var opt Option
	if err := json.NewDecoder(conn).Decode(&opt); err != nil {
		log.Println("rpc server :option error ", err)
		return
	}

	if opt.MagicNumber != MagicNumber {
		log.Println("rpc server :MagicNumber invalid ")
		return

	}
	f := codec.NewCodecFuncMap[opt.CodecType]
	if f == nil {
		log.Println("rpc server :CodecType invalid ")
		return
	}
	server.ServerCodec(f(conn))
}

var invalidRequest = struct {
}{}

func (server *Server) ServerCodec(cc codec.Codec) {
	sending := new(sync.Mutex)
	wg := new(sync.WaitGroup)
	for {
		req, err := server.readRequest(cc)
		if err != nil {
			if req == nil {
				break
			}
			req.h.Error = err.Error()
			server.sendResponse(cc, req.h, invalidRequest, sending)
			continue
		}
		wg.Add(1) //一个请求可以包含多个req
		go server.handleRequest(cc, req, sending, wg)
	}
	wg.Wait()
	_ = cc.Close()
}

type request struct {
	h            *codec.Header
	argv, replyv reflect.Value
}

// 协议解析
func (server *Server) readRequestHeader(cc codec.Codec) (*codec.Header, error) {
	var h codec.Header
	if err := cc.ReadHeader(&h); err != nil {
		if err != io.EOF && err != io.ErrUnexpectedEOF {
			log.Println("rpc server :read header ")
		}
		return nil, err
	}
	//如解析成功则返回h的指针，其指针类型是*codec.Header
	return &h, nil
}

func (server *Server) readRequest(cc codec.Codec) (*request, error) {
	h, err := server.readRequestHeader(cc) //返回请求头的指针
	if err != nil {
		return nil, err
	}
	req := &request{h: h} //结构体指针中有请求头指针

	req.argv = reflect.New(reflect.TypeOf(""))
	// 这里为什么传interface,可以储存不同类型的值，泛型字段
	if err = cc.ReadBody(req.argv.Interface()); err != nil {
		log.Println("rpc server: read argv err:", err)

	}
	return req, nil
}

func (server *Server) sendResponse(cc codec.Codec, h *codec.Header, body interface{}, sending *sync.Mutex) {
	sending.Lock()
	defer sending.Unlock()
	if err := cc.Write(h, body); err != nil {
		log.Println("rpc server:write response error", err)
	}
}

func (server *Server) handleRequest(cc codec.Codec, req *request, sending *sync.Mutex, wg *sync.WaitGroup) {

	defer wg.Done()
	log.Println(*req.h, req.argv.Elem())
	req.replyv = reflect.ValueOf(fmt.Sprintf("FancyRpc resp %d", req.h.Seq))
	// req.replyv.Interface() 泛型，可以是任何类型
	server.sendResponse(cc, req.h, req.replyv.Interface(), sending)
}
