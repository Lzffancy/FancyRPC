package codec

//gob 协议

import (
	"bufio"
	"encoding/gob"
	"io"
	"log"
)

// GobCodec gob 二进制协议
type GobCodec struct {
	conn io.ReadWriteCloser //或者 Unix 建立 socket 时得到的链接实例
	buf  *bufio.Writer      //缓存写入的数据
	dec  *gob.Decoder       //用于解码数据
	enc  *gob.Encoder       //用于编码数据
}

var _ Codec = (*GobCodec)(nil) //通过将(*GobCodec)(nil)赋值给_变量，我们可以检查GobCodec类型是否实现了Codec接口
//它声明了一个名为_的变量，该变量的类型是Codec，并将(*GobCodec)(nil)赋值给了它。
//在Go语言中，_是一个特殊的标识符，用于表示一个不需要使用的变量或值。在这个声明语句中，_表示一个不需要使用的变量，它的作用是用于检查GobCodec类型是否实现了Codec接口。
// 也就是将Codec 实例化一下，看看GobCodec的实现是否满足了方法
// 这里就必须要实现 ReadHeader ReadBody Write

// NewGobCodec GobCodec 结构体(类) 实现了Codec接口规定的所有方法,故该NewGobCodec函数可以返回Codec
func NewGobCodec(conn io.ReadWriteCloser) Codec {
	buf := bufio.NewWriter(conn)
	return &GobCodec{
		conn: conn,
		buf:  buf,
		dec:  gob.NewDecoder(conn),
		enc:  gob.NewEncoder(buf),
	}
}

//实现 ReadHeader、ReadBody、Write 和 Close 方法。实现了这些方法后GobCodec会成为Codec接口类型的子类

func (c *GobCodec) ReadHeader(h *Header) error {
	return c.dec.Decode(h)
}

func (c *GobCodec) ReadBody(body interface{}) error {
	return c.dec.Decode(body)
}

func (c *GobCodec) Write(h *Header, body interface{}) (err error) {
	defer func() {
		_ = c.buf.Flush() //buf 是为了防止阻塞而创建的带缓冲的 Writer，一般这么做能提升性能。
		if err != nil {
			_ = c.Close()
		}
	}()
	if err := c.enc.Encode(h); err != nil {
		log.Println("rpc codec:gob error encoding header:", err)
		return err
	}

	if err := c.enc.Encode(body); err != nil {
		log.Println("rpc codec:gob error encoding body:", err)
		return err
	}
	return nil
}
func (c *GobCodec) Close() error {
	return c.conn.Close()
}
