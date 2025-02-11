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

const (
	Follower = iota //默认值
	PreCandidate
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
	state        int //当前节点的状态,follower,candidate,leader
	timer        Timer
	preVoteCount int //预投票数
	voteCount    int //投票数

	applyCh  chan ApplyMsg
	commitCh chan bool

	snapShot          []byte // 快照
	lastIncludedIndex int    // 日志中的最高索引
	lastIncludedTerm  int    // 日志中的最高Term
}

// 访问rf.log一律使用真实的切片索引, 即Real Index,因为被截断的log是不会被存储的
// 其余情况, 一律使用全局真实递增的索引Virtual Index
func (rf *Raft) RealLogIdx(vIdx int) int {
	// 调用该函数需要是加锁的状态
	return vIdx - rf.lastIncludedIndex
}

func (rf *Raft) VirtualLogIdx(rIdx int) int {
	// 调用该函数需要是加锁的状态
	return rIdx + rf.lastIncludedIndex
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
	rf.nextIndex[rf.me] = rf.VirtualLogIdx(len(rf.log)) // 好像不要，但是加了没问题

	DPrintf("leader %v 准备持久化", rf.me)
	rf.persist()
	DPrintf("Server %d append command %v\n", rf.me, newLogEntry)

	// log.Printf("Server %d append command %v\n", rf.me, newLogEntry)
	select {
	case rf.timer.msgComing <- true:
		// log.Printf("Server %d send msgComing\n", rf.me)
		// 成功发送消息
	default:
		// 通道已满，跳过发送
	}
	rf.resetHeartBeat()
	return rf.VirtualLogIdx(len(rf.log) - 1), rf.currentTerm, true

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
		case <-rf.timer.msgComing:
			DPrintf("server %v .timer.msgComing = %v\n", rf.me, &rf.timer.msgComing)
			rf.mu.Lock()
			if rf.state == Leader {
				rf.doLeader()
			}
			rf.mu.Unlock()
		case <-rf.timer._timer.C:
			// fmt.Printf("Server %d timeout\n", rf.me)
			rf.mu.Lock()
			switch rf.state {
			case Follower:
				rf.state = PreCandidate
				fallthrough
			case PreCandidate:
				rf.preVoteCount = 1
				rf.timer.reset()
				rf.handlePreVote()
			case Candidate:
				// if rf.canSendVote
				rf.currentTerm++ //election用的下一次的term是当前term+1
				rf.votedFor = rf.me
				rf.voteCount = 1
				rf.timer.reset()
				rf.persist()
				//DPrintf("Server %d start election\n", rf.me)
				// 给每个节点发送投票请求
				for i := 0; i < len(rf.peers); i++ {
					// 忽略自己
					if i == rf.me {
						continue
					}
					args := RequestVoteArgs{
						Term:         rf.currentTerm,
						CandidateId:  rf.me,
						LastLogIndex: rf.VirtualLogIdx(len(rf.log)),
						LastLogTerm:  rf.log[len(rf.log)-1].Term,
					}
					reply := RequestVoteReply{}
					// 正式发送投票请求
					go rf.sendRequestVote(i, &args, &reply)
				}
			case Leader:
				rf.doLeader()
			}
			rf.mu.Unlock() // 一把大锁锁住select
		}
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
	rf.currentTerm = 1
	rf.votedFor = -1
	rf.log = make([]LogEntry, 0) // log 从 1 开始
	rf.log = append(rf.log, LogEntry{Term: -1})
	rf.commitIndex = 0
	rf.lastApplied = 0
	rf.nextIndex = make([]int, len(peers))
	rf.matchIndex = make([]int, len(peers))
	rf.voteCount = 0
	rf.applyCh = applyCh

	// 3D 初始化，因为没快照的时候，这个值是-1
	rf.lastIncludedTerm = -1
	//150ms-350ms超时时间
	rf.timer = Timer{_timer: time.NewTicker(time.Duration(150+rand.Intn(200)) * time.Millisecond)} // initialize from state persisted before a crash
	rf.timer.msgComing = make(chan bool, 1)

	rf.commitCh = make(chan bool)
	// initialize from state persisted before a crash
	// 如果读取成功, 将覆盖log, votedFor和currentTerm
	rf.readSnapshot(persister.ReadSnapshot())
	rf.readPersist(persister.ReadRaftState())
	for i := 0; i < len(rf.nextIndex); i++ {
		rf.nextIndex[i] = rf.VirtualLogIdx(len(rf.log)) // raft中的index是从1开始的
		rf.matchIndex[i] = rf.lastIncludedIndex
	}
	// start ticker goroutine to start elections
	go rf.ticker()
	go rf.CommitChecker()
	return rf
}

