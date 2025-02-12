# log compaction (hard)
2025.2.5 - 2025.2.7
## 快照请求与传输
某一时刻， service为了减小内存压力，将状态机状态封装成一个SnapShot并将请求发送给Leader一侧的raft(Follower侧的sevice也会会存在快照操作), raft层保存SnapShot并截断自己的log数组, 当通过心跳发现Follower的节点的log落后SnapShot时, 通过InstallSnapshot发送给Follower, Follower保存SnapShot并将快照发送给service

### InstallSnapshot RPC发送时机
阅读论文可知, 当Leader发现Follower要求回退的日志已经被SnapShot截断时, 需要发生InstallSnapshot RPC, 在我设计的代码中, 以下2个场景会出现:

#### appendEntries发送函数发起
#### appendEntries回复处理函数发起


### 问题
#### 快照时
要保留快照的最后一个index，方便找PreviousIndex和PreviousTerm
#### 数组越界
折磨问题，因为快照导致log的索引必然出现变化，导致一系列问题。。。

#### apply问题
1. 当同时要applyCmd和applySnapshot的时候，会阻塞，因为上层config在lastapplied%10 ==0的时候会进入SnapShot，两个环节都需要锁
2. 当你要apply一个快照，其中**cfg.lastApplied[i] = lastIncludedIndex**代表会直接更新lastApplied,所以要进行相应处理！

## 最后
~~~ sh
fz@Brahmamantra:~/go/src/6.5840/src/raft$ go test -run 3D
Test (3D): snapshots basic ...
  ... Passed --   2.2  3  154   46338  220
Test (3D): install snapshots (disconnect) ...
  ... Passed --  34.2  3 1627  592388  296
Test (3D): install snapshots (disconnect+unreliable) ...
  ... Passed --  42.1  3 2014  802494  324
Test (3D): install snapshots (crash) ...
  ... Passed --  24.5  3 1219  596226  310
Test (3D): install snapshots (unreliable+crash) ...
  ... Passed --  31.8  3 1449  827585  347
Test (3D): crash and restart all servers ...
  ... Passed --   7.7  3  348   78061   63
Test (3D): snapshot initialization after crash ...
  ... Passed --   2.3  3   96   18864   14
PASS
ok      6.5840/raft     144.879s
~~~