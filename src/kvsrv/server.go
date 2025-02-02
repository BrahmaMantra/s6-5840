package kvsrv

import (
	"log"
	"sync"
)

const Debug = false

func DPrintf(format string, a ...interface{}) (n int, err error) {
	if Debug {
		log.Printf(format, a...)
	}
	return
}

type IdCount struct {
	ClerkId int64
	Count   int64
}
type KVServer struct {
	mu sync.Mutex
	// Your definitions here.
	clerkId_count map[int64]int64
	_map          map[string]string

	opReply map[IdCount]string //维护每个请求应该收到的回复
}

func (kv *KVServer) Get(args *GetArgs, reply *GetReply) {
	// Your code here.
	kv.mu.Lock()
	defer kv.mu.Unlock()
	val, ok := kv._map[args.Key]
	if !ok {
		reply.Value = ""
		return
	}
	reply.Value = val
}

func (kv *KVServer) Put(args *PutAppendArgs, reply *PutAppendReply) {
	// Your code here.
	kv.mu.Lock()
	defer kv.mu.Unlock()
	var except_count int64
	// 检查新客户端
	now_count, ok := kv.clerkId_count[args.ClerkId]
	if !ok {
		kv.clerkId_count[args.ClerkId] = 0
		now_count = 0
		except_count = 0
	} else {
		except_count = now_count + 1
	}
	// fmt.Printf("Put()::Input count = %d , key = %s , value = %s\n", args.Count, args.Key, args.Value)
	// fmt.Printf("Put()::Inow_count = %d , except_count = %d\n", now_count, except_count)
	if args.Count != except_count {
		reply.Value = kv._map[args.Key]
		reply.Vaild_op = false
	} else {
		kv._map[args.Key] = args.Value
		kv.clerkId_count[args.ClerkId] = except_count
		reply.Vaild_op = true
		// fmt.Printf("Successfully put key = %s , value = %s , clerkId = %d\n", args.Key, args.Value, args.ClerkId)
	}
}

func (kv *KVServer) Append(args *PutAppendArgs, reply *PutAppendReply) {
	// Your code here.
	kv.mu.Lock()
	defer kv.mu.Unlock()
	var except_count int64
	// 检查新客户端
	now_count, ok := kv.clerkId_count[args.ClerkId]
	if !ok {
		kv.clerkId_count[args.ClerkId] = 0
		now_count = 0
		except_count = 0
	} else {
		except_count = now_count + 1
	}
	// fmt.Printf("Append()::Input count = %d , key = %s , value = %s\n", args.Count, args.Key, args.Value)
	// fmt.Printf("Append()::now_count = %d , except_count = %d\n", now_count, except_count)
	// time.Sleep(1 * time.Second)
	// fmt.Printf("Append()::args.Value = %s ,args.ClerkId = %d, args.Count = %d\n", args.Value, args.ClerkId, args.Count)
	if args.Count != except_count {
		reply.Value = kv.opReply[IdCount{args.ClerkId, args.Count}]
		// fmt.Printf("!=Apppend()::reply.Value = %s\n", reply.Value)
		reply.Vaild_op = false
	} else {
		reply.Value = kv._map[args.Key]
		// fmt.Printf("Append()::reply.Value = %s\n", reply.Value)
		kv.opReply[IdCount{args.ClerkId, args.Count}] = kv._map[args.Key]
		kv._map[args.Key] += args.Value
		kv.clerkId_count[args.ClerkId] = except_count
		delete(kv.opReply, IdCount{args.ClerkId, args.Count - 1})
		reply.Vaild_op = true
	}
}

func StartKVServer() *KVServer {
	kv := new(KVServer)

	// You may need initialization code here.
	kv._map = make(map[string]string)
	kv.clerkId_count = make(map[int64]int64)
	kv.opReply = make(map[IdCount]string)
	return kv
}
