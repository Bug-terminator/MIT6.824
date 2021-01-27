package mr

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)
import "log"
import "net/rpc"
import "hash/fnv"

//
// Map functions return a slice of KeyValue.
//
type KeyValue struct {
	Key   string
	Value string
}

// for sorting by key.
type ByKey []KeyValue

// for sorting by key.
func (a ByKey) Len() int           { return len(a) }
func (a ByKey) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByKey) Less(i, j int) bool { return a[i].Key < a[j].Key }

//

// use ihash(key) % NReduce to choose the reduce
// task number for each KeyValue emitted by Map.
//
func ihash(key string) int {
	h := fnv.New32a()
	h.Write([]byte(key))
	return int(h.Sum32() & 0x7fffffff)
}

//
// main/mrworker.go calls this function.
//
func Worker(mapf func(string, string) []KeyValue,
	reducef func(string, []string) string) {

	// Your worker implementation here.

	// uncomment to send the Example RPC to the master.
	//CallExample()
	//TODO require a job
	// send finish to master
	for {
		args := Args{}
		reply := Reply{}
		call("Master.Request", &args, &reply)
		//fmt.Println(reply)  //fixme
		if !reply.Respond { //master doesn't respond
			break
		} else {
			if reply.Sleep { //need to sleep
				time.Sleep(2 * time.Second)
			} else {                      //normal task
				if reply.FileName != "" { //map task
					//slice to store bufio.writer
					var bufferQ []*bufio.Writer

					//open all related files,and bind them with buffer
					for i := 0; i < reply.NReduce; i++ {
						filename := "mr-" + strconv.Itoa(reply.Idx) + "-" + strconv.Itoa(i)
						file, err := os.OpenFile(filename, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0666)
						if err != nil {
							fmt.Println("open file failed, err:", err)
							return
						}

						defer file.Close()
						writer := bufio.NewWriter(file)

						bufferQ = append(bufferQ, writer)
					}

					//get content,invoke map function and get kva
					file, err := os.Open(reply.FileName)
					if err != nil {
						log.Fatalf("cannot open %v", reply.FileName)
					}
					content, err := ioutil.ReadAll(file)
					if err != nil {
						log.Fatalf("cannot read %v", reply.FileName)
					}
					file.Close()
					kva := mapf(reply.FileName, string(content))

					//hash key,then write them to correlated buffer
					for _, kv := range kva {
						hashKey := ihash(kv.Key) % reply.NReduce
						bufferQ[hashKey].WriteString(kv.Key + " " + kv.Value + " ")
					}
					//flush buffer
					for i, buffer := range bufferQ {
						filename := "mr-" + strconv.Itoa(reply.Idx) + "-" + strconv.Itoa(i)
						info, err := os.Stat(filename)
						if err != nil {
							log.Fatalf("cannot get file info")
						} else if info.Size() == 0 {
							fmt.Println("buffer size:%d", buffer.Size())
							buffer.Flush()

						} //FIXME might be wrong
					}
					//send finish to master
					args.Idx = reply.Idx
					args.Type = 0
					call("Master.Finish", &args, &reply)
				} else { //reduce task
					//get content from correlated files
					intermediate := []KeyValue{}
					for i := 0; i < reply.NMap; i++ {
						filename := "mr-" + strconv.Itoa(i) + "-" + strconv.Itoa(reply.Idx)
						file, err := os.Open(filename)
						if err != nil {
							log.Fatalf("cannot open %v", filename)
						}
						content, err := ioutil.ReadAll(file)
						if err != nil {
							log.Fatalf("cannot read %v", filename)
						}
						file.Close()
						//split content into string slice
						strs := strings.Fields(string(content))
						//append kv-pairs to intermediate
						for j := 0; j < len(strs); j += 2 {
							kv := KeyValue{strs[i], strs[i+1]}
							intermediate = append(intermediate, kv)
						}
						//shuffle
						sort.Sort(ByKey(intermediate))
						//bind output file to a buffer
						oname := "mr-out-" + strconv.Itoa(reply.Idx)
						ofile, err := os.OpenFile(oname, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0666)
						if err != nil {
							fmt.Println("open file failed, err:", err)
							return
						}
						defer ofile.Close()
						writer := bufio.NewWriter(ofile)
						//
						// call Reduce on each distinct key in intermediate[],
						// and print the result to mr-out-X.
						//
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
							output := reducef(intermediate[i].Key, values)

							// this is the correct format for each line of Reduce output.
							//fmt.Fprintf(ofile, "%v %v\n", intermediate[i].Key, output)
							writer.WriteString(intermediate[i].Key + " " + output + "\n")
							i = j
						}
						//flush buffer
						info, err := os.Stat(oname)
						if err != nil {
							log.Fatalf("cannot get file info")
						} else if info.Size() == 0 {
							writer.Flush()
						} //FIXME might be wrong

						//send finish to master
						args.Idx = reply.Idx
						args.Type = 1
						call("Master.Finish", &args, &reply)
					}
				}
			}
		}
	}
}

//
// example function to show how to make an RPC call to the master.
//
// the RPC argument and reply types are defined in rpc.go.
//
func CallExample() {

	// declare an argument structure.
	args := ExampleArgs{}

	// fill in the argument(s).
	args.X = 99

	// declare a reply structure.
	reply := ExampleReply{}

	// send the RPC request, wait for the reply.
	call("Master.Example", &args, &reply)

	// reply.Y should be 100.
	fmt.Printf("reply.Y %v\n", reply.Y)
}

//
// send an RPC request to the master, wait for the response.
// usually returns true.
// returns false if something goes wrong.
//
func call(rpcname string, args interface{}, reply interface{}) bool {
	// c, err := rpc.DialHTTP("tcp", "127.0.0.1"+":1234")
	sockname := masterSock()
	c, err := rpc.DialHTTP("unix", sockname)
	if err != nil {
		log.Fatal("dialing:", err)
	}
	defer c.Close()

	err = c.Call(rpcname, args, reply)
	if err == nil {
		return true
	}

	fmt.Println(err)
	return false
}
