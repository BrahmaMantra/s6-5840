package raft

// example code to send a RequestVote RPC to a server.
// server is the index of the target server in rf.peers[].
// expects RPC arguments in args.
// fills in *reply with RPC reply, so caller should
// pass &reply.
// the types of the args and reply passed to Call() must be
// the same as the types of the arguments declared in the
// handler function (including whether they are pointers).
//
// The labrpc package simulates a lossy network, in which servers
// may be unreachable, and in which requests and replies may be lost.
// Call() sends a request and waits for a reply. If a reply arrives
// within a timeout interval, Call() returns true; otherwise
// Call() returns false. Thus Call() may not return for a while.
// A false return can be caused by a dead server, a live server that
// can't be reached, a lost request, or a lost reply.
//
// Call() is guaranteed to return (perhaps after a delay) *except* if the
// handler function on the server side does not return.  Thus there
// is no need to implement your own timeouts around Call().
//
// look at the comments in ../labrpc/labrpc.go for more details.
//
// if you're having trouble getting RPC to work, check that you've
// capitalized all field names in structs passed over RPC, and
// that the caller passes the address of the reply struct with &, not
// the struct itself.
func (rf *Raft) sendRequestVote(server int, args *RequestVoteArgs, reply *RequestVoteReply) bool {
	// 一直请求rpc，直到成功
	if ok := rf.peers[server].Call("Raft.RequestVote", args, reply); ok {
		for !ok {
			ok = rf.peers[server].Call("Raft.RequestVote", args, reply)
		}
	}
	//超时的RPC
	if reply.Term == 0 {
		DPrintf("server %v handleInstallSnapshot 发送RPC失败", rf.me)
		return false
	}
	rf.mu.Lock()
	defer rf.mu.Unlock()
	// 过时的投票请求
	if rf.currentTerm != args.Term {
		DPrintf("sendRequestVote():: server %d 过时的投票请求\n", rf.me)
		return false
	}
	// 如果同意投票，更新term
	if reply.VoteGranted {
		// 选举路上被别人干成follower了，就不用投票了
		if rf.state < Candidate {
			DPrintf("sendRequestVote():Server %d.state is %d\n", rf.me, rf.state)
			DPrintf("sendRequestVote():Server %d 不再是Candidate\n", rf.me)
			return false
		}
		if rf.voteCount > len(rf.peers)/2 {
			return true
		}
		rf.voteCount++
		DPrintf("server %d is supporting server %d , now voteCount = %d\n", server, rf.me, rf.voteCount)
		// fmt.Printf("Server %d vote for %d success\n", rf.me, server)
		// fmt.Printf("voteCount: %d\n", rf.voteCount)
		if rf.voteCount > len(rf.peers)/2 {
			rf.state = Leader
			DPrintf("Server %d become leader,term = %d\n", rf.me, rf.currentTerm)
			// 初始化nextIndex和matchIndex，因为它们是易失性的
			for i := 0; i < len(rf.nextIndex); i++ {
				rf.nextIndex[i] = rf.VirtualLogIdx(len(rf.log))
				rf.matchIndex[i] = 0
			}
			rf.resetHeartBeat()
		}
	} else {
		DPrintf("Server %d(term is %d) refuse vote because of  server%d(reply.term = %d)\n", rf.me, rf.currentTerm, server, reply.Term)
		// 有人拒绝投票
		rf.state = Follower
	}
	return true
}

