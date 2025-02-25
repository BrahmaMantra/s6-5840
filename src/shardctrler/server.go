package shardctrler

import (
	"sync"
	"sync/atomic"
	"time"

	"6.5840/labgob"
	"6.5840/labrpc"
	"6.5840/raft"
)

type ShardCtrler struct {
	mu      sync.RWMutex // lock for the ShardCtrler to protect the shared data
	me      int
	rf      *raft.Raft
	applyCh chan raft.ApplyMsg

	// Your data here.
	dead           int32                      // set by Kill(), to indicate the ShardCtrler instance is killed
	stateMachine   ConfigStateMachine         // the state machine to store the configuration of the shard system
	lastOperations map[int64]OperationContext // the last operation context to avoid duplicate request
	notifyChans    map[int]chan *CommandReply // the notify channel to notify the client goroutine
}

// Command handles incoming commands.
// If the command is not a Query and is a duplicate request,
// it returns the previous reply.
// Otherwise, it tries to start the command in the raft layer.
// If the current server is not the leader, it returns an error.
// If the command is successfully started, it waits for the result
// or times out after a certain period.
func (sc *ShardCtrler) Command(args *CommandArgs, reply *CommandReply) {
	sc.mu.RLock()
	// check if the command is a duplicate request(only for non-Query command)
	if args.Op != Query && sc.isDuplicateRequest(args.ClientId, args.CommandId) {
		LastReply := sc.lastOperations[args.ClientId].LastReply
		reply.Config, reply.Err = LastReply.Config, LastReply.Err
		sc.mu.RUnlock()
		return
	}
	sc.mu.RUnlock()
	// try to start the command in the raft layer
	index, _, isLeader := sc.rf.Start(Command{args})
	if !isLeader {
		reply.Err = ErrWrongLeader
		return
	}
	sc.mu.Lock()
	// get the notify channel to wait for the result
	notifyChan := sc.getNotifyChan(index)
	sc.mu.Unlock()
	select {
	case result := <-notifyChan:
		reply.Config, reply.Err = result.Config, result.Err
	case <-time.After(ExecuteTimeout):
		reply.Err = ErrTimeout
	}
	go func() {
		sc.mu.Lock()
		// remove the outdated notify channel to reduce memory footprint
		sc.removeOutdatedNotifyChan(index)
		sc.mu.Unlock()
	}()
}

// isDuplicateRequest checks if the given command from the client is a duplicate.
func (sc *ShardCtrler) isDuplicateRequest(clientId int64, commandId int64) bool {
	OperationContext, ok := sc.lastOperations[clientId]
	return ok && commandId <= OperationContext.MaxAppliedCommandId
}

// getNotifyChan gets the notification channel for the given index.
// If the channel doesn't exist, it creates one.
func (sc *ShardCtrler) getNotifyChan(index int) chan *CommandReply {
	notifyChan, ok := sc.notifyChans[index]
	if !ok {
		notifyChan = make(chan *CommandReply, 1)
		sc.notifyChans[index] = notifyChan
	}
	return notifyChan
}

// removeOutdatedNotifyChan removes the outdated notify channel for the given index.
func (sc *ShardCtrler) removeOutdatedNotifyChan(index int) {
	delete(sc.notifyChans, index)
}

// applier is the goroutine to apply the log to the state machine.
func (sc *ShardCtrler) applier() {
	for !sc.killed() {
		select {
		case message := <-sc.applyCh:
			if message.CommandValid {
				reply := new(CommandReply)
				command := message.Command.(Command)
				sc.mu.Lock()

				if command.Op != Query && sc.isDuplicateRequest(command.ClientId, command.CommandId) {
					reply = sc.lastOperations[command.ClientId].LastReply
				} else {
					reply = sc.applyLogToStateMachine(command)
					if command.Op != Query {
						sc.lastOperations[command.ClientId] = OperationContext{
							MaxAppliedCommandId: command.CommandId,
							LastReply:           reply,
						}
					}
				}

				// just notify related channel for currentTerm's log when node is leader
				if currentTerm, isLeader := sc.rf.GetState(); isLeader && message.CommandTerm == currentTerm {
					notifyChan := sc.getNotifyChan(message.CommandIndex)
					notifyChan <- reply
				}
				sc.mu.Unlock()
			}
		}
	}
}

// applyLogToStateMachine applies the command to the state machine and returns the reply.
func (sc *ShardCtrler) applyLogToStateMachine(command Command) *CommandReply {
	reply := new(CommandReply)
	switch command.Op {
	case Join:
		reply.Err = sc.stateMachine.Join(command.Servers)
	case Leave:
		reply.Err = sc.stateMachine.Leave(command.GIDs)
	case Move:
		reply.Err = sc.stateMachine.Move(command.Shard, command.GID)
	case Query:
		reply.Config, reply.Err = sc.stateMachine.Query(command.Num)
	}
	return reply
}

// the tester calls Kill() when a ShardCtrler instance won't
// be needed again. you are not required to do anything
// in Kill(), but it might be convenient to (for example)
// turn off debug output from this instance.
func (sc *ShardCtrler) Kill() {
	sc.rf.Kill()
	// Your code here, if desired.
	atomic.StoreInt32(&sc.dead, 1)
}

// killed checks if the ShardCtrler is killed.
func (sc *ShardCtrler) killed() bool {
	return atomic.LoadInt32(&sc.dead) == 1
}

type Op struct {
	// Your data here.
}

// Raft returns the underlying raft instance, needed by shardkv tester
func (sc *ShardCtrler) Raft() *raft.Raft {
	return sc.rf
}

// servers[] contains the ports of the set of
// servers that will cooperate via Raft to
// form the fault-tolerant shardctrler service.
// me is the index of the current server in servers[].
func StartServer(servers []*labrpc.ClientEnd, me int, persister *raft.Persister) *ShardCtrler {
	labgob.Register(Command{})
	apply := make(chan raft.ApplyMsg)

	sc := &ShardCtrler{
		rf:             raft.Make(servers, me, persister, apply),
		applyCh:        apply,
		stateMachine:   NewMemoryConfigStateMachine(),
		lastOperations: make(map[int64]OperationContext),
		notifyChans:    make(map[int]chan *CommandReply),
		dead:           0,
	}
	go sc.applier()
	return sc
}
