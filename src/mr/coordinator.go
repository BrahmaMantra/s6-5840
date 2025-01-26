package mr

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

type State int

const (
	WAIT int32 = iota
	MAP
	SHUFFLE
	REDUCE
	DONE
)

// Your lab 1 definitions here.
type Coordinator struct {
	inputFiles []string
	state      atomic.Int32
	mMap       int
	nReduce    int
	//worked_id,work_id,
	tasks              []Task
	tasksCh            chan *Task
	tasksCh_sendClosed chan struct{}

	//保存工号为workerid的worker正在处理的任务
	workerId_task map[int]*Task

	workerIdCh chan int

	askTaskCh    chan AskTaskStruct
	finishTaskCh chan FinishTaskStruct
	map2ReduceCh chan bool
	doneCh       chan bool
	mu           sync.Mutex // 添加全局互斥锁
}

// abs returns the absolute value of an integer.
func abs(x int) int {
	if x < 0 {
		return (-x) - 1
	}
	return x
}

// Your code here -- RPC handlers for the worker to call.
// 在文件顶部添加自定义的 max 函数
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func printChannelInfo(ch interface{}) {
	// v := reflect.ValueOf(ch)
	// if v.Kind() == reflect.Chan {
	// 	fmt.Printf("Channel buffer size: %d\n", v.Cap())
	// 	fmt.Printf("Channel current length: %d\n", v.Len())
	// } else {
	// 	fmt.Println("Provided value is not a channel")
	// }
}

// an example RPC handler.
//
// the RPC argument and reply types are defined in rpc.go.
func (c *Coordinator) Example(args *ExampleArgs, reply *ExampleReply) error {
	reply.Y = args.X + 1
	return nil
}

// start a thread that listens for RPCs from worker.go
func (c *Coordinator) server() {
	rpc.Register(c)
	rpc.HandleHTTP()
	//l, e := net.Listen("tcp", ":1234")
	sockname := coordinatorSock()
	os.Remove(sockname)
	l, e := net.Listen("unix", sockname)
	if e != nil {
		log.Fatal("listen error:", e)
	}
	go http.Serve(l, nil)
}

// main/mrcoordinator.go calls Done() periodically to find out
// if the entire job has finished.
func (c *Coordinator) Done() bool {
	ret := false

	// Your code here.
	for c.state.Load() != DONE {
		time.Sleep(1 * time.Second)
	}

	ret = true
	return ret
}

// create a Coordinator.
// main/mrcoordinator.go calls this function.
// nReduce is the number of reduce tasks to use.
func MakeCoordinator(files []string, nReduce int) *Coordinator {
	mMap := len(files)
	c := Coordinator{
		inputFiles:         files,
		state:              atomic.Int32{}, //WAIT
		mMap:               mMap,
		nReduce:            nReduce,
		tasksCh_sendClosed: make(chan struct{}, 1),
		workerId_task:      make(map[int]*Task),
		tasksCh:            make(chan *Task, max(len(files)/4, 2)),
		workerIdCh:         make(chan int, mMap),
		map2ReduceCh:       make(chan bool, 1),
		finishTaskCh:       make(chan FinishTaskStruct, 1),
		askTaskCh:          make(chan AskTaskStruct, mMap),
		doneCh:             make(chan bool, 1),
	}
	for i := 0; i < c.mMap; i++ {
		c.workerIdCh <- i
	}
	// Your code here.

	// 切成16MB-64MB大小的文件块
	// 创建map任务
	c.createMapTask()
	// 这里是启动Coordinator的RPC服务
	c.server()
	// 启动一个goroutine检查任务是否超时
	go c.catchTimeout()

	c.run()

	return &c
}

func (c *Coordinator) createMapTask() {
	// 暂时不切分文件
	for i, filename := range c.inputFiles {
		// open and read file
		task := Task{
			Metadata: TaskMeta{
				TaskId:   i,
				Filename: filename,
				State:    State(MAP),
			},
			Status: Idle,
		}
		c.tasks = append(c.tasks, task)
	}
	// 把任务写入 channel
	go c.mapTaskBegin()

}
func (c *Coordinator) mapTaskBegin() {
	// 将 tasks 切片中的任务逐个写入 channel
	// 这种做法是拷贝了一份任务，而不是直接传递指针
	for i := range c.tasks {
		c.tasksCh <- &c.tasks[i]
	}
	// close(c.tasksCh) // 关闭 channel，表示所有任务已发送完毕
	c.tasksCh_sendClosed <- struct{}{}
}

