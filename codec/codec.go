package codec

import "io"

// Header 客户端调用方式
//
// err = client.Call("Arith.Multiply", args, &reply)
// 所以定义下面的结构体

type Header struct {
	ServerMethod  string // ServiceMethod 是服务名和方法名，通常与 Go 语言中的结构体和方法相映射。
	Seq           uint64 // Seq 是请求的序号，也可以认为是某个请求的 ID，用来区分不同的请求。
	Error         string // Error 是错误信息，客户端置为空，服务端如果如果发生错误，将错误信息置于 Error 中。
	ServiceMethod string
}

// Codec codec 模块用于对pb的编码解码，对传输的内容做了规范，抽象出接口
// 客户端和服务端可以通过 Codec 的 Type 得到构造函数，从而创建 Codec 实例
// 这部分代码和工厂模式类似，与工厂模式不同的是，返回的是构造函数，而非实例。
type Codec interface {
	io.Closer
	ReadHeader(*Header) error
	ReadBody(interface{}) error
	Write(*Header, interface{}) error
}

// NewCodecFunc NewCodecFunc是一个函数类型，它定义了一个用于创建Codec对象的函数。
// 该函数接收一个io.ReadWriteCloser类型的参数，用于创建一个新的Codec对象，并返回该对象的指针。
type NewCodecFunc func(closer io.ReadWriteCloser) Codec

// 这里是两种codec
const (
	GobType  string = "application/gob"
	JsonType string = "application/json"
)

// NewCodecFuncMap 映射关系
var NewCodecFuncMap map[string]NewCodecFunc

func init() {
	NewCodecFuncMap = make(map[string]NewCodecFunc)
	NewCodecFuncMap[GobType] = NewGobCodec // 客户端和服务端可以通过 Codec 的 Type 得到构造函数
}
