package raft

import "math"

// return false if:
// 1. 收到过期的prc回复
// 2. 回复了更新的term, 表示自己已经不是leader了(变成follower)
// 3. prevLogIndex-prevLogTerm匹配的项不对,问题不大
func (rf *Raft) sendAppendEntries(server int, args *AppendEntriesArgs, reply *AppendEntriesReply) bool {
	sendArgs := *args
	// 一直请求rpc，直到成功
	// fmt.Printf("Server %d send heartbeat to %d\n", rf.me, server)
	if ok := rf.peers[server].Call("Raft.AppendEntries", &sendArgs, reply); ok {
		for !ok {
			ok = rf.peers[server].Call("Raft.AppendEntries", args, reply)
		}
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
			// 找到log中term自减的第一个index
			for i := rf.nextIndex[server] - 1; i >= 0; i-- {
				if rf.log[i].Term < rf.log[rf.nextIndex[server]-1].Term {
					rf.nextIndex[server] = i + 1
					// DPrintf("server %v 的nextIndex: %v\n", server, rf.nextIndex[server])
					break
				}
				if i == 0 {
					rf.nextIndex[server] = i
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
		//DPrintf("[]nextindex = %v", rf.nextIndex)
		//DPrintf("server %v 的nextIndex: %v\n", rf.me, rf.nextIndex[server])
		// 需要判断是否可以commit
		N := len(rf.log) - 1

		for N > rf.commitIndex {
			count := 1 // 1表示包括了leader自己
			for i := 0; i < len(rf.peers); i++ {
				if i == rf.me {
					continue
				}
				if rf.matchIndex[i] >= N && rf.log[N].Term == rf.currentTerm {
					count += 1
				}
			}
			if count > len(rf.peers)/2 {
				// 如果至少一半的follower回复了成功, 更新commitIndex
				rf.commitIndex = N
				rf.commitCh <- true
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
	//DPrintf("server %v 收到server %v 的心跳\n", rf.me, args.LeaderId)
	// 1. 收到rpc的term小于当前term，代表这个leader不合法
	if args.Term < rf.currentTerm {
		// follower觉得leader不合法
		reply.Term = rf.currentTerm
		reply.Success = false
		//DPrintf("server %v 检查到server %v 的term不合法\n", rf.me, args.LeaderId)
		return
	}
	if args.Term > rf.currentTerm {
		// 新leader的第一个消息
		if rf.state == Leader {
			DPrintf("RequestVote():Server %d 退位\n", rf.me)
		}
		// rf.currentTerm = args.Term // 更新iterm
		rf.votedFor = -1 // 易错点: 更新投票记录为未投票
		rf.state = Follower
		// rf.persist()
	}
	rf.currentTerm = args.Term
	rf.persist()
	if len(args.Entries) == 0 {
		// 心跳函数
		// fmt.Printf("Server %d receive heartbeat from %d\n", rf.me, args.LeaderId)
		rf.state = Follower
		rf.votedFor = args.LeaderId
		reply.Term = rf.currentTerm
		reply.Success = true
		rf.persist()
		// return
	}
	if args.PrevLogIndex >= len(rf.log) || rf.log[args.PrevLogIndex].Term != args.PrevLogTerm {
		// 校验PrevLogIndex和PrevLogTerm不合法
		// 2. Reply false if log doesn’t contain an entry at prevLogIndex whose term matches prevLogTerm (§5.3)
		reply.Term = rf.currentTerm
		reply.Success = false
		//DPrintf("server %v 检查到心跳中参数不合法:\n\t args.PrevLogIndex=%v, args.PrevLogTerm=%v, \n\tlen(self.log)=%v, self最后一个位置term为:%v\n", rf.me, args.PrevLogIndex, args.PrevLogTerm, len(rf.log), rf.log[len(rf.log)-1].Term)
		//DPrintf("server %v 的log为: %+v\n", rf.me, rf.log)
		//DPrintf("Leader server %v 的log为: %+v\n", args.LeaderId, rf.log)
		return
	}
	// 3. If an existing entry conflicts with a new one (same index
	// but different terms), delete the existing entry and all that
	// follow it (§5.3) 01   0111234
	if len(rf.log) > args.PrevLogIndex+1 /*&& rf.log[args.PrevLogIndex+1].Term != args.Entries[0].Term*/ {
		// 发生了冲突, 移除冲突位置开始后面所有的内容
		DPrintf("server %v 的log与args发生冲突, 进行移除\n", rf.me)
		rf.log = rf.log[:args.PrevLogIndex+1]
		rf.persist()
	}
	// 4. Append any new entries not already in the log
	// 补充apeend的业务
	if len(args.Entries) != 0 {
		DPrintf("server的term为%v, leader的term为%v\n", rf.currentTerm, args.Term)
		DPrintf("server %v 将要append的日志为: %+v\n", rf.me, args.Entries)
		rf.log = append(rf.log, args.Entries...)
		DPrintf("server %v 成功进行apeend from server %d, log: %+v\n", rf.me, args.LeaderId, rf.log)
		rf.persist()
	}
	reply.Success = true
	reply.Term = rf.currentTerm
	if args.LeaderCommit > rf.commitIndex {
		// 5.If leaderCommit > commitIndex, set commitIndex = min(leaderCommit, index of last new entry)
		rf.commitIndex = int(math.Min(float64(args.LeaderCommit), float64(len(rf.log)-1)))
		rf.commitCh <- true
	}
	// //DPrintf("server %v 的commitIndex is %v", rf.me, rf.commitIndex)
}