func (c *Coordinator) createReduceTask() {
	// 暂时不切分文件
	c.tasks = nil
	for i := 0; i < c.nReduce; i++ {
		// open and read file
		task := Task{
			Metadata: TaskMeta{
				TaskId:   (-i) - 1,
				Filename: "",
				State:    State(REDUCE),
			},
			Status: Idle,
		}
		c.tasks = append(c.tasks, task)
	}
	// 把任务写入 channel
	go c.reduceTaskBegin()
}
func (c *Coordinator) reduceTaskBegin() {
	// 将 tasks 切片中的任务逐个写入 channel
	// 这种做法是拷贝了一份任务，而不是直接传递指针
	for i := range c.tasks {
		c.tasksCh <- &c.tasks[i]
		// fmt.Println("reduceTaskBegin(): c.tasks[i] ", c.tasks[i])
	}
	// close(c.tasksCh) // 关闭 channel，表示所有任务已发送完毕
	c.tasksCh_sendClosed <- struct{}{}
}

func (c *Coordinator) run() {
	c.state.Store(MAP)
	for {
		c.mu.Lock()
		select {
		// stru包含了reply和ok两个字段
		case stru := <-c.askTaskCh:
			printChannelInfo(c.askTaskCh)
			c.doAskTask(&stru)
		case stru := <-c.finishTaskCh:
			printChannelInfo(c.finishTaskCh)
			c.doFinishTask(&stru)
		case <-c.map2ReduceCh: // 执行Shuffle操作
			c.map2Reduce()
		case <-c.doneCh:
			c.doEnd()
			c.mu.Unlock()
			return
		}
		c.mu.Unlock()
	}
}

// 向worker暴露的Ask RPC接口
func (c *Coordinator) AskTask(args *AskTaskArgs, reply *AskTaskReply) error {
	stru := AskTaskStruct{Reply: reply, Ok: make(chan struct{})}
	c.askTaskCh <- stru
	<-stru.Ok
	// fmt.Println("AskTask(): stru.ok")
	return nil
}
func (c *Coordinator) doAskTask(stru *AskTaskStruct) {
	// 不在MAP或REDUCE状态，或者任务已经分完了
	if !(c.state.Load() == MAP || c.state.Load() == REDUCE) || len(c.tasksCh) == 0 {
		time.Sleep(500 * time.Microsecond)
		stru.Ok <- struct{}{}
		return
	}
	// if (c.state == MAP || c.state == REDUCE) && len(c.tasks) == 0 {
	// 	stru.Ok <- struct{}{}
	// 	return
	// }

	// Coordinator为其分配任务
	task, ok := <-c.tasksCh
	if !ok {
		// 通道已关闭，处理这种情况
		fmt.Println("tasksCh 已关闭")
		// 你可以根据需要设置默认值或返回错误
		stru.Reply.Task = Task{} // 设置为零值
		stru.Reply.NReduce = 0
		return
	}
	task.Metadata.StartWorkTime = time.Now()
	task.Metadata.State = State(c.state.Load())
	task.Status = Inprogress

	//Coordinator为其分配工号
	if len(c.workerIdCh) == 0 {
		stru.Ok <- struct{}{}
		return
	}
	stru.Reply.WorkerId = <-c.workerIdCh
	// fmt.Printf("分配一个workerId: %d , taskId:%d \n", stru.Reply.WorkerId, task.Metadata.TaskId)
	// 存储工号与任务的映射
	stru.Reply.NReduce = c.nReduce
	stru.Reply.MMap = c.mMap
	stru.Reply.TaskBeginTime = task.Metadata.StartWorkTime
	// 确认工号和任务都已分配

	stru.Reply.Task = *task
	c.workerId_task[stru.Reply.WorkerId] = task
	stru.Ok <- struct{}{}
}

