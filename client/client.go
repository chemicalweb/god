package client

import (
	"../common"
	"fmt"
	"net/rpc"
)

type Conn struct {
	ring *common.Ring
}

func NewConnRing(ring *common.Ring) *Conn {
	return &Conn{ring: ring}
}
func NewConn(addr string) (result *Conn, err error) {
	result = &Conn{common.NewRing()}
	var newNodes common.Remotes
	err = common.Switch.Call(addr, "Node.Nodes", 0, &newNodes)
	result.ring.SetNodes(newNodes)
	return
}
func MustConn(addr string) (result *Conn) {
	var err error
	if result, err = NewConn(addr); err != nil {
		panic(err)
	}
	return
}
func (self *Conn) Reconnect() {
	_, _, successor := self.ring.Remotes(nil)
	var err error
	for {
		var newNodes common.Remotes
		if err = successor.Call("Node.Ring", 0, &newNodes); err == nil {
			self.ring.SetNodes(newNodes)
			return
		}
		self.ring.Remove(*successor)
		_, _, successor = self.ring.Remotes(nil)
	}
}
func (self *Conn) Put(key, value []byte) {
	data := common.Item{
		Key:   key,
		Value: value,
	}
	_, _, successor := self.ring.Remotes(key)
	var x int
	if err := successor.Call("DHash.Put", data, &x); err != nil {
		self.Reconnect()
		self.Put(key, value)
	}
}
func (self *Conn) Get(key []byte) (value []byte, existed bool) {
	data := common.Item{
		Key: key,
	}
	currentRedundancy := self.ring.Redundancy()
	futures := make([]*rpc.Call, currentRedundancy)
	results := make([]*common.Item, currentRedundancy)
	nextKey := key
	var nextSuccessor *common.Remote
	for i := 0; i < currentRedundancy; i++ {
		_, _, nextSuccessor = self.ring.Remotes(nextKey)
		result := &common.Item{}
		results[i] = result
		futures[i] = nextSuccessor.Go("DHash.Get", data, result)
		nextKey = nextSuccessor.Pos
	}
	var result *common.Item
	for index, future := range futures {
		<-future.Done
		if future.Error != nil {
			self.Reconnect()
			return self.Get(key)
		}
		if result == nil || result.Timestamp < results[index].Timestamp {
			result = results[index]
		}
	}
	value, existed = result.Value, result.Exists
	return
}
func (self *Conn) DescribeTree(key []byte) (result string, err error) {
	_, match, _ := self.ring.Remotes(key)
	if match == nil {
		err = fmt.Errorf("No node with position %v found", common.HexEncode(key))
		return
	}
	err = match.Call("DHash.DescribeTree", 0, &result)
	return
}
func (self *Conn) Describe() string {
	return self.ring.Describe()
}
