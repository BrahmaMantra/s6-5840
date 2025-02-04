package raft

//
// this is an outline of the API that raft must expose to
// the service (or tester). see comments below for
// each of these functions for more details.
//
// rf = Make(...)
//   create a new Raft server.
// rf.Start(command interface{}) (index, term, isleader)
//   start agreement on a new log entry
// rf.GetState() (term, isLeader)
//   ask a Raft for its current term, and whether it thinks it is leader
// ApplyMsg
//   each time a new entry is committed to the log, each Raft peer
//   should send an ApplyMsg to the service (or tester)
//   in the same server.
//

import (
	//	"bytes"

	"bytes"
	"fmt"
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	//	"6.5840/labgob"
	"6.5840/labgob"
	"6.5840/labrpc"
)

func max(a int, b int) int {
	if a > b {
		return a
	} else {
		return b
	}
}

// as each Raft peer becomes aware that successive log entries are
// committed, the peer should send an ApplyMsg to the service (or
// tester) on the same server, via the applyCh passed to Make(). set
// CommandValid to true to indicate that the ApplyMsg contains a newly
// committed log entry.
//
// in part 3D you'll want to send other kinds of messages (e.g.,
// snapshots) on the applyCh, but set CommandValid to false for these
// other uses.
type ApplyMsg struct {
	CommandValid bool
	Command      interface{}
	CommandIndex int

	// For 3D:
	SnapshotValid bool
	Snapshot      []byte
	SnapshotTerm  int
	SnapshotIndex int
}

// A log entry contains a command for the state machine, and the term when the entry was received by the leader.
type LogEntry struct {
	Term    int
	Command interface{}
}

// 心跳
type AppendEntriesArgs struct {
	Term         int // leader's term
	LeaderId     int // so follower can redirect clients
	PrevLogIndex int // index of log entry immediately preceding new ones
	PrevLogTerm  int // term of prevLogIndex entry
	Entries      []LogEntry
	LeaderCommit int // leader's commitIndex
}

type AppendEntriesReply struct {
	Term    int
	Success bool
}

const HeartBeatTimeout = 75 * time.Millisecond
const CommitCheckTimeInterval = 40 * time.Millisecond
const (
	Follower = iota //默认值
	Candidate
	Leader
)

// A Go object implementing a single Raft peer.
type Raft struct {
	mu        sync.Mutex          // Lock to protect shared access to this peer's state
	peers     []*labrpc.ClientEnd // RPC end points of all peers
	persister *Persister          // Object to hold this peer's persisted state
	me        int                 // this peer's index into peers[]
	dead      int32               // set by Kill()

	// Your data here (3A, 3B, 3C).
	// Look at the paper's Figure 2 for a description of what
	// state a Raft server must maintain.

	// Persistent state on all servers
	currentTerm int        //latest term server has seen
	votedFor    int        //candidateId that received vote in current term(or null if none)
	log         []LogEntry //log entries; each entry contains command for state machine, and term when entry was received by leader(first index is 1)

	// Volatile state on all servers
	commitIndex int // 将要提交的日志的最高索引
	lastApplied int // 已经被应用到状态机的日志的最高索引

	// Volatile state on leaders(Reinitialized after election)
	// 我们认为nextIndex = 1,matchIndex = 0是起始点
	nextIndex  []int //for each server, index of the next log entry to send to that server
	matchIndex []int //for each server, index of highest log entry known to be replicated on server

	// 论文外，用于实现状态转换
	state     int //当前节点的状态,follower,candidate,leader
	timer     Timer
	voteCount int //投票数

	applyCh chan ApplyMsg
}
type Timer struct {
	_timer *time.Ticker
}

// randomTime 心跳超时时间
// 每当有心跳到来时，重置Timer
func (t *Timer) reset() {
	randomTime := time.Duration(250+rand.Intn(150)) * time.Millisecond // 250~400ms
	t._timer.Reset(randomTime)                                         // 重置时间
}
func (rf *Raft) resetHeartBeat() {
	// fmt.Printf("Server %d reset heartbeat\n", rf.me)
	rf.timer._timer.Reset(HeartBeatTimeout)
}

