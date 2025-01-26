package mr

import "time"

type Task struct {
	Status   int
	Metadata TaskMeta
}

type TaskMeta struct {
	TaskId        int
	Filename      string
	State         State
	StartWorkTime time.Time
}

const (
	Idle int = iota
	Inprogress
	Completed
)
