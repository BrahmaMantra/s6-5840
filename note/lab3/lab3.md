# 总结
lab3是一个十分折磨的实验啊!!!

- 数据只由leader流向follower
- 不管true还是false，不是一个term都不处理(避免过时的RPC请求)

- 通过for来尝试重复发送RPC的时候，一定要检查reply.term == 0 是否成立，因为超时的RPC也会返回true但是reply是默认值，我们通过判断它的term来判断它是否超时了
~~~ sh
fz@Brahmamantra:~/go/src/6.5840/src/raft$ go test -run 3
Test (3A): initial election ...
  ... Passed --   3.1  3  114   26356    0
Test (3A): election after network failure ...
  ... Passed --   4.6  3  242   41466    0
Test (3A): multiple elections ...
  ... Passed --   5.5  7 1386  215214    0
Test (3B): basic agreement ...
  ... Passed --   0.6  3   20    3840    3
Test (3B): RPC byte count ...
  ... Passed --   1.1  3   56  112544   11
Test (3B): test progressive failure of followers ...
  ... Passed --   4.5  3  232   41721    3
Test (3B): test failure of leaders ...
  ... Passed --   4.8  3  362   71026    3
Test (3B): agreement after follower reconnects ...
  ... Passed --   3.3  3  144   31904    7
Test (3B): no agreement if too many followers disconnect ...
  ... Passed --   3.5  5  400   63756    4
Test (3B): concurrent Start()s ...
  ... Passed --   0.6  3   20    3884    6
Test (3B): rejoin of partitioned leader ...
  ... Passed --   3.8  3  252   52112    4
Test (3B): leader backs up quickly over incorrect follower logs ...
  ... Passed --  13.0  5 2296 1525712  102
Test (3B): RPC counts aren't too high ...
  ... Passed --   2.0  3   78   18582   12
Test (3C): basic persistence ...
  ... Passed --   3.4  3  140   27708    6
Test (3C): more persistence ...
  ... Passed --  18.8  5 2273  375283   17
Test (3C): partitioned leader and one follower crash, leader restarts ...
  ... Passed --   2.2  3   68   12122    4
Test (3C): Figure 8 ...
  ... Passed --  38.1  5 2428  311430   31
Test (3C): unreliable agreement ...
  ... Passed --   1.8  5  372  111019  246
Test (3C): Figure 8 (unreliable) ...
  ... Passed --  35.1  5 10691 24406400  609
Test (3C): churn ...
  ... Passed --  16.1  5 4680 21403778 1560
Test (3C): unreliable churn ...
  ... Passed --  16.1  5 3828 6061125  972
Test (3D): snapshots basic ...
  ... Passed --   2.3  3  150   44722  198
Test (3D): install snapshots (disconnect) ...
  ... Passed --  33.2  3 1610  649364  288
Test (3D): install snapshots (disconnect+unreliable) ...
  ... Passed --  41.4  3 1951  695282  314
Test (3D): install snapshots (crash) ...
  ... Passed --  25.5  3 1214  583724  311
Test (3D): install snapshots (unreliable+crash) ...
  ... Passed --  33.3  3 1566  843567  321
Test (3D): crash and restart all servers ...
  ... Passed --   9.7  3  423   92957   60
Test (3D): snapshot initialization after crash ...
  ... Passed --   2.7  3  110   20910   14
PASS
ok      6.5840/raft     330.121s
~~~
## 补的坑
### 不要以term为单位去进行覆盖！！
~~~ go
//不能这样做，因为可能有旧的RPC过来，把我之前弄好的log给破坏了
		if rf.VirtualLogIdx(len(rf.log)) > args.PrevLogIndex+1 /*&& rf.log[args.PrevLogIndex+1].Term != args.Entries[0].Term*/ {
			// 发生了冲突, 移除冲突位置开始后面所有的内容
			DPrintf("server %v 的log与args发生冲突, 进行移除\n", rf.me)
			rf.log = rf.log[:rf.RealLogIdx(args.PrevLogIndex+1)] //后面append
			rf.persist()
		}
~~~
~~~ go
//更好的做法，找到冲突的位置
		for idx, log := range args.Entries {
			ridx := rf.RealLogIdx(args.PrevLogIndex) + 1 + idx
			if ridx < len(rf.log) && rf.log[ridx].Term != log.Term {
				// 某位置发生了冲突, 覆盖这个位置开始的所有内容
				rf.log = rf.log[:ridx]
				rf.log = append(rf.log, args.Entries[idx:]...)
				break
			} else if ridx == len(rf.log) {
				// 没有发生冲突但长度更长了, 直接拼接
				rf.log = append(rf.log, args.Entries[idx:]...)
				break
			}
		}
~~~