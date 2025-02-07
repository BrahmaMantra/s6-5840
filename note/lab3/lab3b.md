
2025.2.01 - 2025.2.04
日志复制的逻辑如下:

### leader视角
1. client想集群的一个节点发送的命令, 如果不是leader, follower会通过心跳得知leader并返回给client
2. leader收到了命令, 将其构造为一个日志项, 添加当前节点的currentTerm为日志项的Term, 并将其追加到自己的log中
3. leader发送AppendEntries RPC将log复制到所有的节点, AppendEntries RPC需要增加**PrevLogIndex**、**PrevLogTerm**以供follower校验, 其中PrevLogIndex、PrevLogTerm由**nextIndex**确定
4. 如果RPC返回了成功, 则更新**matchIndex**和**nextIndex**, 同时寻找一个满足过半的matchIndex[i] >= N的索引位置**N**, 将其更新为自己**commitIndex**, 并提交直到commitIndex部分的日志项
5. 如果RPC返回了失败, 且伴随的的Term更大, 表示自己已经不是leader了, 将自身的角色转换为**Follower**, 并更新currentTerm和votedFor, 重启计器
6. 如果RPC返回了失败, 且伴随的的Term和自己的currentTerm相同, 将nextIndex自减再重试(后续以Term为单位)
### follower视角
1. follower收到AppendEntries RPC后,如果自身日志比Leader日志还新，直接告知更新的Term, 并返回false
2. follower收到AppendEntries RPC后, 通过**PrevLogIndex**、**PrevLogTerm**可以判断出"leader认为自己log的结尾位置"是否存在并且Term匹配, 如不匹配, 返回false并不执行操作;
3. 如果上述位置的信息匹配, 则需要判断插入位置是否有旧的日志项, 如果有, 则向后将log中冲突的内容清除
4. 将RPC中的日志项追加到log中，根据RPC的传入参数**更新commitIndex**, 并提交直到commitIndex部分的日志项



## 一些想法
所有server都有一个term =-1，index=0的类似虚拟头结点的东西，防止访问越界的

### nextIndex和matchIndex是易失性的

### if args.Term > rf.currentTerm && args.LastLogTerm >= rf.log[len(rf.log)-1].Term
比较的时候不要只args.Term，要看日志的term，term只有写到日志后才有意义
- 只看Term:网络分区的孤立节点会一直elect，自增term，会很大
- 只看日志的Term:一旦X节点重复选举，因为voteFor!=-1，所以我不知道是新的选举

### Reply false if term < currentTerm (§5.1)
我觉得如果单纯这么判断有问题，因为有可能一个节点因为网络分区导致孤立，所以Term一直在自增，后面回归后currentTerm太大导致无法同意别人投票，日志太久又不能自己选举

总结来说，leader不能直接提交之前term的条目，而是通过提交当前term的条目来间接提交之前的。因此，严格来说，leader提交的是当前term的条目，而之前的条目因此被提交。所以，用户的问题的答案是：是的，leader只能显式提交当前term的日志条目，但通过这种方式，之前的条目也会被隐式提交。

### 发生冲突的逻辑判断
	if len(args.Entries) != 0 && len(rf.log) > args.PrevLogIndex+1 /*&& rf.log[args.PrevLogIndex+1].Term != args.Entries[0].Term*/ {
		// 发生了冲突, 移除冲突位置开始后面所有的内容
		DPrintf("server %v 的log与args发生冲突, 进行移除\n", rf.me)
		rf.log = rf.log[:args.PrevLogIndex+1]
	}
发都发来了，我直接看整个term为A的第一个的前一个（也就是PrevLogIndex+PrevLogTerm），如果一样我就认为合法，直接整个Copy了，不找到具体的Index


### RequestVote相关
- 不能只看日志新旧：如果只看日志新旧，会出现一个被网络分区的前Leader（有较新的日志），在一个新Leader出现但还没有开始Append的时候，恢复网络并向follower传递fake news（此时就有两个Leader）


## 最后
Testing completed: 50 iterations run.
Successes: 50
Failures: 0
fz@Brahmamantra:~/go/src/6.5840/src/raft$ go test -run 3B
Test (3B): basic agreement ...
  ... Passed --   0.7  3   14    3624    3
Test (3B): RPC byte count ...
  ... Passed --   2.0  3   46  112504   11
Test (3B): test progressive failure of followers ...
  ... Passed --   4.7  3  156   32543    3
Test (3B): test failure of leaders ...
  ... Passed --   5.0  3  242   53399    3
Test (3B): agreement after follower reconnects ...
  ... Passed --   3.9  3  106   26016    7
Test (3B): no agreement if too many followers disconnect ...
  ... Passed --   3.5  5  252   51356    4
Test (3B): concurrent Start()s ...
  ... Passed --   0.8  3   18    4692    6
Test (3B): rejoin of partitioned leader ...
  ... Passed --   4.4  3  180   42006    4
Test (3B): leader backs up quickly over incorrect follower logs ...
  ... Passed --  21.8  5 2320 1758704  102
Test (3B): RPC counts aren't too high ...
  ... Passed --   2.2  3   56   15072   12
PASS
ok      6.5840/raft     48.942s

## 尚未解决
在开启 -race的时候，call 和append(log)的时候会有极小概率的竞争，但是似乎没有影响到结果
(后续发现可能是Call传参要传入log，用来encode之类的，但是)