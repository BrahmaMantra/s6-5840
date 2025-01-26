package mr

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"log"
	"net/rpc"
	"os"
	"sort"
	"time"
)

type Worker_ struct {
	id            int //mMap
	task          Task
	nReduce       int
	mMap          int
	mapf          func(string, string) []KeyValue
	reducef       func(string, []string) string
	TaskBeginTime time.Time
}

// Map functions return a slice of KeyValue.
type KeyValue struct {
	Key   string
	Value string
}

// use ihash(key) % NReduce to choose the reduce
// task number for each KeyValue emitted by Map.
func ihash(key string) int {
	h := fnv.New32a()
	h.Write([]byte(key))
	return int(h.Sum32() & 0x7fffffff)
}

// main/mrworker.go calls this function.
//  1. send an RPC to the coordinator asking for a task
//  2. modify the coordinator to respond with the filename
//     of an as-yet-unstarted map task
//  3. modify the worker to read the file,call the application Map func,as in mrsequential.go
func Worker(mapf func(string, string) []KeyValue,
	reducef func(string, []string) string) {

	worker := Worker_{
		mapf:    mapf,
		reducef: reducef,
	}
	worker.start()
}

// example function to show how to make an RPC call to the coordinator.
//
// the RPC argument and reply types are defined in rpc.go.
func CallExample() {

	// declare an argument structure.
	args := ExampleArgs{}

	// fill in the argument(s).
	args.X = 99

	// declare a reply structure.
	reply := ExampleReply{}

	// send the RPC request, wait for the reply.
	// the "Coordinator.Example" tells the
	// receiving server that we'd like to call
	// the Example() method of struct Coordinator.
	ok := call("Coordinator.Example", &args, &reply) // 第一步,发送RPC请求给Coordinator,等待回复
	if ok {
		// reply.Y should be 100.
		fmt.Printf("reply.Y %v\n", reply.Y)
	} else {
		fmt.Printf("call failed!\n")
	}
}

// send an RPC request to the coordinator, wait for the response.
// usually returns true.
// returns false if something goes wrong.
func call(rpcname string, args interface{}, reply interface{}) bool {
	// c, err := rpc.DialHTTP("tcp", "127.0.0.1"+":1234")
	sockname := coordinatorSock()
	c, err := rpc.DialHTTP("unix", sockname)
	if err != nil {
		log.Fatal("dialing:", err)
	}
	defer c.Close()

	err = c.Call(rpcname, args, reply)
	if err == nil {
		return true
	}

	// fmt.Println(err)
	return false
}

func (w *Worker_) start() {
	for {
		err := w.getOneTask() //先假设一定可以获取到任务
		if err != nil && err.Error() != "all tasks are done" {
			break
		}
		switch int32(w.task.Metadata.State) {
		case MAP:
			w.doMap()
		case REDUCE:
			// 包含了 shuffle 和 reduce 两个阶段
			w.doReduce()
		case WAIT:
			time.Sleep(5 * time.Second)
		}
	}
}

// generate the intermediate files for reduce tasks
func generateFileName(r int, MMap int) []string {
	var fileName []string
	for TaskID := 0; TaskID < MMap; TaskID++ {
		fileName = append(fileName, fmt.Sprintf("mr-%d-%d", TaskID, r))
	}
	return fileName
}

