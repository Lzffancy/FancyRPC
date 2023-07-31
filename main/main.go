package main

import (
	"FancyRPC"
	"FancyRPC/codec"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"time"
)

func startServer(addr chan string) {

	l, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatal("network error", err)
	}
	log.Println("START FancyRPC server on ", l.Addr())
	addr <- l.Addr().String()
	FancyRPC.Accept(l)

}

func main() {
	addr := make(chan string)
	go startServer(addr)
	//使用管道确保服务端端口监听成功，客户端再发起请求
	conn, _ := net.Dial("tcp", <-addr)
	defer func() { _ = conn.Close() }()

	time.Sleep(time.Second)
	_ = json.NewEncoder(conn).Encode(FancyRPC.DefaultOption)

	cc := codec.NewGobCodec(conn)
	for i := 0; i < 5; i++ {
		h := &codec.Header{
			ServerMethod: "Foo.Sum",
			Seq:          uint64(i),
		}
		_ = cc.Write(h, fmt.Sprintf("FancyRPC req %d", h.Seq))
		_ = cc.ReadHeader(h)
		var reply string
		_ = cc.ReadBody(&reply)
		log.Println("reply content:", reply)
	}
}
