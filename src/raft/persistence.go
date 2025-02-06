package raft

import (
	"bytes"

	"6.5840/labgob"
)

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
	// 3C
	e.Encode(rf.currentTerm)
	// DPrintf("persist():server %v persist currentTerm = %v\n", rf.me, rf.currentTerm)
	e.Encode(rf.votedFor)
	e.Encode(rf.log)
	// 3D
	e.Encode(rf.lastIncludedIndex)
	e.Encode(rf.lastIncludedTerm)

	raftstate := w.Bytes()
	rf.persister.Save(raftstate, rf.snapShot)
	// rf.persister.Save(raftstate, nil)
}

// restore previously persisted state.
func (rf *Raft) readPersist(data []byte) {
	// 目前只在Make中调用, 因此不需要锁
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

	var lastIncludedIndex int
	var lastIncludedTerm int
	if d.Decode(&currentTerm) != nil ||
		d.Decode(&votedFor) != nil ||
		d.Decode(&log) != nil ||
		d.Decode(&lastIncludedIndex) != nil ||
		d.Decode(&lastIncludedTerm) != nil {
		DPrintf("server %v readPersist failed\n", rf.me)
	} else {
		// 3C
		rf.votedFor = votedFor
		rf.currentTerm = currentTerm
		DPrintf("now server %v currentTerm = %v\n", rf.me, rf.currentTerm)
		rf.log = log
		// 3D
		rf.lastIncludedIndex = lastIncludedIndex
		rf.lastIncludedTerm = lastIncludedTerm

		rf.commitIndex = lastIncludedIndex
		rf.lastApplied = lastIncludedIndex
	}
}
func (rf *Raft) readSnapshot(data []byte) {
	// 目前只在Make中调用, 因此不需要锁
	if len(data) == 0 {
		DPrintf("server %v 读取快照失败: 无快照\n", rf.me)
		return
	}
	rf.snapShot = data
	// DPrintf("server %v 读取快照c成功\n", rf.me)
}

// the service says it has created a snapshot that has
// all info up to and including index. this means the
// service no longer needs the log through (and including)
// that index. Raft should now trim its log as much as possible.

// 上层通过这个接口调用, 传入的index是最后一个snapshot的index(在类似index %10==0的时候触发)
// 但是这个index本身我们留着，当做log[0]的位置
func (rf *Raft) Snapshot(index int, snapshot []byte) {
	// Your code here (3D).
	// DPrintf("Snapshot(): server %d wanna lock\n", rf.me)
	rf.mu.Lock()
	// DPrintf("Snapshot(): server %d lock\n", rf.me)
	defer func() {
		rf.mu.Unlock()
		// DPrintf("Snapshot(): server %d unlock\n", rf.me)
	}()
	// 如果snapshot的index比当前的snapshot的index还小
	// 如果snapshot的index比当前的commitIndex还大
	if index <= rf.lastIncludedIndex || index > rf.commitIndex {
		DPrintf("Server %d ignore snapshot request, index = %v, lastIncludedIndex = %v, commitIndex = %v\n", rf.me, index, rf.lastIncludedIndex, rf.commitIndex)
		return
	}
	//DPrintf("Server %d receive snapshot request, index = %v, lastIncludedIndex = %v, commitIndex = %v\n", rf.me, index, rf.lastIncludedIndex, rf.commitIndex)
	// 更新snapshot,到这一步我们就认为snapshot是合法的了
	rf.snapShot = snapshot
	rf.lastIncludedTerm = rf.log[rf.RealLogIdx(index)].Term

	// 截断log
	rf.log = rf.log[rf.RealLogIdx(index):] //index位置的log被存放在索引0处
	rf.lastIncludedIndex = index
	if rf.lastApplied < index {
		rf.lastApplied = index
	}
	DPrintf("Snapshot(): now server %d lastIncludedIndex = %v, lastIncludedTerm = %v\n", rf.me, rf.lastIncludedIndex, rf.lastIncludedTerm)
	rf.persist()
}
