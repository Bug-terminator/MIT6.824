package mr

import (
	"log"
	"sync"
)
import "net"
import "os"
import "net/rpc"
import "net/http"

//map task
type MapTask struct {
	fileName string
	state int // 0/1/2 not_started/undergoing/finish
}
//map queue
type MapQueue struct {
	mu sync.Mutex
	finishNum, totalNum int
	q []MapTask
}

//check if done
func ( m *MapQueue) done() bool{
	return m.finishNum == m.totalNum
}

//reduce queue
type ReduceQueue struct {
	mu sync.Mutex
	finishNum, totalNum int
	q []int
}
func ( m *ReduceQueue) done() bool{
	return m.finishNum == m.totalNum
}

type Master struct {
	// Your definitions here.
	mpq MapQueue
	rdq ReduceQueue
}

//TODO Your code here -- RPC handlers for the worker to call.

//
// an example RPC handler.
//
// the RPC argument and reply types are defined in rpc.go.
//
func (m *Master) Example(args *ExampleArgs, reply *ExampleReply) error {
	reply.Y = args.X + 1
	return nil
}


//
// start a thread that listens for RPCs from worker.go
//
func (m *Master) server() {
	rpc.Register(m)
	rpc.HandleHTTP()
	//l, e := net.Listen("tcp", ":1234")
	sockname := masterSock()
	os.Remove(sockname)
	l, e := net.Listen("unix", sockname)
	if e != nil {
		log.Fatal("listen error:", e)
	}
	go http.Serve(l, nil)
}

//TODO
// main/mrmaster.go calls Done() periodically to find out
// if the entire job has finished.
//
func (m *Master) Done() bool {
	ret := false

	// Your code here.


	return ret
}

//
// create a Master.
// main/mrmaster.go calls this function.
// nReduce is the number of reduce tasks to use.
//
func MakeMaster(files []string, nReduce int) *Master {
	m := Master{}

	// TODO Your code here.
	// init map queue
	m.mpq.totalNum = len(files)
	for _,file := range files{
		newTask := MapTask{file,0}
		m.mpq.q = append(m.mpq.q, newTask)
	}
	// init reduce queue
	m.rdq.totalNum = nReduce
	m.rdq.q = make([]int, nReduce)

	m.server()
	return &m
}
