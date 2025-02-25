## 快照的生成和传递逻辑
- 简单说, lab3B就是要在底层raft的log过大时生成快照并截断日志, 从而节省内存空间, 并且快照会持久化存储到本地。因此， 原来的代码结构只需要在以下几个方面做出调整：
    - 需要再某个地方定期地判断底层raft的日志大小, 决定是否要生成快照, 生成快照直接调用我们在lab3中实现的接口Snapshot即可
    由于follower的底层raft会出现无法从Leader获取log的情况, 这时Leader会发送给follower的raft层一个快照, raft层会将其上交给server, server通过快照改变自己的状态机
    - server启动时需要判断是否有持久化的快照需要加载, 如果有就加载

## 快照应该包含什么?

快照首先应该包含的肯定是内存中的KV数据库, 也就是自己维护的**map**, 但是还应该包含对**每个clerk序列号的记录信息**, 因为从快照恢复后的server应该具备判断重复的客户端请求的能力, 同时也应该记录**最近一次应用到状态机的日志索引**, 凡是低于这个索引的日志都是包含在快照中

因此, server结构体需要添加如下成员:

type KVServer struct {
    ...
	persister    *raft.Persister
	lastApplied  int
}

## 加载和生成快照

通过上述分析, 快照的加载和生成就很简单了,代码如下:

- GenSnapShot和LoadSnapShot分别生成和加载快照, 唯一需要注意的就是这两个函数应当在持有锁时才能调用
## 生成快照的时机判断

- 由于ApplyHandler协程会不断地读取raft commit的通道, 所以每收到一个log后进行判断即可

- 这里还需要进行之前提到的判断: 低于lastApplied索引的日志都是包含在快照中, 在尽显lab4A的操作之后, 再判断是否需要生成快照, 在我的实现中, 如果仅仅比较maxraftstate和persister.RaftStateSize()相等才生成快照的话, 无法通过测例, 因为可能快照RPC存在一定延时, 所以我采用的手段是只要达到阈值的95%, 就生成快照
## 加载快照的时机判断
首先启动时需要判断是否需要加载快照, 然后就是ApplyHandler从通道收到快照时需要判断加载, 都很简单


## 遇到的问题
### TestSnapshotUnreliable3B超时
在之前的设计中，每个请求到来我都要通知AppendEntries，导致发送的RPC过多