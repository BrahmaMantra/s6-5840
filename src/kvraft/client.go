package kvraft

import (
	"crypto/rand"
	"math/big"
	"time"

	"6.5840/labrpc"
)

const (
	RpcRetryInterval = time.Millisecond * 50 // RPC重试间隔时间
)

// Clerk 结构体表示客户端
type Clerk struct {
	servers    []*labrpc.ClientEnd // 服务器列表
	seq        uint64              // 请求序列号
	identifier int64               // 客户端唯一标识符
	leaderId   int                 // 当前认为的leader服务器ID
}

// 生成一个随机的64位整数作为客户端唯一标识符
func nrand() int64 {
	max := big.NewInt(int64(1) << 62)
	bigx, _ := rand.Int(rand.Reader, max)
	x := bigx.Int64()
	return x
}

// 获取当前请求的序列号，并将序列号加1
func (ck *Clerk) GetSeq() (SendSeq uint64) {
	SendSeq = ck.seq
	ck.seq += 1
	return
}

// 创建一个新的Clerk实例
func MakeClerk(servers []*labrpc.ClientEnd) *Clerk {
	ck := new(Clerk)
	ck.servers = servers
	ck.identifier = nrand()
	ck.seq = 0
	return ck
}

// 获取指定键的当前值。
// 如果键不存在，则返回空字符串。
// 在遇到所有其他错误时会不断重试。
func (ck *Clerk) Get(key string) string {
	args := &GetArgs{Key: key, Seq: ck.GetSeq(), Identifier: ck.identifier}

	for {
		reply := &GetReply{}
		ok := ck.servers[ck.leaderId].Call("KVServer.Get", args, reply)
		if !ok || reply.Err == ErrNotLeader || reply.Err == ErrLeaderOutDated {
			if !ok {
				reply.Err = ERRRPCFailed
			}
			if reply.Err != ErrNotLeader {
				DPrintf("clerk %v Seq %v 重试Get(%v), Err=%s", args.Identifier, args.Key, args.Key, reply.Err)
			}

			ck.leaderId += 1
			ck.leaderId %= len(ck.servers)
			time.Sleep(RpcRetryInterval)
			continue
		}

		switch reply.Err {
		case ErrChanClose:
			DPrintf("clerk %v Seq %v 重试Get(%v), Err=%s", args.Identifier, args.Key, args.Key, reply.Err)
			time.Sleep(time.Microsecond * 5)
			continue
		case ErrHandleOpTimeOut:
			DPrintf("clerk %v Seq %v 重试Get(%v), Err=%s", args.Identifier, args.Key, args.Key, reply.Err)
			time.Sleep(RpcRetryInterval)
			continue
		case ErrKeyNotExist:
			DPrintf("clerk %v Seq %v 成功: Get(%v)=%v, Err=%s", args.Identifier, args.Key, args.Key, reply.Value, reply.Err)
			return reply.Value
		}
		DPrintf("clerk %v Seq %v 成功: Get(%v)=%v, Err=%s", args.Identifier, args.Key, args.Key, reply.Value, reply.Err)

		return reply.Value
	}
}

// 共享的Put和Append方法。
// 发送一个RPC请求。
func (ck *Clerk) PutAppend(key string, value string, op string) {
	args := &PutAppendArgs{Key: key, Value: value, Op: op, Seq: ck.GetSeq(), Identifier: ck.identifier}

	for {
		reply := &PutAppendReply{}
		ok := ck.servers[ck.leaderId].Call("KVServer.PutAppend", args, reply)
		if !ok || reply.Err == ErrNotLeader || reply.Err == ErrLeaderOutDated {
			if !ok {
				reply.Err = ERRRPCFailed
			}
			if reply.Err != ErrNotLeader {
				DPrintf("clerk %v Seq %v 重试%s(%v, %v), Err=%s", args.Identifier, args.Key, args.Op, args.Key, args.Value, reply.Err)
			}

			ck.leaderId += 1
			ck.leaderId %= len(ck.servers)
			time.Sleep(RpcRetryInterval)
			continue
		}

		switch reply.Err {
		case ErrChanClose:
			DPrintf("clerk %v Seq %v 重试%s(%v, %v), Err=%s", args.Identifier, args.Key, args.Op, args.Key, args.Value, reply.Err)
			time.Sleep(RpcRetryInterval)
			continue
		case ErrHandleOpTimeOut:
			DPrintf("clerk %v Seq %v 重试%s(%v, %v), Err=%s", args.Identifier, args.Key, args.Op, args.Key, args.Value, reply.Err)
			time.Sleep(RpcRetryInterval)
			continue
		}
		DPrintf("clerk %v Seq %v 成功: %s(%v, %v), Err=%s", args.Identifier, args.Key, args.Op, args.Key, args.Value, reply.Err)

		return
	}
}

// Put方法，调用PutAppend方法实现
func (ck *Clerk) Put(key string, value string) {
	ck.PutAppend(key, value, "Put")
}

// Append方法，调用PutAppend方法实现
func (ck *Clerk) Append(key string, value string) {
	ck.PutAppend(key, value, "Append")
}
