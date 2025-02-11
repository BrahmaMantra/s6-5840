# Part 3A: leader election (moderate)
- 先想一下，需要做的东西有哪些。根据hints来看的话，暂时有初始化操作、定时选举、投票、leader发送心跳（AppendEntries）
- 心跳就是一种特殊的AppendEntries, 其特殊在Entries长度为0,

2025.1.27 - 2025.2.01
## 测试
- go test -run 3A

## 遇到的问题
### 为什么选举时必须递增currentTerm？
- 原来在Raft共识算法中，当节点发起选举成为Candidate时，无论选举是否成功，其currentTerm都会递增

    - 避免重复任期冲突：每个任期（Term）唯一标识一次选举周期，确保系统能区分新旧Leader的合法性。如果选举失败后不递增Term，可能导致多个节点在同一Term内反复发起冲突的选举。

    - 保证安全性：Raft规定，只有包含最新日志的节点才能成为Leader。递增Term可强制节点在发起新选举前更新自己的状态，确保选举的合法性。

### warning: term changed even though there were no failures
- 是因为election time设置小了，导致send心跳之前就触发，设大100ms后就没问题了

## 结论
~~~ sh
fz@Brahmamantra:~/go/src/6.5840/src/raft$ go test -run 3A
Test (3A): initial election ...
  ... Passed --   3.0  3   72   18440    0
Test (3A): election after network failure ...
  ... Passed --   4.5  3  122   23726    0
Test (3A): multiple elections ...
  ... Passed --   5.5  7  588  107288    0
PASS
ok      6.5840/raft     12.989s
~~~