// example RequestVote RPC handler.
// 接收投票请求
// 内部会锁
func (rf *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) {
	// Your code here (3A, 3B).
	rf.mu.Lock()
	defer rf.mu.Unlock()
	// fmt.Printf("Server %d(term is:%d ) receive vote request from %d(term is:%d)\n", rf.me, rf.currentTerm, args.CandidateId, args.Term)
	reply.Term = rf.currentTerm
	reply.VoteGranted = false
	// 如果请求的term小于当前term，拒绝投票
	if args.Term < rf.currentTerm /*|| args.LastLogTerm < rf.log[len(rf.log)-1].Term */ {
		// //DPrintf("Server %d refuse vote for %d\n", rf.me, args.CandidateId)
		return
	}
	// 在日志至少是一样新的情况下，(如果请求的term大于当前termXXXX )，同意投票（这个就不是同一个election了）
	if args.Term > rf.currentTerm {
		// if rf.state == Leader {
		// 	DPrintf("RequestVote():Server %d 退位\n", rf.me)
		// }
		rf.currentTerm = args.Term //更新当前term,防止孤立节点
		rf.state = Follower        // 必要，因为一个旧的leader要变成Follower
		rf.votedFor = -1
		DPrintf("Server %d(term is %d) find a new election in term %v from server %v\n", rf.me, rf.currentTerm, args.Term, args.CandidateId)
		rf.persist()
	}

	// 如果term>currentTerm||term == currentTerm&& 没投过票
	if rf.votedFor == -1 || rf.votedFor == args.CandidateId {
		// 只有虚拟log || 它的上一个index的term比我的最后一个大(日志比我新) || term一样大但是它长度>=我
		//DPrintf("rf.log = %v\nargs.LastLogTerm = %v,args.LastLogIndex = %v\n", rf.log, args.LastLogTerm, args.LastLogIndex)
		if args.LastLogTerm > rf.log[len(rf.log)-1].Term ||
			(args.LastLogTerm == rf.log[len(rf.log)-1].Term && args.LastLogIndex >= rf.VirtualLogIdx(len(rf.log))) {
			// 正式确定投票
			if rf.state == Leader {
				DPrintf("RequestVote():Server %d 退位\n", rf.me)
			}
			DPrintf("server %v (term is %d)vote for server%v(term is %d)\n", rf.me, rf.currentTerm, args.CandidateId, args.Term)
			rf.currentTerm = args.Term //更新当前term,防止孤立节点
			rf.state = Follower
			reply.VoteGranted = true
			reply.Term = rf.currentTerm
			rf.votedFor = args.CandidateId
			rf.persist()
			//DPrintf("server %v vote for server %v\n", rf.me, args.CandidateId)
			//DPrintf("[]nextindex = %v", rf.nextIndex)
		} else {
			if args.LastLogTerm < rf.log[len(rf.log)-1].Term {
				DPrintf("server %v 拒绝向 server %v 投票: 更旧的LastLogTerm, args = %+v\n", rf.me, args.CandidateId, args)
			} else {
				DPrintf("server %v 拒绝向 server %v 投票: 更短的Log, args = %+v\n", rf.me, args.CandidateId, args)
			}
			DPrintf("server %v log = %v\n", rf.me, rf.log)
		}
	} else {
		DPrintf("server %v(term is %d) 拒绝向 server %v投票: 已投票给 %v\n", rf.me, rf.currentTerm, args.CandidateId, rf.votedFor)
	}
}

func (rf *Raft) handlePreVote() {
	for i := 0; i < len(rf.peers); i++ {
		// 忽略自己
		if i == rf.me {
			continue
		}
		args := PreVoteArgs{
			// Term: rf.currentTerm + 1,
		}
		reply := PreVoteReply{}
		go rf.sendPreVote(i, &args, &reply)
	}
}
func (rf *Raft) sendPreVote(server int, args *PreVoteArgs, reply *PreVoteReply) {
	// 一直请求rpc，直到成功
	// 正式发送投票请求
	ok := rf.peers[server].Call("Raft.PreVote", args, reply)
	if !ok || !reply.IsOk {
		return
	}

	rf.mu.Lock()
	defer rf.mu.Unlock()
	if rf.state != PreCandidate {
		return
	}
	DPrintf("Server %d(term is %d) send PreVote to server %d\n", rf.me, rf.currentTerm, server)
	// 选举路上被别人干成follower了，就不用投票了
	rf.preVoteCount++
	if rf.preVoteCount > len(rf.peers)/2 && rf.state <= PreCandidate {
		DPrintf("Server %d become Candidate\n", rf.me)
		rf.state = Candidate
	}

}

// 先简单实现一个ping
func (rf *Raft) PreVote(args *PreVoteArgs, reply *PreVoteReply) {
	reply.IsOk = true
}
