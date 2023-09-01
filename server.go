package FancyRPC

import (
	"FancyRPC/codec"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"io"
	"log"
	"net"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
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
	serviceMap sync.Map
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
	//处理请求是并发的，但是回复请求的报文必须是逐个发送的，并发容易导致多个回复报文交织在一起，客户端无法解析。在这里使用锁(sending)保证
	wg := &sync.WaitGroup{} //返回wait对象的指针
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
	argv, replyv reflect.Value //本来就是反射类型
	mtype        *methodType
	svc          *service
}

// 协议解析
func (server *Server) readRequestHeader(cc codec.Codec) (*codec.Header, error) {
	var h codec.Header

	if err := cc.ReadHeader(&h); err != nil {
		if err != io.EOF && err != io.ErrUnexpectedEOF {
			log.Println("rpc server :read header ok")
		}
		log.Printf("readRequestHeader err:%s\n", err)
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
	req := &request{h: h} //结构体指针中 有请求头指针,
	req.svc, req.mtype, err = server.findService(h.ServiceMethod)
	if err != nil {
		log.Printf("find service error in readRequest%s:\n", err)
		return req, err
	}

	req.argv = req.mtype.newArgv()
	req.replyv = req.mtype.newReplyv()

	//确保传入的是指针
	argvi := req.argv.Interface()
	if req.argv.Type().Kind() != reflect.Ptr {
		argvi = req.argv.Addr().Interface()
	}

	req.argv = reflect.New(reflect.TypeOf(""))
	// 这里为什么传interface,可以储存不同类型的值，泛型字段
	if err = cc.ReadBody(argvi); err != nil {
		log.Println("rpc server: read argv err:", err)

	}
	return req, nil
}

func (server *Server) sendResponse(cc codec.Codec, h *codec.Header, body interface{}, sending *sync.Mutex) {
	//log.Println("加 sendResponse ending.Lock() 锁")
	// 发送之前对
	sending.Lock()
	defer sending.Unlock()
	if err := cc.Write(h, body); err != nil {
		log.Println("rpc server:write response error", err)
	}
	//sending.Unlock()
}

func (server *Server) handleRequest(cc codec.Codec, req *request, sending *sync.Mutex, wg *sync.WaitGroup) {

	defer wg.Done()
	//log.Println(*req.h, req.argv.Elem())
	//req.replyv = reflect.ValueOf(1)
	//目前只处理返回序列

	//调用目标函数
	err := req.svc.call(req.mtype, req.argv, req.replyv)
	if err != nil {
		req.h.Error = err.Error()
		server.sendResponse(cc, req.h, invalidRequest, sending)
		return
	}
	req.replyv = reflect.ValueOf(fmt.Sprintf("FancyRpc resp %d", req.h.Seq))
	//req.replyv = fmt.Sprintf("FancyRpc resp %d", req.h.Seq)
	// req.replyv.Interface() 泛型，可以是任何类型
	//server.sendResponse(cc, req.h, req.replyv.Interface(), sending)

	// req.replyv.Interface() 泛型，可以是任何类型
	server.sendResponse(cc, req.h, req.replyv.Interface(), sending)

}

// 在给定的代码中，req.replyv是一个reflect.Value类型的对象，通过调用Interface()方法，可以将其底层值转换为interface{}类型。
// 这样做的意义在于，可以将不同类型的值传递给sendResponse方法，而不需要显式地指定具体的类型。这种灵活性使得代码可以处理各种类型的响应值，而不需要为每种类型编写不同的处理逻辑。
// 总结来说，使用reflect.ValueOf和Interface()可以在运行时动态地处理不同类型的值，提供了更大的灵活性和通用性。这对于需要处理未知类型或根据条件进行不同操作的情况非常有用
type methodType struct {
	method    reflect.Method
	ArgType   reflect.Type
	ReplyType reflect.Type
	numCalls  uint64
}

func (m *methodType) NumCalls() uint64 {
	return atomic.LoadUint64(&m.numCalls) //原子操作，利用atomic库

}

func (m *methodType) newArgv() reflect.Value {
	var argv reflect.Value
	//指针类型和值类型创建实例的方式有细微区别
	//该方法用于根据 methodType 结构体中的 ArgType 字段的类型信息 创建一个新的参数对象。
	//if m.ArgType.Kind() == reflect.Ptr {
	//	// 如果是指针类型则创建一个
	//	//新的指针类型对象赋值给argv
	//	argv = reflect.New(m.ArgType.Elem())
	//
	//} else {
	// 如果非指针则返回对象
	argv = reflect.New(m.ArgType).Elem()
	//}
	return argv

}

func (m *methodType) newReplyv() reflect.Value {
	replyv := reflect.New(m.ReplyType.Elem())
	switch m.ReplyType.Elem().Kind() {
	case reflect.Map:
		replyv.Elem().Set(reflect.MakeMap(m.ReplyType.Elem()))
	case reflect.Slice:
		replyv.Elem().Set(reflect.MakeSlice(m.ReplyType.Elem(), 0, 0))

	}
	return replyv

}

type service struct {
	name   string                 //name 即映射的结构体的名称，比如 T，比如 WaitGroup
	typ    reflect.Type           ///typ 是结构体的类型
	rcvr   reflect.Value          //rcvr 即结构体的实例本身
	method map[string]*methodType //method 是 map 类型，存储映射的结构体的所有符合条件的方法。
}

func (server *Server) Register(rcvr interface{}) error {
	s := newService(rcvr)

	// LoadOrStore 存在则加载dup为true，不存在侧存储dup为false
	if _, dup := server.serviceMap.LoadOrStore(s.name, s); dup {
		return errors.New("rpc: service already defined" + s.name)
	} else {
		log.Println("service register ok" + s.name)
	}
	return nil
}
func Register(rcvr interface{}) error { return DefaultServer.Register(rcvr) }

func (server *Server) findService(serviceMethod string) (svc *service, mtype *methodType, err error) {
	//为 ServiceMethod 的构成是 “Service.Method”，因此先将其分割成 2 部分，第一部分是 Service 的名称，
	//第二部分即方法名。现在 serviceMap 中找到对应的 service 实例，再从 service 实例的 method 中，找到对应的 methodType。
	dot := strings.LastIndex(serviceMethod, ".")
	fmt.Println("serviceMethod!!!!!", serviceMethod)
	if dot < 0 {
		err = errors.New("rpc server: service/method request ill-formed: " + serviceMethod)
		return
	}

	serverName, methodName := serviceMethod[:dot], serviceMethod[dot+1:]
	svci, ok := server.serviceMap.Load(serverName)
	if !ok {
		err = errors.New("rpc server:cant find service: " + serviceMethod)
		return
	}
	svc = svci.(*service)
	mtype = svc.method[methodName]
	if mtype == nil {
		err = errors.New("rpc server: can't find method " + methodName)
	}
	return

}

func (s *service) registMethod() {
	s.method = make(map[string]*methodType)
	for i := 0; i < s.typ.NumMethod(); i++ {
		method := s.typ.Method(i)
		mType := method.Type
		if mType.NumIn() != 3 || mType.NumOut() != 1 {
			continue
		}
		if mType.Out(0) != reflect.TypeOf((*error)(nil)).Elem() {
			continue
		}
		argType, replyType := mType.In(1), mType.In(2)
		if !isExportedOrBuiltinType(argType) || !isExportedOrBuiltinType(replyType) {
			continue
		}
		s.method[method.Name] = &methodType{
			method:    method,
			ArgType:   argType,
			ReplyType: replyType,
		}
		log.Printf("rpc server: register %s.%s\n", s.name, method.Name)
	}
}
func isExportedOrBuiltinType(t reflect.Type) bool {
	return ast.IsExported(t.Name()) || t.PkgPath() == ""
}

func newService(rcvr interface{}) *service {
	///client 需要首先调用newService 来获取Service 可用的method
	s := &service{}
	//回参结构rcvr,处理到service 结构体中，做一个service实例
	s.rcvr = reflect.ValueOf(rcvr)
	s.name = reflect.Indirect(s.rcvr).Type().Name()
	s.typ = reflect.TypeOf(rcvr)

	if !ast.IsExported(s.name) {
		log.Fatal(fmt.Sprintf("rpc server: %s is not a valid service name", s.name))
	}
	s.registMethod() //注册该服务下所有方法
	return s
}

// 即能够通过反射值调用方法
func (s *service) call(m *methodType, argv, replyv reflect.Value) error {
	atomic.AddUint64(&m.numCalls, 1)
	f := m.method.Func
	returnValues := f.Call([]reflect.Value{s.rcvr, argv, replyv})
	if errInter := returnValues[0].Interface(); errInter != nil {
		return errInter.(error)
	}
	return nil
}