// return currentTerm and whether this server
// believes it is the leader.
func (rf *Raft) GetState() (int, bool) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	var term int
	var isleader bool
	// Your code here (3A).
	if rf.state == Leader {
		isleader = true
		// fmt.Println("Leader是:", rf.me)
	}
	term = rf.currentTerm
	return term, isleader
}

// save Raft's persistent state to stable storage,
// where it can later be retrieved after a crash and restart.
// see paper's Figure 2 for a description of what should be persistent.
// before you've implemented snapshots, you should pass nil as the
// second argument to persister.Save().
// after you've implemented snapshots, pass the current snapshot
// (or nil if there's not yet a snapshot).
func (rf *Raft) persist() {
	// Your code here (3C).
	// Example:
	w := new(bytes.Buffer)
	e := labgob.NewEncoder(w)
	e.Encode(rf.currentTerm)
	e.Encode(rf.votedFor)
	e.Encode(rf.log)
	raftstate := w.Bytes()
	rf.persister.Save(raftstate, nil)
}

// restore previously persisted state.
func (rf *Raft) readPersist(data []byte) {
	if data == nil || len(data) < 1 { // bootstrap without any state?
		return
	}
	// Your code here (3C).
	// Example:
	r := bytes.NewBuffer(data)
	d := labgob.NewDecoder(r)
	var currentTerm int
	var votedFor int
	var log []LogEntry
	if d.Decode(&currentTerm) != nil ||
		d.Decode(&votedFor) != nil ||
		d.Decode(&log) != nil {
		fmt.Errorf("readPersist error")
	} else {
		rf.currentTerm = currentTerm
		rf.votedFor = votedFor
		rf.log = log
	}
}

// the service says it has created a snapshot that has
// all info up to and including index. this means the
// service no longer needs the log through (and including)
// that index. Raft should now trim its log as much as possible.
func (rf *Raft) Snapshot(index int, snapshot []byte) {
	// Your code here (3D).

}

// example RequestVote RPC arguments structure.
// field names must start with capital letters!
type RequestVoteArgs struct {
	// Your data here (3A, 3B).
	Term        int //candidate's term
	CandidateId int //candidate requesting vote
	// eg. term =[4] index = 13
	LastLogIndex int //index of candidate's last log entry
	LastLogTerm  int //term of candidate's last log entry
}

// example RequestVote RPC reply structure.
// field names must start with capital letters!
type RequestVoteReply struct {
	// Your data here (3A).
	Term        int  //current term, for candidate to update itself
	VoteGranted bool //true means candidate received vote
}

