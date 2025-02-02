## MIT s6.5840 lab2
2025.1.26 - 2025.1.27

### 任务
构建一个key/value server，保证操作的linearizable，Put/Append/Get methods


- cd src/kvsrv && go test

### zz瞬间
~~~ go
for ok := ck.server.Call("KVServer."+op, &args, &reply); !ok {}
~~~
我想表达的是每个循环都要执行ok := ck.server.Call("KVServer."+op, &args, &reply);但是我写出来的代码确是只在初始化的时候执行了，所以ok一直不会变

### 结果
~~~ sh
fz@Brahmamantra:~/go/src/6.5840/src/kvsrv$ go test -race
Test: one client ...
  ... Passed -- t  3.2 nrpc 14305 ops 14305
Test: many clients ...
  ... Passed -- t  3.5 nrpc 52913 ops 52913
Test: unreliable net, many clients ...
  ... Passed -- t  3.3 nrpc  1106 ops  892
Test: concurrent append to same key, unreliable ...
  ... Passed -- t  0.3 nrpc    61 ops   52
Test: memory use get ...
  ... Passed -- t  3.9 nrpc     6 ops    0
Test: memory use put ...
  ... Passed -- t  3.6 nrpc     2 ops    0
Test: memory use append ...
  ... Passed -- t  7.1 nrpc     2 ops    0
Test: memory use many put clients ...
  ... Passed -- t 20.0 nrpc 100000 ops    0
Test: memory use many get client ...
  ... Passed -- t 19.7 nrpc 100001 ops    0
Test: memory use many appends ...
2025/01/28 00:21:31 m0 451664 m1 2460976
  ... Passed -- t  4.8 nrpc  1000 ops    0
PASS
ok      6.5840/kvsrv    72.077s
~~~

### 评价
整体难度不大，只要认真读题，有一个 append返回的是Append之前的数据 这个条件要注意，用一个oldValue的map维护一下，因为题目认为没有历史遗留问题，只有调用失败问题，所以当count = 6的时候，不会有count=5的东西进来了，我们就可以在map中delete掉了