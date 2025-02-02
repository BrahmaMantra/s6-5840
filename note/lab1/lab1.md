## MIT s6.5840	lab1
2025.1.22--2025.1.25
### 实验前
阅读mrsequential.go，掌握mapreduce在golang中的写法
- 解释一些刚开始没看懂的操作：
~~~ go
// for sorting by key.
type ByKey []mr.KeyValue

// for sorting by key.
func (a ByKey) Len() int           { return len(a) }
func (a ByKey) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByKey) Less(i, j int) bool { return a[i].Key < a[j].Key }
~~~ 
这是因为我们后面调用 sort.Sort(ByKey(intermediate)) 的时候，sort是需要实现一个接口的，包括实现 Len Swap Less三个函数。
- mapf:将文件的所有单词按isNodigit划分为{word,"1"}的形式
- reducef将中间结果（若干kv）统计成{"word",num}的形式
### 实验任务
实现一个分布式MapReduce,由Coordinator和多个并行的workers组成（在这个实验都是在一个机子上跑）。workers通过RPC与coordinator通信，每个worker将循环向coordinator请求一个task，从一个或多个文件读取task's input，执行任务，然后将任务的output写入一个或多个文件，然后再次向coordinator请求新任务。如果worker没有在合理的时间内（这个lab是10s）完成任务，coordinator就应该注意到，并将相同的任务交给不同的工作器。
- coordinator，worker的主要逻辑在main/mrcoordinator.go和main/mrworker.go（这两个是不能动的）
- 我们应该将自己的实现放到mr/coordinator.go,mr/worker.go和mr/rpc.go里面

### 流程
在main/mrcoordinator.go里面调用了MakeCoordinator，初始化一个Coordinator，然后在里面初始化任务，随后我们在main/mrworker.go里面启动Worker的入口函数，从Coordinator里面领取一个任务处理。

1. coordinator进行Map任务的分发：coordinator有n个文件输入，需要对这些文件进行wordcount，我们不妨就以n为Map的任务个数(记为nMap)。于是，coordinator将这nMap个文件封装成任务，分发给worker，同时告知他们结果会被分成多少份，即nReduce(由调用coordinator的对象)。
2. worker执行Map工作：worker从coordinator处获得一个任务，以及nReduce。于是，worker读取任务中指定的文件，通过mapf将内容变为KeyValue的数组。接下来需要将这些中间结果Map写入nReduce个文件。对于每个kv，根据ihash(kv.key)%nReduce决定它要被写入哪个文件。因此对于第i个执行Map工作的worker，我们会得到nReduce个文件: mr-i-0至mr-i-(nReduce-1)。
3. coordinator等到所有的Map任务完成后，进行Reduce任务的分发：封装nReduce个任务，同时告知nMap。
4. worker执行Reduce工作：worker从coordinator处获得一个任务，以及nMap。于是，worker-i读取任务指定的一批文件: mr-0-i至mr-(nMap-1)-i。可以发现，这些内容就是所有输入文件中被hash到该reduce的结果的并集。之后需要先对这些内容进行sort，再调用reducef，就得到了最终结果的第i部分，写入到文件mr-out-i中。(在wordcount的例子里，就相当于将A-Z的所有出现单词划分为了nReduce份，而A开头的单词的统计结果一定在mr-out-0中)
### xx
How to run our code on the word-count MapReduce application?
- go build -buildmode=plugin ../mrapps/wc.go
#### run coordinator
- rm mr-out*
- go run mrcoordinator.go pg-*.txt  
#### run some workers: 
- go run mrworker.go wc.so

#### test
- bash test-mr.sh

### 设计思路
- worker只需要get向Coordinator调用函数，接到任务后在判断是MAP还是REDUCE，利用os.CreateTemp("", oname)处理本地中间文件的管理，处理完后post上去
- Coordinator维护一个select-channel组合，多路复用实现高性能处理
- Coordinator维护一个工号-task的map，记录谁拿走的，以及协助处理catchTimeout
- 在理想设计中，工号数量应等于mmap，但是这个实验为了简化，将mmap设为len(files),所以我们就拿taskid当做mmap了

### 遇到的坑点：
- 在用Decoder的时候，如果不用for循环包裹，会读不完文件内容（buffer大小有限）
~~~ go
		decoder := json.NewDecoder(file)
		for {
			kva := []KeyValue{}
			if err := decoder.Decode(&kva); err != nil {
				if err == io.EOF {
					fmt.Println("doReduce(): EOF")
					break
				}
				fmt.Printf("无法解码文件 %s: %v\n", name, err)
			}
			intermediate = append(intermediate, kva...)
		}
		file.Close()
~~~


- 发现select channel停止工作，可能是某个case处理有问题，导致阻塞或者死循环了
### some solution
- c.workerIdCh <- workerId + c.mMap来解决crush后工号重复问题
- Coordinator有个map会存这个workerId-taskId
- createtemp生成临时文件，避免doMap的时候因为意外导致数据出错
#### 为什么要使用 os.Rename？
- os.Rename(ofile.Name(), oname) 之所以需要，主要是出于以下几个原因：
    - 保证原子性：
        在许多操作系统中，文件重命名（Rename）是一个原子操作，即要么完全成功，要么完全失败。使用 os.Rename 可以确保文件重命名时的操作是原子性的。如果在重命名之前临时文件已经完全写入，重命名后，最终文件就能立刻可见。如果发生错误，临时文件不会留下不完整的数据。
        这种方式比直接写入最终文件更安全，因为如果在写入过程中程序崩溃，临时文件依然存在，且不会留下不完整的数据。这使得文件的处理过程更具容错性。

    - 避免部分写入的错误文件：
        如果你直接将文件写入最终目标（即直接使用 oname），在文件写入过程中可能发生程序崩溃或错误，导致目标文件只有部分内容。这会造成数据不一致和损坏。
        通过先将数据写入临时文件，然后重命名，确保只有在临时文件完整写入且关闭后，才会被重命名为最终文件名。这样，最终文件始终是完整的，没有不一致的风险。

    - 清理与安全性：
        临时文件的创建是为了确保数据写入的完整性和一致性。如果直接操作最终目标文件，可能会因为程序中途崩溃或其他错误，导致目标文件处于不一致的状态。
        使用临时文件还可以确保系统会自动清理这些临时文件（特别是在操作系统的临时目录中），如果程序正常完成，最终文件才会被保留下来。

### 结果
[lab-1-test.md](lab1-test.md)

### 评价
第一个lab，强度有点太大了，不仅是一些go语法不熟，还有一些不太常用的东西（比如CreateTemp）