package raft

import (
	"math/rand"
	"time"
)

type Timer struct {
	// 主要处理一般心跳
	_timer *time.Ticker

	// leader处理msg到来
	lastComing time.Time
	msgComing  chan bool
}

// 单位ms
const (
	HeartBeatTimeout = 75 * time.Millisecond
	followerTimeout  = 180
	followerRand     = 150

	// leader处理msg到来
	msgGap = 10
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