// example RequestVote RPC handler.
// 接收投票请求
// 内部会锁
func (rf *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) {
	// Your code here (3A, 3B).
	rf.mu.Lock()
	defer rf.mu.Unlock()
	// fmt.Printf("Server %d receive vote request from %d\n", rf.me, args.CandidateId)
	reply.Term = rf.currentTerm
	reply.VoteGranted = false

	// //DPrintf("Server %d log = %v\n", rf.me, rf.log)
	// 如果请求的term小于当前term，拒绝投票
	if /*args.Term < rf.currentTerm &&*/ args.LastLogTerm < rf.log[len(rf.log)-1].Term {
		// //DPrintf("Server %d refuse vote for %d\n", rf.me, args.CandidateId)
		return
	}
	rf.timer.reset()
	// 在日志至少是一样新的情况下，(如果请求的term大于当前termXXXX )，同意投票（这个就不是同一个election了）
	if args.LastLogTerm >= rf.log[len(rf.log)-1].Term {
		rf.currentTerm = args.Term //更新当前term,防止孤立节点
		rf.state = Follower
		rf.votedFor = -1
		//DPrintf("Server %d find a new election in term %v from server %v\n", rf.me, args.Term, args.CandidateId)
		rf.persist()
	}

	// 如果term一样，没投过票，看日志长度
	if rf.votedFor == -1 || rf.votedFor == args.CandidateId {
		// 只有虚拟log || 它的上一个index的term比我的最后一个大 || term一样大但是它长度>=我
		//DPrintf("rf.log = %v\nargs.LastLogTerm = %v,args.LastLogIndex = %v\n", rf.log, args.LastLogTerm, args.LastLogIndex)
		if len(rf.log) == 1 || args.LastLogTerm > rf.log[len(rf.log)-1].Term ||
			(args.LastLogTerm == rf.log[len(rf.log)-1].Term && args.LastLogIndex >= len(rf.log)) {
			// 正式确定投票
			reply.VoteGranted = true
			rf.votedFor = args.CandidateId
			//DPrintf("server %v vote for server %v\n", rf.me, args.CandidateId)
			//DPrintf("[]nextindex = %v", rf.nextIndex)
		}
	}

}

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
	rf.mu.Lock()
	defer rf.mu.Unlock()
	// 如果请求的term小于当前term，拒绝投票
	if args.Term < rf.currentTerm {
		return false
	}
	// 如果同意投票，更新term
	if reply.VoteGranted {
		// 选举路上被别人干成follower了，就不用投票了
		if rf.state == Follower {
			return false
		}
		rf.voteCount++
		// fmt.Printf("Server %d vote for %d success\n", rf.me, server)
		// fmt.Printf("voteCount: %d\n", rf.voteCount)
		if rf.voteCount > len(rf.peers)/2 {
			rf.state = Leader
			//DPrintf("Server %d become leader\n", rf.me)
			// 初始化nextIndex和matchIndex，因为它们是易失性的
			for i := 0; i < len(rf.nextIndex); i++ {
				rf.nextIndex[i] = len(rf.log)
				rf.matchIndex[i] = 0
			}
			rf.resetHeartBeat()
		}
	} else {
		// fmt.Printf("Server %d vote for %d failed\n", rf.me, server)
	}
	return true
}

// the service using Raft (e.g. a k/v server) wants to start
// agreement on the next command to be appended to Raft's log. if this
// server isn't the leader, returns false. otherwise start the
// agreement and return immediately. there is no guarantee that this
// command will ever be committed to the Raft log, since the leader
// may fail or lose an election. even if the Raft instance has been killed,
// this function should return gracefully.
//
// the first return value is the index that the command will appear at
// if it's ever committed. the second return value is the current
// term. the third return value is true if this server believes it is
// the leader.

// 使用 Raft 的服务（例如 k/v 服务器）想要启动协议，
// 以便将下一个命令附加到 Raft 的日志中。如果此
// 服务器不是领导者，则返回 false。否则启动协议并立即返回。
// 无法保证此命令一定会提交到 Raft 日志，因为领导者
// 可能会失败或输掉选举。即使 Raft 实例已被终止，
// 此函数也应该正常返回。
//
// 第一个返回值是命令将出现在的索引
// （如果曾经提交过）。第二个返回值是当前
// 任期。如果此服务器认为它是
// 领导者，则第三个返回值为 true。
func (rf *Raft) Start(command interface{}) (int, int, bool) {
	// Your code here (3B).
	rf.mu.Lock()
	defer rf.mu.Unlock()
	if rf.state != Leader {
		//DPrintf("Server %d term: %d,accept command %v\n", rf.me, rf.currentTerm, command)
		return -1, -1, false
	}
	newLogEntry := LogEntry{
		Term:    rf.currentTerm,
		Command: command,
	}
	rf.log = append(rf.log, newLogEntry)
	rf.nextIndex[rf.me] = len(rf.log)

	// DPrintf("leader %v 准备持久化", rf.me)
	rf.persist()

	return len(rf.log) - 1, rf.currentTerm, true

}

// the tester doesn't halt goroutines created by Raft after each test,
// but it does call the Kill() method. your code can use killed() to
// check whether Kill() has been called. the use of atomic avoids the
// need for a lock.
//
// the issue is that long-running goroutines use memory and may chew
// up CPU time, perhaps causing later tests to fail and generating
// confusing debug output. any goroutine with a long-running loop
// should call killed() to check whether it should stop.
func (rf *Raft) Kill() {
	atomic.StoreInt32(&rf.dead, 1)
	// Your code here, if desired.
}

func (rf *Raft) killed() bool {
	z := atomic.LoadInt32(&rf.dead)
	return z == 1
}

