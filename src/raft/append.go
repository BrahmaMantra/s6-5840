package raft

import "math"

// return false if:
// 1. 收到过期的prc回复
// 2. 回复了更新的term, 表示自己已经不是leader了(变成follower)
// 3. prevLogIndex-prevLogTerm匹配的项不对,问题不大
func (rf *Raft) sendAppendEntries(server int, args *AppendEntriesArgs, reply *AppendEntriesReply) bool {
	rf.mu.Lock()
	if rf.state != Leader {
		rf.mu.Unlock()
		return false
	}
	rf.mu.Unlock()
	// 一直请求rpc，直到成功
	// fmt.Printf("Server %d send heartbeat to %d\n", rf.me, server)
	if ok := rf.peers[server].Call("Raft.AppendEntries", args, reply); ok {
		for !ok {
			ok = rf.peers[server].Call("Raft.AppendEntries", args, reply)
		}
	}
	if reply.Term == 0 && !reply.Success {
		// DPrintf("server %v sendAppendEntries 发送RPC失败", rf.me)
		return false
	}
	rf.mu.Lock()
	defer rf.mu.Unlock()
	// 收到过期的prc回复
	if args.Term < rf.currentTerm {
		return false
	}
	// 没有append成功，可能是很多原因，具体在AppendEntries函数有介绍
	if !reply.Success {
		// 回复了更新的term, 表示自己已经不是leader了
		if reply.Term > rf.currentTerm {
			rf.state = Follower
			rf.currentTerm = reply.Term
			rf.votedFor = -1
			rf.voteCount = 0
			rf.timer.reset()
			rf.persist()
			DPrintf("sendAppendEntries():Server %d 退位\n", rf.me)
			return false
		}

		// term仍然相同, 且自己还是leader, 表示prevLogIndex-prevLogTerm匹配的项不对
		// 将nextIndex自减再重试，（后续改成term自减的那个index重试）
		if reply.Term == rf.currentTerm && rf.state == Leader {
			// 快速回退的处理
			// 找到log中term自减的第一个index
			for i := rf.nextIndex[server] - 1; i >= 0; i-- {
				// [1122]2233 3 [11222]233344
				if i < rf.lastIncludedIndex {
					// 代表这个位置在snapshot了
					// 也就是想穿进去的PreviousIndex在snapshot中,也是不行的
					DPrintf("i = %v, nextIndex[%v] = %v, lastIncludedIndex = %v\n", i, server, rf.nextIndex[server], rf.lastIncludedIndex)
					// DPrintf("sendAppendEntries():handleInstallSnapshot\n")
					// go rf.handleInstallSnapshot(server)
					rf.nextIndex[server] = rf.lastIncludedIndex
					break
				}
				// PrevLogIndex - PrevLogTerm到了下一个term
				if rf.log[rf.RealLogIdx(i)].Term < rf.log[rf.RealLogIdx(rf.nextIndex[server]-1)].Term {
					rf.nextIndex[server] = i + 1
					DPrintf("server %v 发现server %v 的log与自己的log发生冲突, 重试\n", rf.me, server)
					DPrintf(" i = %d,server %v 的nextIndex: %v\n", i, server, rf.nextIndex[server])
					break
				}
				if i == rf.lastIncludedIndex {
					// 刚好在snapshot末的位置，并且不符合，发送快照
					rf.nextIndex[server] = i + 1
					DPrintf("i = %v, nextIndex[%v] = %v, lastIncludedIndex = %v\n", i, server, rf.nextIndex[server], rf.lastIncludedIndex)
					DPrintf("rf.log[rf.RealLogIdx(rf.nextIndex[server]-1)].Term =%v,args.PrevLogTerm = %v\n", rf.log[rf.RealLogIdx(rf.nextIndex[server]-1)].Term, args.PrevLogTerm)
					// DPrintf("sendAppendEntries():handleInstallSnapshot\n")
					// go rf.handleInstallSnapshot(server)
					rf.nextIndex[server] = rf.lastIncludedIndex
					// DPrintf("server %v 的log为: %+v\n", rf.me, rf.log)
					break
				}
			}
			// rf.nextIndex[server] -= 1
			//DPrintf("server %v 发现server %v 的log与自己的log发生冲突, 重试\n", rf.me, server)
			return false
		}
	} else {
		// server回复成功
		rf.matchIndex[server] = args.PrevLogIndex + len(args.Entries)
		rf.nextIndex[server] = rf.matchIndex[server] + 1
		// DPrintf("server %v 回复成功", server)
		// 需要判断是否可以commit
		N := rf.VirtualLogIdx(len(rf.log) - 1)

		for N > rf.commitIndex {
			count := 1 // 1表示包括了leader自己
			for i := 0; i < len(rf.peers); i++ {
				if i == rf.me {
					continue
				}
				if rf.matchIndex[i] >= N && rf.log[rf.RealLogIdx(N)].Term == rf.currentTerm {
					// 需要确保调用SnapShot时检查索引是否超过commitIndex
					// 谨防数组越界
					count += 1
				}
			}
			if count > len(rf.peers)/2 {
				// 如果至少一半的follower回复了成功, 更新commitIndex
				rf.commitIndex = N
				DPrintf("server %v 的rf.commitIndex = %v\n", rf.me, rf.commitIndex)
				select {
				case rf.commitCh <- true:
				default:
				}
				break
			}
			N -= 1
		}
		return true
	}
	return false
}

