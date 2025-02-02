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

	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	//	"6.5840/labgob"
	"6.5840/labrpc"
)

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
	commitIndex int //index of highest log entry known to be committed
	lastApplied int //index of highest log entry applied to state machine

	// Volatile state on leaders(Reinitialized after election)
	nextIndex  []int //for each server, index of the next log entry to send to that server
	matchIndex []int //for each server, index of highest log entry known to be replicated on server

	// 论文外，用于实现状态转换
	state     int //当前节点的状态,follower,candidate,leader
	timer     Timer
	voteCount int //投票数
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
	// w := new(bytes.Buffer)
	// e := labgob.NewEncoder(w)
	// e.Encode(rf.xxx)
	// e.Encode(rf.yyy)
	// raftstate := w.Bytes()
	// rf.persister.Save(raftstate, nil)
}

// restore previously persisted state.
func (rf *Raft) readPersist(data []byte) {
	if data == nil || len(data) < 1 { // bootstrap without any state?
		return
	}
	// Your code here (3C).
	// Example:
	// r := bytes.NewBuffer(data)
	// d := labgob.NewDecoder(r)
	// var xxx
	// var yyy
	// if d.Decode(&xxx) != nil ||
	//    d.Decode(&yyy) != nil {
	//   error...
	// } else {
	//   rf.xxx = xxx
	//   rf.yyy = yyy
	// }
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

	// 如果请求的term小于当前term，拒绝投票
	if args.Term < rf.currentTerm {
		// fmt.Printf("Server %d refuse vote for %d\n", rf.me, args.CandidateId)
		return
	}
	// 如果votedFor为空或候选人Id，且候选人的日志至少与接收方的日志一样新，则同意投票（§5.2，§5.4）
	// if (rf.votedFor == -1 || rf.votedFor == args.CandidateId) &&
	// 	(args.LastLogTerm > rf.log[len(rf.log)-1].term || (args.LastLogTerm == rf.log[len(rf.log)-1].term && args.LastLogIndex >= len(rf.log)) ) {
	rf.timer.reset()
	// 如果请求的term大于当前term，同意投票（这个就不是同一个election了，有人降维打击）
	if args.Term > rf.currentTerm {
		rf.currentTerm = args.Term //更新当前term
		rf.state = Follower
		rf.votedFor = -1
		// rf.persist()
	}
	// 如果term一样，没投过票，就先投票（后续看日志长度）
	// 正式确定投票
	if rf.votedFor == -1 {
		// fmt.Printf("Server %d vote for %d\n", rf.me, args.CandidateId)
		reply.VoteGranted = true
		rf.votedFor = args.CandidateId
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
			// rf.currentTerm++
			rf.state = Leader
			// fmt.Printf("Server %d become leader\n", rf.me)
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
func (rf *Raft) Start(command interface{}) (int, int, bool) {
	index := -1
	term := -1
	isLeader := true

	// Your code here (3B).

	return index, term, isLeader
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
				// fmt.Printf("Server %d start election\n", rf.me)
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
						// LastLogTerm:  rf.log[len(rf.log)-1].Term,
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
					args := AppendEntriesArgs{
						Term:     rf.currentTerm,
						LeaderId: rf.me,
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
	rf.commitIndex = -1
	rf.lastApplied = -1
	rf.nextIndex = make([]int, len(peers))
	rf.matchIndex = make([]int, len(peers))
	rf.voteCount = 0
	//200ms-350ms超时时间
	rf.timer = Timer{_timer: time.NewTicker(time.Duration(150+rand.Intn(200)) * time.Millisecond)} // initialize from state persisted before a crash

	rf.readPersist(persister.ReadRaftState())

	// start ticker goroutine to start elections
	go rf.ticker()

	return rf
}

func (rf *Raft) sendAppendEntries(server int, args *AppendEntriesArgs, reply *AppendEntriesReply) bool {
	// 一直请求rpc，直到成功
	// fmt.Printf("Server %d send heartbeat to %d\n", rf.me, server)
	if ok := rf.peers[server].Call("Raft.AppendEntries", args, reply); ok {
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
	// 不允许的leader
	if !reply.Success {
		// fmt.Printf("Server %d refuse leader %d\n", rf.me, server)
		rf.state = Follower
		rf.currentTerm = reply.Term
		rf.votedFor = -1
		rf.voteCount = 0
		rf.timer.reset()
	}
	return true
}

// follwer接收leader的心跳
func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	rf.timer.reset()
	// fmt.Printf("Server %d receive heartbeat\n", rf.me)
	// 收到rpc的term小于当前term，代表这个leader不合法
	if args.Term < rf.currentTerm {
		reply.Term = rf.currentTerm
		reply.Success = false
	} else {
		// fmt.Printf("Server %d receive heartbeat from %d\n", rf.me, args.LeaderId)
		rf.state = Follower
		rf.votedFor = args.LeaderId
		rf.currentTerm = args.Term

		reply.Term = rf.currentTerm
		reply.Success = true
	}
}
