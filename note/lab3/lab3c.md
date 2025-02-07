2025.2.04 - 2025.2.05
3.3 持久化时是否需要锁?

按照我的理解, 持久化时不需要锁保护log, 原因如下:

    Leader视角
    Leader永远不会删除自己的log(此时没有快照), 因此不需要锁保护
    Follower视角
    尽管Follower可能截断log, 但永远不会截断在commit的log之前, 而持久化只需要保证已经commit的log, 因此也不需要锁


### index和term都一样但是网络分区导致不是一个command
1是一个被网络分区的自以为leader，在term=4的时候上任了(此时大家都是 111 )，但是上任后就没有和别人进行通信了，然后就是4以term=3上任了（因为随机数选举时间），导致下面的问题

0: 111
1: 111444444 444444444
2: 111333333 444444444444444
3: 111333333 444444444444444
4: 111333333 44444444444444455

解决办法：原本我只在“日志比我新的leader”判定中写了rf.currentTerm = args.Term，但是会有情况是日志一样但是我还没投票，此时我也要更新currentTerm

## 结果
fz@Brahmamantra:~/go/src/6.5840/src/raft$ go test -run 3C
Test (3C): basic persistence ...
  ... Passed --   3.5  3   92   20574    6
Test (3C): more persistence ...
  ... Passed --  23.1  5 1564  314754   17
Test (3C): partitioned leader and one follower crash, leader restarts ...
  ... Passed --   1.9  3   42    9258    4
Test (3C): Figure 8 ...
  ... Passed --  31.9  5  736  132481   14
Test (3C): unreliable agreement ...
  ... Passed --   6.1  5  312   89727  246
Test (3C): Figure 8 (unreliable) ...
  ... Passed --  36.3  5 3864 17807811  909
Test (3C): churn ...
  ... Passed --  16.5  5 1316 2393064  327
Test (3C): unreliable churn ...
  ... Passed --  16.2  5  772  318077  424
PASS
ok      6.5840/raft     135.502s