func (w *Worker_) doMap() {
	// 读取每个输入文件，
	// 将其传递给 Map 函数，
	// 累积中间 Map 输出结果。
	filename := w.task.Metadata.Filename
	// 打开文件
	file, err := os.Open(filename)
	if err != nil {
		// 如果无法打开文件，记录错误并退出
		log.Fatalf("cannot open %v", filename)
		return
	}
	// 读取文件内容
	content, err := io.ReadAll(file)
	if err != nil {
		// 如果无法读取文件，记录错误并退出
		log.Fatalf("cannot read %v", filename)
		return
	}
	file.Close()

	// 调用 Map 函数处理文件内容，生成中间结果
	immediate := w.mapf(filename, string(content))
	// fmt.Println("doMap(): len(immediate) is ", len(immediate))
	slot := make([][]KeyValue, w.nReduce)
	for _, _kva := range immediate {
		index := ihash(_kva.Key) % w.nReduce
		slot[index] = append(slot[index], _kva)
	}
	for i, inmeimmediate := range slot {
		oname := fmt.Sprintf("mr-%d-%d", w.task.Metadata.TaskId, i)
		ofile, err := os.CreateTemp("", oname)
		if err != nil {
			log.Fatalf("cannot create temp file %v", oname)
		}
		encoder := json.NewEncoder(ofile)
		for _, kv := range inmeimmediate {
			encoder.Encode(&kv)
		}
		ofile.Close()
		// Atomic file renaming：rename the tempfile to the final intermediate file
		os.Rename(ofile.Name(), oname)
	}
	call("Coordinator.FinishTask", &FinishTaskArgs{WorkerId: w.id, TaskId: w.task.Metadata.TaskId, TaskBeginTime: w.TaskBeginTime}, &FinishTaskReply{})
}

func (w *Worker_) doReduce() {
	// mr-mmap[i]-nreduce[j] => mr-out-nreduce[j]
	intermediate := []KeyValue{}
	intermediateFiles := generateFileName(-w.task.Metadata.TaskId-1, w.mMap)
	for _, filename := range intermediateFiles {
		// 打开文件
		file, err := os.Open(filename)
		if err != nil {
			fmt.Printf("无法打开文件 %s: %v\n", filename, err)
			return
		}
		// decode the file
		decoder := json.NewDecoder(file)
		for {
			var kv KeyValue
			if err := decoder.Decode(&kv); err != nil {
				break
			}
			intermediate = append(intermediate, kv)
		}
		file.Close()
	}

	// sort the intermediate key-value pairs by key
	sort.Slice(intermediate, func(i, j int) bool {
		return intermediate[i].Key < intermediate[j].Key
	})

	// 写到中间文件
	oname := fmt.Sprintf("mr-out-%d", w.task.Metadata.TaskId)
	ofile, err := os.Create(oname)
	if err != nil {
		fmt.Printf("cannot create %v\n", oname)
		return
	}
	for i := 0; i < len(intermediate); {
		j := i + 1
		for j < len(intermediate) && intermediate[j].Key == intermediate[i].Key {
			j++
		}
		values := []string{}
		for k := i; k < j; k++ {
			values = append(values, intermediate[k].Value)
		}
		output := w.reducef(intermediate[i].Key, values)
		// this is the correct format for each line of Reduce output.
		fmt.Fprintf(ofile, "%v %v\n", intermediate[i].Key, output)
		i = j
	}
	ofile.Close()
	// Atomic file renaming：rename the tempfile to the final intermediate file
	os.Rename(ofile.Name(), oname)

	call("Coordinator.FinishTask", &FinishTaskArgs{WorkerId: w.id, TaskId: w.task.Metadata.TaskId, TaskBeginTime: w.TaskBeginTime}, &FinishTaskReply{})
}

func (w *Worker_) getOneTask() error {
	args := AskTaskArgs{}
	reply := AskTaskReply{}

	res := call("Coordinator.AskTask", &args, &reply)
	if !res {
		// fmt.Println("getOneTask(): RPC 调用失败")
		return fmt.Errorf("RPC 调用失败: %v", res)
	}
	if reply.NReduce == 0 {
		// fmt.Println("getOneTask(): no task")
		return fmt.Errorf("no task")
	} else if reply.NReduce == -1 {
		return fmt.Errorf("all tasks are done")
	} else {
		// fmt.Println("getOneTask():worker_id is ", reply.WorkerId, "task_id is ", reply.Task.Metadata.TaskId)
		w.task = reply.Task
		w.id = reply.WorkerId
		w.nReduce = reply.NReduce
		w.mMap = reply.MMap
		w.TaskBeginTime = reply.TaskBeginTime
		return nil
	}
}