func (rf *Raft) CommitChecker() {
	// 检查是否有新的commit
	for !rf.killed() {
		<-rf.commitCh
		rf.mu.Lock()
		// 由于applyCh会阻塞，所以我们创建一个缓冲区，将所有的消息缓存起来，避免长时间的锁
		DPrintf("commitIndex: %v, lastApplied: %v\n", rf.commitIndex, rf.lastApplied)
		msgBuf := make([]*ApplyMsg, 0, rf.commitIndex-rf.lastApplied)
		tmpApplied := rf.lastApplied
		for rf.commitIndex > tmpApplied {
			tmpApplied += 1
			if tmpApplied <= rf.lastIncludedIndex {
				// tmpApplied可能是snapShot中已经被截断的日志项, 这些日志项就不需要再发送了
				continue
			}
			msg := &ApplyMsg{
				CommandValid: true,
				Command:      rf.log[rf.RealLogIdx(tmpApplied)].Command,
				CommandIndex: tmpApplied,
				SnapshotTerm: rf.log[rf.RealLogIdx(tmpApplied)].Term,
			}

			msgBuf = append(msgBuf, msg)
		}
		rf.mu.Unlock()
		// DPrintf("server %v CommitChecker 释放锁mu", rf.me)

		// 注意, 在解锁后可能又出现了SnapShot进而修改了rf.lastApplied
		for _, msg := range msgBuf {
			rf.mu.Lock()
			if msg.CommandIndex != rf.lastApplied+1 {
				rf.mu.Unlock()
				continue
			}
			DPrintf("server %v 准备commit, log = %v:%v, lastIncludedIndex=%v", rf.me, msg.CommandIndex, msg.SnapshotTerm, rf.lastIncludedIndex)

			rf.mu.Unlock()
			// 注意, 在解锁后可能又出现了SnapShot进而修改了rf.lastApplied

			rf.applyCh <- *msg

			rf.mu.Lock()
			if msg.CommandIndex != rf.lastApplied+1 {
				rf.mu.Unlock()
				continue
			}
			rf.lastApplied = msg.CommandIndex
			rf.mu.Unlock()
		}
	}
}

func (rf *Raft) doLeader() {
	// fmt.Printf("Server %d send heartbeat\n", rf.me)
	rf.resetHeartBeat()
	// 给每个节点发送心跳
	for i := 0; i < len(rf.peers); i++ {
		// 忽略自己
		if i == rf.me {
			continue
		}
		// DPrintf("ticker():send heartbeat from %d to %d \n", rf.me, i)
		args := AppendEntriesArgs{
			Term:         rf.currentTerm,
			LeaderId:     rf.me,
			PrevLogIndex: max(rf.nextIndex[i]-1, 0),
			// PrevLogTerm:  prevLogTerm,
			LeaderCommit: rf.commitIndex,
		}
		sendInstallSnapshot := false
		if args.PrevLogIndex < rf.lastIncludedIndex {
			DPrintf("PrevLogIndex is %v, lastIncludedIndex is %v\n", args.PrevLogIndex, rf.lastIncludedIndex)
			// 表示Follower有落后的部分并且被截断
			sendInstallSnapshot = true
		} else if rf.VirtualLogIdx(len(rf.log)-1) > args.PrevLogIndex {
			//有新的log需要发送
			args.Entries = rf.log[rf.RealLogIdx(args.PrevLogIndex+1):]
		} else {
			// 如果没有新的log发送, 就发送一个长度为0的切片, 表示心跳
			args.Entries = make([]LogEntry, 0)
		}

		reply := AppendEntriesReply{}
		if sendInstallSnapshot {
			// 刚开始设计了这个地方，但是发现不需要，但是我也懒得删了
			DPrintf("ticker():handleInstallSnapshot from %d to %d \n", rf.me, i)
			go rf.handleInstallSnapshot(i)
		} else {
			args.PrevLogTerm = rf.log[rf.RealLogIdx(args.PrevLogIndex)].Term
			go rf.sendAppendEntries(i, &args, &reply)
		}
		// 正式发送心跳
		// go rf.sendAppendEntries(i, &args, &reply)
	}
}