func (rf *Raft) ticker() {
	for rf.killed() == false {
		// Your code here (3A)
		// Check if a leader election should be started.
		select {
		case <-rf.timer._timer.C:
			// fmt.Printf("Server %d timeout\n", rf.me)
			rf.mu.Lock()
			switch rf.state {
			case Follower:
				rf.state = Candidate
				// fmt.Printf("Server %d become candidate\n", rf.me)
				fallthrough
			case Candidate:
				rf.currentTerm++ //election用的下一次的term是当前term+1
				rf.votedFor = rf.me
				rf.voteCount = 1
				rf.timer.reset()
				//DPrintf("Server %d start election\n", rf.me)
				// 给每个节点发送投票请求
				for i := 0; i < len(rf.peers); i++ {
					// 忽略自己
					if i == rf.me {
						continue
					}
					// 如果在投票期间，状态改变了，就不用投票了
					// 这里是为了防止在投票期间，状态改变了，比如收到了别人的投票被干成follower了
					// 又或者你已经是leader了
					// if rf.state != Candidate {
					// 	break
					// }
					args := RequestVoteArgs{
						Term:         rf.currentTerm,
						CandidateId:  rf.me,
						LastLogIndex: len(rf.log),
						LastLogTerm:  rf.log[len(rf.log)-1].Term,
					}
					reply := RequestVoteReply{}
					// 正式发送投票请求
					go rf.sendRequestVote(i, &args, &reply)
				}
			case Leader:
				// fmt.Printf("Server %d send heartbeat\n", rf.me)
				rf.resetHeartBeat()
				// 给每个节点发送心跳
				for i := 0; i < len(rf.peers); i++ {
					// 忽略自己
					if i == rf.me {
						continue
					}
					var prevLogTerm int
					if rf.nextIndex[i] > 0 {
						prevLogTerm = rf.log[rf.nextIndex[i]-1].Term
					} else {
						prevLogTerm = -1 // 或者设置为一个默认值
					}
					args := AppendEntriesArgs{
						Term:         rf.currentTerm,
						LeaderId:     rf.me,
						PrevLogIndex: max(rf.nextIndex[i]-1, 0),
						PrevLogTerm:  prevLogTerm,
						LeaderCommit: rf.commitIndex,
					}
					// 有实际的log entry需要发送,0是虚拟的log entry
					follower_nextIndex := max(rf.nextIndex[i], 1)
					if len(rf.log)-1 >= rf.nextIndex[i] {
						// 一次性发送全部没发的entries
						args.Entries = rf.log[follower_nextIndex:]
						//DPrintf("server %v 的nextIndex: %v\n", rf.me, rf.nextIndex)
						//DPrintf("server %v 发送给server %v 的日志为: %+v\n", rf.me, i, args.Entries)
					} else {
						// 如果没有新的log发送, 就发送一个长度为0的切片, 表示心跳
						args.Entries = make([]LogEntry, 0)
					}
					reply := AppendEntriesReply{}
					// 正式发送心跳
					go rf.sendAppendEntries(i, &args, &reply)
				}
			}
			rf.mu.Unlock() // 一把大锁锁住select
		}
		// pause for a random amount of time between 50 and 350
		// milliseconds.注释掉，不需要
		// ms := 50 + (rand.Int63() % 300)
		// time.Sleep(time.Duration(ms) * time.Millisecond)
		time.Sleep(10 * time.Millisecond)
	}
}

