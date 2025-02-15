## KV服务器的流程
### Get/Put请求
#### 概括
Clerk Machine也就是user发送一个KV请求，我们的状态复制机State Machine Replica接收这个请求cmd，并且通过Start函数交给下层raft处理，其中kvserver和raft通过applyCh管道进行通信，待管道有可读数据的时候取出。

#### 实现路径
1. kv.Get(args,reply)/kv.PutAppend(args,reply)
2. kv.HandleOp(opArgs) :
    1. 从historyMap和lastSeq判断是否是历史记录或者过时的RPC
    2. 调用Start()和raft进行通信
    3. 通过select case等到ApplyHandler处理的消息到达，管道存储在waiCh
3. kv.ApplyHandler: 是一个一直在的gorutinue，内部逻辑比较复杂
    1. 从applyCh里面接收一个log，合法的话就更新kv.lastApplied
    2. 判断请求是否needApply，如果需要就调用res = kv.DBExecute(&op)，并且更新历史记录
    3. 将上述res写到waiCh，发送给HandleOp进行处理
    4. 检测是否需要生成快照
4.  kv.DBExecute: 根据op类型，进行相应处理（在这一步真正访问db，上述都是确保一致性）

### 快照方面
Raft层通过 ReadPersist 和持久存储进行交互，状态复制机通过 ReadSnapshot 和持久储存进行交互，状态复制机要向持久储存存储数据，需要通过 CondInstallSnap 向raft发送请求。

#### 实现路径
1. raft发送cmd请求到applyCh，被kv.ApplyHandler接收

#### 存储内容
- Raft:
    - rf.currentTerm 3C
    - rf.votedFor 
    - rf.log

    - rf.lastIncludedIndex 3D
    - rf.lastIncludedTerm
- KV Server
    - kv.db
    - kv.historyMap

## 一些优化
### 修复过多的InstallSnapshot RPC
- 在我原来的设计中, InstallSnapshot RPC的发送有2中情形:
    - handleAppendEntries在处理AppendEntries RPC回复时发现follower需要的日志项背快照截断, 立即调用go rf.handleInstallSnapshot(serverTo)协程发送快照
    - 心跳函数发送时发现PrevLogIndex < rf.lastIncludedIndex, 则发送快照

- 这和之前的情形类似, 在高并发的场景下，follower和Leader之间的日志复制也很频繁, 如果某一个日志触发了InstallSnapshot RPC的发送, 接下来连续很多个日志也会触发InstallSnapshot RPC的发送,并且还可能有很多历史RPC，因为InstallSnapshot RPC的发送时间消耗更大, 这样以来, 又加大了raft的压力, 所以, 我对InstallSnapshot RPC的发送做出修改:

    - handleAppendEntries在处理AppendEntries RPC回复时发现follower需要的日志项背快照截断, 仅仅设置rf.nextIndex[serverTo] = rf.lastIncludedIndex, 这将导致下一次心跳时调用go rf.handleInstallSnapshot(serverTo)协程发送快照
    - 心跳函数发送时发现PrevLogIndex < rf.lastIncludedIndex, 则发送快照

### Server应尽量少调用Raft接口
很多很杂的处理