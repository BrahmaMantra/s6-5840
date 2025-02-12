package raft

import (
	"math/rand"
	"time"
)

type Timer struct {
	_timer    *time.Ticker
	msgComing chan bool
}

// 单位ms
const (
	HeartBeatTimeout = 50 * time.Millisecond
	followerTimeout  = 125
	followerRand     = 150
)

// randomTime 心跳超时时间
// 每当有心跳到来时，重置Timer
func (t *Timer) reset() {
	randomTime := time.Duration(followerTimeout+rand.Intn(followerRand)) * time.Millisecond // 200~400ms
	t._timer.Reset(randomTime)                                                              // 重置时间
}
func (rf *Raft) resetHeartBeat() {
	// fmt.Printf("Server %d reset heartbeat\n", rf.me)
	rf.timer._timer.Reset(HeartBeatTimeout)
}