// the service or tester wants to create a Raft server. the ports
// of all the Raft servers (including this one) are in peers[]. this
// server's port is peers[me]. all the servers' peers[] arrays
// have the same order. persister is a place for this server to
// save its persistent state, and also initially holds the most
// recent saved state, if any. applyCh is a channel on which the
// tester or service expects Raft to send ApplyMsg messages.
// Make() must return quickly, so it should start goroutines
// for any long-running work.
// 服务或测试人员想要创建一个 Raft 服务器。所有 Raft 服务器（包括这个）的端口
// 都在 peers[] 中。这个服务器的端口是 peers[me]。所有服务器的 peers[] 数组
// 具有相同的顺序。persister 是此服务器保存其持久状态的地方，并且最初还保存最近保存的状态（如果有）。
// applyCh 是测试人员或服务期望 Raft 发送 ApplyMsg 消息的通道。
// Make() 必须快速返回，因此它应该为任何长时间运行的工作启动 goroutines。
func Make(peers []*labrpc.ClientEnd, me int,
	persister *Persister, applyCh chan ApplyMsg) *Raft {
	rf := &Raft{}
	rf.peers = peers
	rf.persister = persister
	rf.me = me

	// Your initialization code here (3A, 3B, 3C).
	// 初始化 Raft 的状态
	rf.state = Follower
	rf.currentTerm = 0
	rf.votedFor = -1
	rf.log = make([]LogEntry, 0) // log 从 0 开始
	rf.log = append(rf.log, LogEntry{Term: -1})
	rf.commitIndex = 0
	rf.lastApplied = 0
	rf.nextIndex = make([]int, len(peers))
	rf.matchIndex = make([]int, len(peers))
	rf.voteCount = 0
	rf.applyCh = applyCh
	//200ms-350ms超时时间
	rf.timer = Timer{_timer: time.NewTicker(time.Duration(150+rand.Intn(200)) * time.Millisecond)} // initialize from state persisted before a crash

	rf.readPersist(persister.ReadRaftState())

	// start ticker goroutine to start elections
	go rf.ticker()
	go rf.CommitChecker()
	return rf
}

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
			DPrintf("Server %d 退位\n", rf.me)
			return false
		}

		// term仍然相同, 且自己还是leader, 表示prevLogIndex-prevLogTerm匹配的项不对
		// 将nextIndex自减再重试，（后续改成term自减的那个index重试）
		if reply.Term == rf.currentTerm && rf.state == Leader {
			// 找到log中term自减的第一个index
			for i := rf.nextIndex[server] - 1; i >= 0; i-- {
				if rf.log[i].Term < rf.log[rf.nextIndex[server]-1].Term {
					rf.nextIndex[server] = i + 1
					DPrintf("server %v 的nextIndex: %v\n", server, rf.nextIndex[server])
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
	if args.Term < rf.currentTerm && args.PrevLogTerm < rf.log[len(rf.log)-1].Term {
		// follower觉得leader不合法
		reply.Term = rf.currentTerm
		reply.Success = false
		//DPrintf("server %v 检查到server %v 的term不合法\n", rf.me, args.LeaderId)
		return
	}
	rf.currentTerm = args.Term
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
	rf.log = append(rf.log, args.Entries...)
	DPrintf("server %v 将要append的日志为: %+v\n", rf.me, args.Entries)
	DPrintf("server %v 成功进行apeend from server %d, log: %+v\n", rf.me, args.LeaderId, rf.log)
	rf.persist()
	reply.Success = true
	reply.Term = rf.currentTerm
	if args.LeaderCommit > rf.commitIndex {
		// 5.If leaderCommit > commitIndex, set commitIndex = min(leaderCommit, index of last new entry)
		rf.commitIndex = int(math.Min(float64(args.LeaderCommit), float64(len(rf.log)-1)))
	}
	// //DPrintf("server %v 的commitIndex is %v", rf.me, rf.commitIndex)
}
func (rf *Raft) CommitChecker() {
	// 检查是否有新的commit
	for !rf.killed() {
		rf.mu.Lock()
		// //DPrintf("server %v arrive here\n", rf.me)
		for rf.commitIndex > rf.lastApplied {
			// //DPrintf("rf.commitIndex = %v, rf.lastApplied = %v\n", rf.commitIndex, rf.lastApplied)
			rf.lastApplied += 1
			msg := &ApplyMsg{
				CommandValid: true,
				Command:      rf.log[rf.lastApplied].Command,
				CommandIndex: rf.lastApplied,
			}
			rf.applyCh <- *msg
			//DPrintf("server %v 准备将命令(索引为 %v ) 应用到状态机\n", rf.me, msg.CommandIndex)
		}
		rf.mu.Unlock()
		time.Sleep(CommitCheckTimeInterval)
	}
}