// follwer接收leader的心跳,return false if :
// 1. args.term < currentTerm
// 2. prevLogIndex和prevLogTerm对应不上

// prevLogIndex对应的log entry和args.Entries[0]对应的term不一样,有（遗留）冲突，我们先true

func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	rf.timer.reset()
	// DPrintf("server %v 收到server %v 的心跳\n", rf.me, args.LeaderId)
	reply.Term = rf.currentTerm
	// 1. 收到rpc的term小于当前term，代表这个leader不合法
	if args.Term < rf.currentTerm {
		// follower觉得leader不合法
		reply.Success = false
		DPrintf("server %v 检查到server %v 的term不合法\n", rf.me, args.LeaderId)
		return
	}
	if args.Term > rf.currentTerm {
		// 新leader的第一个消息
		if rf.state == Leader {
			DPrintf("RequestVote():Server %d 退位\n", rf.me)
		}
		rf.currentTerm = args.Term // 更新iterm
		rf.votedFor = -1           // 易错点: 更新投票记录为未投票
		rf.state = Follower
		rf.persist()
	}
	rf.persist()

	// isConflict := false
	// 在快照里面
	if args.PrevLogIndex < rf.lastIncludedIndex {
		reply.Success = false
		DPrintf("AppendEntries():在快照里面server %v 的lastIncludedIndex = %v, args.PrevLogIndex = %v\n", rf.me, rf.lastIncludedIndex, args.PrevLogIndex)
		return
	}
	if args.PrevLogIndex >= rf.VirtualLogIdx(len(rf.log)) || rf.log[rf.RealLogIdx(args.PrevLogIndex)].Term != args.PrevLogTerm {
		// 校验PrevLogIndex和PrevLogTerm不合法
		// 2. Reply false if log doesn’t contain an entry at prevLogIndex whose term matches prevLogTerm (§5.3)
		reply.Success = false
		DPrintf("server %v 校验server %d PrevLogIndex和PrevLogTerm不合法", rf.me, args.LeaderId)
		// DPrintf("server %v 的log为: %+v\n", rf.me, rf.log)
		if args.PrevLogIndex < rf.VirtualLogIdx(len(rf.log)) {
			DPrintf("args.PrevLogIndex = %d,rf.VirtualLogIdx(len(rf.log)) = %d, rf.log[rf.RealLogIdx(args.PrevLogIndex)].Term = %d, args.PrevLogTerm = %d\n", args.PrevLogIndex, rf.VirtualLogIdx(len(rf.log)), rf.log[rf.RealLogIdx(args.PrevLogIndex)].Term, args.PrevLogTerm)
		}
		return
	}
	/// 到这一步就是准备要append了
	if len(args.Entries) != 0 {
		// 3. If an existing entry conflicts with a new one (same index
		// but different terms), delete the existing entry and all that
		// follow it (§5.3) 01   0111234

		// if rf.VirtualLogIdx(len(rf.log)) > args.PrevLogIndex+1 /*&& rf.log[args.PrevLogIndex+1].Term != args.Entries[0].Term*/ {
		// 	// 发生了冲突, 移除冲突位置开始后面所有的内容	不能这样做，因为可能有旧的RPC过来，把我之前弄好的log给破坏了
		// 	DPrintf("server %v 的log与args发生冲突, 进行移除\n", rf.me)
		// 	rf.log = rf.log[:rf.RealLogIdx(args.PrevLogIndex+1)]
		// 	rf.persist()
		// }

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
		// 4. Append any new entries not already in the log
		// 补充apeend的业务
		if len(args.Entries) != 0 {
			DPrintf("server %v 成功进行apeend, lastApplied=%v, len(log)=%v\n", rf.me, rf.lastApplied, len(rf.log))
		}
		rf.persist()
	}
	reply.Success = true
	if args.LeaderCommit > rf.commitIndex {
		// 5.If leaderCommit > commitIndex, set commitIndex = min(leaderCommit, index of last new entry)
		DPrintf("args.LeaderCommit = %v,rf.commitIndex = %v", args.LeaderCommit, rf.commitIndex)
		rf.commitIndex = int(math.Min(float64(args.LeaderCommit), float64(rf.VirtualLogIdx(len(rf.log)-1))))
		select {
		case rf.commitCh <- true:
		default:
		}
	}
	// //DPrintf("server %v 的commitIndex is %v", rf.me, rf.commitIndex)
}

