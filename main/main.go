package main

import (
	"FancyRPC"
	"log"
	"net"
	"sync"
)

//func startServer(addr chan string) {
//
//	l, err := net.Listen("tcp", ":0")
//	if err != nil {
//		log.Fatal("network error", err)
//	}
//	log.Println("START FancyRPC server on ", l.Addr())
//	addr <- l.Addr().String()
//	FancyRPC.Accept(l)
//
//}

func startServer(addr chan string) {
	var foo Foo
	if err := FancyRPC.Register(&foo); err != nil {
		log.Fatal("register error", err)

	}
	// 选择一个空闲端口
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatal("network error:", err)
	}
	log.Println("start rpc server on ", l.Addr())
	addr <- l.Addr().String()
	FancyRPC.Accept(l)
	log.Println("server accept ok 1")
}

// 被调用的目标函数
// 可以使用 type 关键字来创建自定义类型。
// 在这里，Foo 是一个新的类型，它是基于 int 类型的。
// 这意味着 Foo 类型的变量可以存储整数值，并且可以使用 int 类型的操作和方法来操作这些值
type Foo int
type Args struct {
	Num1, Num2 int
}

func (f Foo) Sum(args Args, reply *int) error {
	*reply = args.Num1 + args.Num2
	return nil
}

//func main() {
//	addr := make(chan string)
//	go startServer(addr)
//	//使用管道确保服务端端口监听成功，客户端再发起请求
//	conn, _ := net.Dial("tcp", <-addr)
//	defer func() { _ = conn.Close() }()
//
//	time.Sleep(time.Second)
//	_ = json.NewEncoder(conn).Encode(FancyRPC.DefaultOption)
//
//	cc := codec.NewGobCodec(conn)
//	for i := 0; i < 5; i++ {
//		h := &codec.Header{
//			ServerMethod: "Foo.Sum",
//			Seq:          uint64(i),
//		}
//		_ = cc.Write(h, fmt.Sprintf("FancyRPC req %d", h.Seq))
//		_ = cc.ReadHeader(h)
//		var reply string
//		_ = cc.ReadBody(&reply)
//		log.Println("reply content:", reply)
//	}
//}

func main() {
	log.SetFlags(2)
	addr := make(chan string) //这里为啥用chan
	go startServer(addr)
	//使用管道确保服务端端口监听成功，客户端再发起请求
	client, _ := FancyRPC.Dial("tcp", <-addr)
	defer func() { _ = client.Close() }()

	//time.Sleep(time.Second)
	var wg sync.WaitGroup
	//typ := reflect.TypeOf(&wg)
	//fmt.Println(typ)
	//通过反射，我们能够非常容易地获取某个结构体的所有方法，并且能够通过方法，获取到该方法所有的参数类型与返回值。例如：
	//for i := 0; i < typ.NumMethod(); i++ {
	//	method := typ.Method(i)
	//	println("函数名")
	//	fmt.Println(method)
	//	argv := make([]string, 0, method.Type.NumIn()) //方法获取函数类型的输入参数数量
	//	returns := make([]string, 0, method.Type.NumOut())
	//
	//	for j := 1; j < method.Type.NumIn(); j++ {
	//		// j 从 1 开始，第 0 个入参是 wg 自己。
	//		argv = append(argv, method.Type.In(j).Name())
	//		log.Printf("函数入参类型%s", method.Type.In(j).Name())
	//
	//	}
	//
	//	for j := 0; j < method.Type.NumOut(); j++ {
	//		returns = append(returns, method.Type.Out(j).Name())
	//		log.Printf("函数出参类型%s", method.Type.Out(j).Name())
	//	}
	//	log.Printf("func (w *%s) %s(%s) %s",
	//		typ.Elem().Name(),
	//		method.Name,
	//		strings.Join(argv, ","),
	//		strings.Join(returns, ","))
	//}
	for i := 0; i < 1; i++ {
		wg.Add(1)
		log.Printf("requst: ----%d-----", i)
		go func(i int) {
			defer wg.Done()
			//args := fmt.Sprintf("rpc req %d", i)
			args := &Args{Num1: i, Num2: i + i}
			var reply int
			if err := client.Call("Foo.Sum.", args, &reply); err != nil {
				log.Printf("call Foo.Sum err %s  %d \n", err, 1)
			} else {
				log.Printf("%d + %d = %d", args.Num1, args.Num2, reply)
			}
		}(i) //定义匿名函数并且传值
	}
	wg.Wait()
}

//通过反射，我们能够非常容易地获取某个结构体的所有方法，并且能够通过方法，获取到该方法所有的参数类型与返回值。例如：
