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

	commitCh chan bool
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

// the service says it has created a snapshot that has
// all info up to and including index. this means the
// service no longer needs the log through (and including)
// that index. Raft should now trim its log as much as possible.
func (rf *Raft) Snapshot(index int, snapshot []byte) {
	// Your code here (3D).

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
	DPrintf("Server %d append command %v\n", rf.me, newLogEntry)
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
				rf.persist()
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

	rf.commitCh = make(chan bool)
	rf.readPersist(persister.ReadRaftState())

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
		// //DPrintf("server %v arrive here\n", rf.me)
		// for rf.commitIndex <= rf.lastApplied {
		// 	rf.condApply.Wait()
		// }
		for rf.commitIndex > rf.lastApplied {
			// //DPrintf("rf.commitIndex = %v, rf.lastApplied = %v\n", rf.commitIndex, rf.lastApplied)
			DPrintf("server %v 准备将命令(索引为 %v ) 应用到状态机\n", rf.me, rf.lastApplied)
			rf.lastApplied += 1
			msg := &ApplyMsg{
				CommandValid: true,
				Command:      rf.log[rf.lastApplied].Command,
				CommandIndex: rf.lastApplied,
			}
			rf.applyCh <- *msg
		}
		rf.mu.Unlock()
		// time.Sleep(CommitCheckTimeInterval)
	}
}