// 只管更新快照不管剩下的log是否完全
func (rf *Raft) handleInstallSnapshot(server int) {
	reply := &InstallSnapshotReply{}

	// DPrintf("handleInstallSnapshot():server %v 尝试获取锁mu", rf.me)
	rf.mu.Lock()

	if rf.state != Leader {
		// 自己已经不是Lader了, 返回
		DPrintf("handleInstallSnapshot():server %v 不再是leader", rf.me)
		rf.mu.Unlock()
		return
	}

	args := &InstallSnapshotArgs{
		Term:              rf.currentTerm,
		LeaderId:          rf.me,
		LastIncludedIndex: rf.lastIncludedIndex,
		LastIncludedTerm:  rf.lastIncludedTerm,
		Data:              rf.snapShot,
		LastIncludedCmd:   rf.log[0].Command,
	}

	rf.mu.Unlock()
	// DPrintf("server %v handleInstallSnapshot 释放锁mu", rf.me)

	// 发送RPC时不要持有锁
	if ok := rf.peers[server].Call("Raft.InstallSnapshot", args, reply); !ok {
		for !ok {
			ok = rf.peers[server].Call("Raft.InstallSnapshot", args, reply)
		}
	}
	if reply.Term == 0 {
		DPrintf("server %v handleInstallSnapshot 发送RPC失败", rf.me)
		return
	}
	DPrintf("server %v handleInstallSnapshot 发送RPC成功", rf.me)
	rf.mu.Lock()
	defer rf.mu.Unlock()
	if reply.ShouldDie {
		DPrintf("ShouldDie: server %v currentTerm = %v, reply.Term = %v\n", rf.me, rf.currentTerm, reply.Term)
	}
	if reply.Term > rf.currentTerm {
		// 自己是旧Leader
		rf.currentTerm = reply.Term
		rf.state = Follower
		rf.votedFor = -1
		rf.timer.reset()
		rf.persist()
		DPrintf("handleInstallSnapshot():Server %d 退位\n", rf.me)
		return
	}

	rf.nextIndex[server] = rf.VirtualLogIdx(1)
	DPrintf("server %v handleInstallSnapshot: server %v 的lastIncludedIndex: %v\n", rf.me, server, rf.lastIncludedIndex)
}

