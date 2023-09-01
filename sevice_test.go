package FancyRPC

import (
	"fmt"
	"reflect"
	"testing"
)

type Foo int
type Args struct{ Num1, Num2 int }

func (f Foo) Sum(args Args, reply *int) error {
	*reply = args.Num2 + args.Num1
	return nil

}

func (f Foo) sum(args Args, reply *int) error {
	*reply = args.Num2 + args.Num1
	return nil
}

func _assert(condition bool, msg string, v ...interface{}) {
	if !condition {
		panic(fmt.Sprintf("assertion failed "+msg, v...))
	}

}

// 测试用例
func TestNewService(t *testing.T) {
	var foo Foo           //这个是回参的接收
	s := newService(&foo) //返回取得的服务指针，并且将该服务注册 ， 入参是interface泛型
	_assert(len(s.method) == 1, "wrong service Method, expect 1, but got %d", len(s.method))
	mType := s.method["Sum"] //取得远程调用函数？？
	_assert(mType != nil, "wrong Method, Sum shouldn't nil")

}
func TestMethodType_Call(t *testing.T) {
	var foo Foo           //这个是回参的接收
	s := newService(&foo) //返回取得的服务指针，并且将该服务注册 ， 入参是interface泛型
	_assert(len(s.method) == 1, "wrong service Method, expect 1, but got %d", len(s.method))
	mType := s.method["Sum"] //取得远程调用函数？？
	argv := mType.newArgv()
	replyv := mType.newReplyv()
	argv.Set(reflect.ValueOf(Args{Num1: 1, Num2: 3}))
	err := s.call(mType, argv, replyv)
	_assert(err == nil && *replyv.Interface().(*int) == 4 && mType.NumCalls() == 1, "failed to call Foo.Sum")
}