// 向worker暴露的Finish RPC接口
func (c *Coordinator) FinishTask(args *FinishTaskArgs, reply *FinishTaskReply) error {
	stru := FinishTaskStruct{Args: *args, Reply: reply, Ok: make(chan struct{})}
	// fmt.Printf("FinishTask(): workerId %d, taskId %d\n", args.WorkerId, args.TaskId)
	// 打印通道信息
	// printChannelInfo(c.finishTaskCh)
	c.finishTaskCh <- stru
	<-stru.Ok
	return nil
}
func (c *Coordinator) doFinishTask(stru *FinishTaskStruct) {
	workerId := stru.Args.WorkerId
	// 检查 workerId_task 是否为 nil
	task, ok := c.workerId_task[workerId]
	if !ok || task == nil || task.Status == Completed {
		time.Sleep(2000 * time.Microsecond)
		// fmt.Printf("workerId %d has no task or task is completed\n", workerId)
		stru.Ok <- struct{}{}
		return
	}
	_time := stru.Args.TaskBeginTime.Truncate(time.Second)
	_startWorkTime := task.Metadata.StartWorkTime.Truncate(time.Second)
	if !_time.Equal(_startWorkTime) {
		// fmt.Println("doFinishTask(): workerId ", workerId, " taskBeginTime ", _time, " startWorkTime ", _startWorkTime)
		stru.Ok <- struct{}{}
		return
	}
	// 任务完成后,将工号和任务的映射删除,并将工号重新放入workerIdCh给下一个worker使用(如果是map任务的话)
	// fixme:其实这个任务并不需要放回去
	delete(c.workerId_task, workerId)
	// fmt.Printf("delete workerId %d\n", workerId)
	task.Status = Completed
	c.workerIdCh <- workerId // mmap不等于task数量
	// 所有的任务都已被放入缓冲区，并且缓冲区的任务都丢给worker处理了
	if len(c.tasksCh_sendClosed) != 0 && len(c.tasksCh) == 0 {
		// 所有任务都已完成
		// fmt.Printf("doFinishTask(): all tasks are done\n")
		if len(c.workerId_task) == 0 {
			<-c.tasksCh_sendClosed
			c.state.Add(1) //切换到下一个状态
			// fmt.Println("doFinishTask(): c.state ", c.state.Load())
			switch c.state.Load() {
			case SHUFFLE:
				c.map2ReduceCh <- true
				// fmt.Println("doFinishTask(): map2ReduceCh")
			case DONE:
				c.doneCh <- true
			default:
				panic("doFinishTask():unexcepted state")
			}
		} else {
			//TODO backup操作
			// fmt.Printf("workerId_task: %v\n", c.workerId_task)
		}
	}
	stru.Ok <- struct{}{}
}
func (c *Coordinator) catchTimeout() {
	for {
		time.Sleep(5 * time.Second)
		c.mu.Lock()
		for workerId, task := range c.workerId_task {
			if task.Status != Inprogress {
				continue
			}
			if time.Since(c.tasks[abs(task.Metadata.TaskId)].Metadata.StartWorkTime) > 10*time.Second {
				// 超时处理
				// fmt.Printf("now: %v, startWorkTime: %v\n", time.Now(), c.tasks[abs(task.Metadata.TaskId)].Metadata.StartWorkTime)
				// 重新分配任务
				c.workerIdCh <- workerId
				// 删除映射
				delete(c.workerId_task, workerId)
				c.tasksCh <- &c.tasks[abs(task.Metadata.TaskId)]
				// fmt.Printf("catchTimeout(): worker %d ,taskId %d\n", workerId, task.Metadata.TaskId)
			}
		}
		c.mu.Unlock()
	}
}
func (c *Coordinator) map2Reduce() {
	if c.state.Load() != SHUFFLE {
		log.Fatal("map2Reduce(): state is not INTERMIDIATE")
	}
	c.workerId_task = make(map[int]*Task)
	c.workerIdCh = make(chan int, c.nReduce)
	c.tasksCh = make(chan *Task, max(c.nReduce/2, 2))
	for i := 0; i < c.nReduce; i++ {
		c.workerIdCh <- i
	}
	c.createReduceTask()
	// 切换成reduce状态
	c.state.Store(REDUCE)
}
func (c *Coordinator) doEnd() {

}