// InstallSnapshot handler
func (rf *Raft) InstallSnapshot(args *InstallSnapshotArgs, reply *InstallSnapshotReply) {
	// DPrintf("server %v InstallSnapshot: args.LastIncludedIndex= %v\n", rf.me, args.LastIncludedIndex)
	rf.mu.Lock()
	defer func() {
		rf.mu.Unlock()
		// DPrintf("InstallSnapshot(): server %d unlock\n", rf.me)
	}()

	// 1. Reply immediately if term < currentTerm
	if args.Term < rf.currentTerm {
		reply.Term = rf.currentTerm
		reply.ShouldDie = true
		DPrintf("server %v 拒绝来自 %v 的 InstallSnapshot, 更小的Term\n", rf.me, args.LeaderId)
		// DPrintf("server %v currentTerm = %v, args.Term = %v\n", rf.me, rf.currentTerm, args.Term)
		return
	}
	rf.timer.reset()
	// 不需要实现分块的RPC
	if args.Term > rf.currentTerm {
		rf.currentTerm = args.Term
		rf.votedFor = -1
		DPrintf("server %v 接受来自 %v 的 InstallSnapshot, 且发现了更大的Term\n", rf.me, args.LeaderId)
	}

	rf.state = Follower

	hasEntry := false
	rIdx := 0
	// 6. 如果现有的log entry 有一样的 index and term as snapshot’s last included entry, 保留 log entries following it 并且回复
	//  [111]222333  334455 but snapshot is [111222333]
	for ; rIdx < len(rf.log); rIdx++ {
		if rf.VirtualLogIdx(rIdx) == args.LastIncludedIndex && rf.log[rIdx].Term == args.LastIncludedTerm {
			hasEntry = true
			DPrintf("server %v rIdx = %v,args.LastIncludedIndex = %v\n", rf.me, rIdx, args.LastIncludedIndex)
			break
		}
	}

	msg := &ApplyMsg{
		SnapshotValid: true,
		Snapshot:      args.Data,
		SnapshotTerm:  args.LastIncludedTerm,
		SnapshotIndex: args.LastIncludedIndex,
	}
	if hasEntry {
		DPrintf("server %v InstallSnapshot: args.LastIncludedIndex= %v 位置存在, 保留后面的log\n", rf.me, args.LastIncludedIndex)

		rf.log = rf.log[rIdx:]
	} else {
		DPrintf("server %v InstallSnapshot: 清空log\n", rf.me)
		rf.log = make([]LogEntry, 0)
		rf.log = append(rf.log, LogEntry{Term: rf.lastIncludedTerm, Command: args.LastIncludedCmd}) // 索引为0处占位
	}

	// 8. Reset state machine using snapshot contents (and load snapshot’s cluster configuration)
	rf.snapShot = args.Data
	rf.lastIncludedIndex = args.LastIncludedIndex
	rf.lastIncludedTerm = args.LastIncludedTerm
	if rf.commitIndex < args.LastIncludedIndex {
		rf.commitIndex = args.LastIncludedIndex
	}
	// if rf.lastApplied < args.LastIncludedIndex {
	rf.lastApplied = args.LastIncludedIndex
	// }
	reply.Term = rf.currentTerm
	DPrintf("server %v InstallSnapshot: args.LastIncludedIndex= %v, lastIncludedIndex = %v,lastApplied = %d \n", rf.me, args.LastIncludedIndex, rf.lastIncludedIndex, rf.lastApplied)
	rf.applyCh <- *msg
	// DPrintf("InstallSnapshot(): server %d applyCh <- msg\n", rf.me)
	rf.persist()
}
