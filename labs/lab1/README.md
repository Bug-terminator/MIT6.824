### Lab1 MapReduce

MapReduce有两个阶段：拆分处理（Map）和归一统计（Reduce）。

**1、实验**

要设计一个MapReduce系统，我们需要考虑一些问题：

1. 一个master，worker需要包含哪些成员
2. 如何表示任务，任务用什么样的方式进行分发

我们希望系统按照下图的方式运行：worker向master请求任务，master根据实际情况分发任务（先Map，后Reduce）。

![img](https://github.com/Bug-terminator/MIT6.824/blob/master/labs/lab1/%E6%B5%81%E7%A8%8B%E5%9B%BE.jpg)

知晓了整个流程框架，我们回到第一个问题，应该设计怎么样的成员变量。

1. 首先维护一个Map队列和一个Reduce队列：

```go
//Map队列成员
type MapTask struct {
	mu sync.Mutex//队列中的每一个任务使用一把锁，与整个队列维护一把锁相比，粒度更低，更利于并发。
	fileName string
	state int // 表示状态：0/1/2分别表示尚未完成/正在进行/已经完成
}
//Map队列
type MapQueue struct {
	finishNum, totalNum int
	q []MapTask
}
//检查Map任务是否结束，这是master决定是否分发Reduce任务的标准
func ( m *MapQueue) done() bool{
	return m.finishNum == m.totalNum
}
//Reduce队列成员
type ReduceTask struct {
	mu sync.Mutex
	state int
}
//Reduce队列
type ReduceQueue struct {
	finishNum, totalNum int
	q []ReduceTask
}
//检查Reduce是否完成，这是衡量整个任务结束的标准
func ( m *ReduceQueue) done() bool{
	return m.finishNum == m.totalNum
}
```

2. 然后需要在master中定义两个RCP处理函数，分别为Request和Finish，用于结点请求任务的分发和结点完成任务的汇报。

```go
//每个结点只负责请求任务，具体分配到的任务是Map还是Reduce，需要master来决定
func (m *master) Request(args *Args, reply *Reply) error{
	m.mu.Lock()
	defer m.mu.Unlock()
	reply.Respond = true
	if m.mpq.done(){//Map完成，从Reduce队列中分配任务
		//遍历Reduce队列，找到一个尚未开始的任务（状态为0）
		for i,tsk := range m.rdq.q{
			if tsk.state == 0{
				tsk.state = 1
				reply.Idx = i
				reply.NMap = m.mpq.totalNum
				return nil
			}
		}
	}else {//Map尚未完成，从Map队列中分配任务
		//遍历Reduce队列，找到一个尚未开始的任务
		for i,tsk := range m.mpq.q{
			if tsk.state == 0{
				tsk.state = 1
				reply.Idx = i
				reply.NReduce = m.rdq.totalNum
				reply.FileName = tsk.fileName
				return nil
			}
		}
	}
	//如果没有找到尚未开始的任务，那么返回Sleep，让该结点睡眠一段时间
	reply.Sleep = true
	return nil
}
```

```go
unc (m *master) Finish(args *Args, reply *Reply) error{
	m.mu.Lock()
	defer m.mu.Unlock()
	if args.Type == 0{//类型为0，表示Map任务的结束
		tsk := &m.mpq.q[args.Idx]
		tsk.mu.Lock()
		if tsk.state != 2{
			m.mpq.finishNum++
			tsk.state = 2
		}
		tsk.mu.Unlock()
	}else{//Reduce任务结束
		tsk := &m.rdq.q[args.Idx]
		tsk.mu.Lock()
		if tsk.state != 2{
			m.rdq.finishNum++
			tsk.state = 2
		}
		tsk.mu.Unlock()
	}
	return nil
}
```

3. master数据结构：Map队列+Reduce队列+互斥锁：

```go
type master struct {
	mu sync.Mutex
	mpq MapQueue
	rdq ReduceQueue
}
```

测试程序会调用Makemaster方法创建一个master对象，我们需要根据传入的文件名和Reduce任务的数量来初始化我们的Map队列和Reduce队列：

```go
func Makemaster(files []string, nReduce int) *master {
	m := master{}
	//根据传入的文件切片来初始化Map队列
	m.mpq.totalNum = len(files)
	for _,file := range files{
		newTask := MapTask{fileName:file}
		m.mpq.q = append(m.mpq.q, newTask)
	}
	//根据传入的Reduce数量来初始化Reduce队列
	m.rdq.totalNum = nReduce
	m.rdq.q = make([]ReduceTask, nReduce)
	m.server()//这将开启一个线程用来监听worker发来的RPC
	return &m
}
```

测试程序会循环调用Done方法检查整个程序是否已经结束了，很简单，判断Reduce任务是否完成即可：

```go
func (m *master) Done() bool {
	return 	m.rdq.done()
}
```

至此，master部分的代码已经完成了。

在写Worker部分的代码时，让我们先回顾一下整个MapReduce的流程：

用单词统计作为例子：

- 我有一堆文件需要统计单词的数量，首先将把文件分配到各个Map，它们会统计字符，并且输出键值对：

> Hello, my name is gyh, your name?
> Hello, your name is nice

- 上述两句话，分配给两个Map任务，将得到：

> 1: <hello,1> <my,1> <name,1> <is,1> <gyh,1> <your,1><name,1>
> 2: <hello,1> <name,1> <is,1> <your,1> <nice,1>

- 紧接着，在将输出的键值对分配给Reduce任务之前，我们需要根据键来把相同的单词哈希到相同的文件中去。但是在实际中，由于Map任务和Reduce任务均运行在不同的结点中，我们无法把相同的单词都汇总在同一个文件里，所以我们需要将相同的单词哈希到同一组文件中去，假设有N个Map任务，M个Reduce任务，那么中间文件的个数将达到MXN个。我们约定中间文件的命名为mr-X-Y,其中X表示Map任务号，Y表示Reduce任务号，哈希函数会把相同的单词哈希成同一个32位整型数，我们用这个数对Reduce任务数取余就能得到中间文件的后缀：

```go
func ihash(key string) int {
	h := fnv.New32a()
	h.Write([]byte(key))
	return int(h.Sum32() & 0x7fffffff)
}
for _, kv := range kva {
	hashKey := ihash(kv.Key) % reply.NReduce
	encoderQ[hashKey].Encode(&kv)
}
```

- 那么上面两句话经过哈希将产生下面四个文件：

> mr-1-1:<hello,1><name,1><your,1><name,1>
>
> mr-1-2:<my,1><is,1><gyh,1>
>
> mr-2-1:<hello,1> <name,1><your,1>
>
> mr-2-2: <is,1><nice,1>

- Reduce任务就根据自身的号来这读取些文件，将它们汇总成一个文件：

> Map任务1：读入mr-1-1和mr-2-1并汇总：<hello,1><name,1><your,1><name,1><hello,1> <name,1><your,1>
>
> Map任务2：读入mr-1-2和mr-2-2并汇总：<my,1><is,1><gyh,1><is,1><nice,1>

- 可以发现相同的单词一定会在相同的后缀文件中出现，我们现在需要排一下序（也就是所谓的shuffle）：

> <name,1><name,1><name,1><hello,1><hello,1><your,1><your,1>
> <gyh,1><is,1><is,1><my,1><nice,1>

- 然后遍历并统计连续单词的数量得到最终答案：

> <name,3><hello,2><your,2>
>
> <gyh,1><is,2><my,1><nice,1>

以上便是整个MapReduce的流程，在实现过程中，还有几点细节需要注意：

1. 由于实验是在单机下运行的，所以依赖于单机的文件系统，但是在实际中，运行的分布式集群必须使用GFS这种分布式文件系统。

2. Map任务需要一种方法将切片中的键值对打包到文件中，Reduce任务需要一种方法将文件中的键值对读入切片，一种比较好的办法就是使用Go的`encoding/json`包。

   ```go
   //打包 
   enc := json.NewEncoder(file)
     for _, kv := ... {
       err := enc.Encode(&kv)
       dec := json.NewDecoder(file)
     }
   //解压缩
   for {
       var kv KeyValue
       if err := dec.Decode(&kv); err != nil {
         break
       }
       kva = append(kva, kv)
   }
   ```

3. 为了确保worker在崩溃时不会有人观察到部分写入的文件，MapReduce论文提到了使用临时文件并在完全写入后对其进行原子重命名的技巧。我们可以使用 `ioutil.TempFile`创建一个临时文件，并使用`os.Rename` 原子地对其进行重命名。

##### worker代码

```go
func Worker(Mapf func(string, string) []KeyValue,
	Reducef func(string, []string) string) {
	for {
		args := Args{}
		reply := Reply{}
		call("master.Request", &args, &reply)
		if !reply.Respond { 
			break
		} else {
			if reply.Sleep { 
				time.Sleep(2 * time.Second)
			} else {
				if reply.FileName != "" { //分配到Map任务
					//创建临时文件和其对应的Encoder，我使用数组，当然也可以使用Map
					openfileQ := make([]*os.File, reply.NReduce, reply.NReduce)
					encoderQ := make([]*json.Encoder, reply.NReduce, reply.NReduce)
					for i := 0; i < reply.NReduce; i++ {
						tmp, err := ioutil.TempFile(".", "Inter")
						if err != nil {
							log.Fatalf("cannot create temp file")
						}
						openfileQ[i] = tmp
						encoderQ[i] = json.NewEncoder(tmp)
					}
                    
					//从文件中获取内容，并调用Map函数获取键值对数组
					file, err := os.Open(reply.FileName)
					if err != nil {
						log.Fatalf("cannot open %v", reply.FileName)
					}
					content, err := ioutil.ReadAll(file)
					if err != nil {
						log.Fatalf("cannot read %v", reply.FileName)
					}
					file.Close()
					kva := Mapf(reply.FileName, string(content))

					//对键进行哈希，将它们分发到不同的文件中去
					for _, kv := range kva {
						hashKey := ihash(kv.Key) % reply.NReduce
						encoderQ[hashKey].Encode(&kv)
					}

                    //关闭文件并重命名，注意关闭要在重命名之前进行，否则无法重命名
					for i, f:= range openfileQ {
						filename := "mr-" + strconv.Itoa(reply.Idx) + "-" + strconv.Itoa(i)
						fname := f.Name()
						f.Close()
						os.Rename(fname, filename)
					}
					//向master发送完成
					args.Idx = reply.Idx
					args.Type = 0
					call("master.Finish", &args, &reply)
				} else { //Reduce任务
					//从后缀与自身号码匹配的文件中读取内容，并汇总至一个文件中
					intermediate := []KeyValue{}
					for i := 0; i < reply.NMap; i++ {
						filename := "mr-" + strconv.Itoa(i) + "-" + strconv.Itoa(reply.Idx)
						file, err := os.Open(filename)
						if err != nil {
							log.Fatalf("cannot open %v", filename)
						}
						dec := json.NewDecoder(file)
						//将键值对加入中间文件
						for {
							var kv KeyValue
							if err := dec.Decode(&kv); err != nil {
								break
							}
							intermediate = append(intermediate, kv)
						}
						file.Close()
					}
					//排序（shuffle）
					sort.Sort(ByKey(intermediate))

					//创建临时输出文件
					tmp, err := ioutil.TempFile(".", "Out")
					if err != nil {
						log.Fatalf("cannot create temp file")
					}

					// 对中间文件中每一个不同的key调用Reduce函数
					// 并将结果打印到文件mr-out-X中.
					i := 0
					for i < len(intermediate) {
						j := i + 1
						for j < len(intermediate) && intermediate[j].Key == intermediate[i].Key {
							j++
						}
						values := []string{}
						for k := i; k < j; k++ {
							values = append(values, intermediate[k].Value)
						}
						output := Reducef(intermediate[i].Key, values)
						fmt.Fprintf(tmp, "%v %v\n", intermediate[i].Key, output)

						i = j
					}
					//重命名
					fname := tmp.Name()
					tmp.Close()
					oname := "mr-out-" + strconv.Itoa(reply.Idx)
					os.Rename(fname, oname)
					//向master汇报完成
					args.Idx = reply.Idx
					args.Type = 1
					call("master.Finish", &args, &reply)
				}
			}
		}
	}
}
```

##### 结果

```shell
litang@LAPTOP-A13E53DB:/mnt/c/Users/litang/githubWorkSpace/MIT-6.824/src/main$ sh test-mr.sh
--- wc test: PASS
--- indexer test: PASS
--- Map parallelism test: PASS
--- Reduce parallelism test: PASS
--- crash test: PASS
*** PASSED ALL TESTS
```

