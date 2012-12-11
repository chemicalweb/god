package radix

import (
	"../persistence"
	"bytes"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

type NaiveTimer struct{}

func (self NaiveTimer) ContinuousTime() int64 {
	return time.Now().UnixNano()
}

type TreeIterator func(key, value []byte, timestamp int64) (cont bool)

type TreeIndexIterator func(key, value []byte, timestamp int64, index int) (cont bool)

func cmps(mininc, maxinc bool) (mincmp, maxcmp int) {
	if mininc {
		mincmp = -1
	}
	if maxinc {
		maxcmp = 1
	}
	return
}

func newNodeIterator(f TreeIterator) nodeIterator {
	return func(key, bValue []byte, tValue *Tree, use int, timestamp int64) (cont bool) {
		return f(key, bValue, timestamp)
	}
}

func newNodeIndexIterator(f TreeIndexIterator) nodeIndexIterator {
	return func(key, bValue []byte, tValue *Tree, use int, timestamp int64, index int) (cont bool) {
		return f(key, bValue, timestamp, index)
	}
}

// Tree defines a more specialized wrapper around the node structure.
// It contains an RWMutex to make it thread safe, and it defines a simplified and limited access API.
type Tree struct {
	lock   *sync.RWMutex
	timer  Timer
	logger *persistence.Logger
	root   *node
}

func NewTree() *Tree {
	return NewTreeTimer(NaiveTimer{})
}
func NewTreeTimer(timer Timer) (result *Tree) {
	result = &Tree{
		lock:  new(sync.RWMutex),
		timer: timer,
	}
	result.root, _, _, _, _ = result.root.insert(nil, newNode(nil, nil, nil, 0, true, 0), result.timer.ContinuousTime())
	return
}
func (self *Tree) Log(dir string) *Tree {
	self.logger = persistence.NewLogger(dir)
	<-self.logger.Record()
	return self
}
func (self *Tree) Restore() *Tree {
	self.logger.Stop()
	self.logger.Play(func(op persistence.Op) {
		if op.Put {
			if op.SubKey == nil {
				self.Put(op.Key, op.Value, op.Timestamp)
			} else {
				self.SubPut(op.Key, op.SubKey, op.Value, op.Timestamp)
			}
		} else {
			if op.SubKey == nil {
				if op.Clear {
					self.root, _, _, _, _ = self.root.del(nil, rip(op.Key), treeValue, self.timer.ContinuousTime())
				} else {
					self.Del(op.Key)
				}
			} else {
				self.SubDel(op.Key, op.SubKey)
			}
		}
	})
	<-self.logger.Record()
	return self
}
func (self *Tree) log(op persistence.Op) {
	if self.logger != nil && self.logger.Recording() {
		self.logger.Dump(op)
	}
}
func (self *Tree) newTreeWith(key []Nibble, byteValue []byte, timestamp int64) (result *Tree) {
	result = NewTreeTimer(self.timer)
	result.PutTimestamp(key, byteValue, true, 0, timestamp)
	return
}
func (self *Tree) Each(f TreeIterator) {
	if self == nil {
		return
	}
	self.lock.RLock()
	defer self.lock.RUnlock()
	self.root.each(nil, byteValue, newNodeIterator(f))
}
func (self *Tree) ReverseEach(f TreeIterator) {
	if self == nil {
		return
	}
	self.lock.RLock()
	defer self.lock.RUnlock()
	self.root.reverseEach(nil, byteValue, newNodeIterator(f))
}
func (self *Tree) EachBetween(min, max []byte, mininc, maxinc bool, f TreeIterator) {
	if self == nil {
		return
	}
	self.lock.RLock()
	defer self.lock.RUnlock()
	mincmp, maxcmp := cmps(mininc, maxinc)
	self.root.eachBetween(nil, rip(min), rip(max), mincmp, maxcmp, byteValue, newNodeIterator(f))
}
func (self *Tree) ReverseEachBetween(min, max []byte, mininc, maxinc bool, f TreeIterator) {
	if self == nil {
		return
	}
	self.lock.RLock()
	defer self.lock.RUnlock()
	mincmp, maxcmp := cmps(mininc, maxinc)
	self.root.reverseEachBetween(nil, rip(min), rip(max), mincmp, maxcmp, byteValue, newNodeIterator(f))
}
func (self *Tree) IndexOf(key []byte) (index int, existed bool) {
	if self == nil {
		return
	}
	self.lock.RLock()
	defer self.lock.RUnlock()
	index, ex := self.root.indexOf(0, rip(key), byteValue, true)
	existed = ex&byteValue != 0
	return
}
func (self *Tree) ReverseIndexOf(key []byte) (index int, existed bool) {
	if self == nil {
		return
	}
	self.lock.RLock()
	defer self.lock.RUnlock()
	index, ex := self.root.indexOf(0, rip(key), byteValue, false)
	existed = ex&byteValue != 0
	return
}
func (self *Tree) EachBetweenIndex(min, max *int, f TreeIndexIterator) {
	if self == nil {
		return
	}
	self.lock.RLock()
	defer self.lock.RUnlock()
	self.root.eachBetweenIndex(nil, 0, min, max, byteValue, newNodeIndexIterator(f))
}
func (self *Tree) ReverseEachBetweenIndex(min, max *int, f TreeIndexIterator) {
	if self == nil {
		return
	}
	self.lock.RLock()
	defer self.lock.RUnlock()
	self.root.reverseEachBetweenIndex(nil, 0, min, max, byteValue, newNodeIndexIterator(f))
}
func (self *Tree) Hash() []byte {
	if self == nil {
		return nil
	}
	self.lock.RLock()
	defer self.lock.RUnlock()
	return self.root.hash
}
func (self *Tree) ToMap() (result map[string][]byte) {
	if self == nil {
		return
	}
	result = make(map[string][]byte)
	self.Each(func(key []byte, value []byte, timestamp int64) bool {
		result[hex.EncodeToString(key)] = value
		return true
	})
	return
}
func (self *Tree) String() string {
	if self == nil {
		return ""
	}
	return fmt.Sprint(self.ToMap())
}
func (self *Tree) sizeBetween(min, max []byte, mininc, maxinc bool, use int) int {
	if self == nil {
		return 0
	}
	self.lock.RLock()
	defer self.lock.RUnlock()
	mincmp, maxcmp := cmps(mininc, maxinc)
	return self.root.sizeBetween(nil, rip(min), rip(max), mincmp, maxcmp, use)
}
func (self *Tree) RealSizeBetween(min, max []byte, mininc, maxinc bool) int {
	return self.sizeBetween(min, max, mininc, maxinc, 0)
}
func (self *Tree) SizeBetween(min, max []byte, mininc, maxinc bool) int {
	return self.sizeBetween(min, max, mininc, maxinc, byteValue|treeValue)
}
func (self *Tree) RealSize() int {
	if self == nil {
		return 0
	}
	self.lock.RLock()
	defer self.lock.RUnlock()
	return self.root.realSize
}
func (self *Tree) Size() int {
	if self == nil {
		return 0
	}
	self.lock.RLock()
	defer self.lock.RUnlock()
	return self.root.byteSize + self.root.treeSize
}
func (self *Tree) describeIndented(first, indent int) string {
	if self == nil {
		return ""
	}
	indentation := &bytes.Buffer{}
	for i := 0; i < first; i++ {
		fmt.Fprint(indentation, " ")
	}
	buffer := bytes.NewBufferString(fmt.Sprintf("%v<Radix size:%v hash:%v>\n", indentation, self.Size(), hex.EncodeToString(self.Hash())))
	self.root.describe(indent+2, buffer)
	return string(buffer.Bytes())
}
func (self *Tree) Describe() string {
	if self == nil {
		return ""
	}
	self.lock.RLock()
	defer self.lock.RUnlock()
	return self.describeIndented(0, 0)
}

func (self *Tree) FakeDel(key []byte, timestamp int64) (oldBytes []byte, oldTree *Tree, existed bool) {
	self.lock.Lock()
	defer self.lock.Unlock()
	var ex int
	self.root, oldBytes, oldTree, _, ex = self.root.fakeDel(nil, rip(key), byteValue, timestamp, self.timer.ContinuousTime())
	existed = ex&byteValue != 0
	if existed {
		self.log(persistence.Op{
			Key: key,
		})
	}
	return
}
func (self *Tree) put(key []Nibble, byteValue []byte, treeValue *Tree, use int, timestamp int64) (oldBytes []byte, oldTree *Tree, existed int) {
	self.root, oldBytes, oldTree, _, existed = self.root.insert(nil, newNode(key, byteValue, treeValue, timestamp, false, use), self.timer.ContinuousTime())
	return
}
func (self *Tree) Put(key []byte, bValue []byte, timestamp int64) (oldBytes []byte, existed bool) {
	self.lock.Lock()
	defer self.lock.Unlock()
	oldBytes, _, ex := self.put(rip(key), bValue, nil, byteValue, timestamp)
	existed = ex*byteValue != 0
	self.log(persistence.Op{
		Key:       key,
		Value:     bValue,
		Timestamp: timestamp,
		Put:       true,
	})
	return
}
func (self *Tree) Get(key []byte) (bValue []byte, timestamp int64, existed bool) {
	self.lock.RLock()
	defer self.lock.RUnlock()
	bValue, _, timestamp, ex := self.root.get(rip(key))
	existed = ex&byteValue != 0
	return
}
func (self *Tree) PrevMarker(key []byte) (prevKey []byte, existed bool) {
	if self == nil {
		return
	}
	self.lock.RLock()
	defer self.lock.RUnlock()
	self.root.reverseEachBetween(nil, nil, rip(key), 0, 0, 0, func(k, b []byte, t *Tree, u int, v int64) bool {
		prevKey, existed = k, true
		return false
	})
	return
}
func (self *Tree) NextMarker(key []byte) (nextKey []byte, existed bool) {
	if self == nil {
		return
	}
	self.lock.RLock()
	defer self.lock.RUnlock()
	self.root.eachBetween(nil, rip(key), nil, 0, 0, 0, func(k, b []byte, t *Tree, u int, v int64) bool {
		nextKey, existed = k, true
		return false
	})
	return
}
func (self *Tree) Prev(key []byte) (prevKey, prevValue []byte, prevTimestamp int64, existed bool) {
	if self == nil {
		return
	}
	self.lock.RLock()
	defer self.lock.RUnlock()
	self.root.reverseEachBetween(nil, nil, rip(key), 0, 0, byteValue, func(k, b []byte, t *Tree, u int, v int64) bool {
		prevKey, prevValue, prevTimestamp, existed = k, b, v, u != 0
		return false
	})
	return
}
func (self *Tree) Next(key []byte) (nextKey, nextValue []byte, nextTimestamp int64, existed bool) {
	if self == nil {
		return
	}
	self.lock.RLock()
	defer self.lock.RUnlock()
	self.root.eachBetween(nil, rip(key), nil, 0, 0, byteValue, func(k, b []byte, t *Tree, u int, v int64) bool {
		nextKey, nextValue, nextTimestamp, existed = k, b, v, u != 0
		return false
	})
	return
}
func (self *Tree) NextMarkerIndex(index int) (key []byte, existed bool) {
	if self == nil {
		return
	}
	self.lock.RLock()
	defer self.lock.RUnlock()
	self.root.eachBetweenIndex(nil, 0, &index, nil, 0, func(k, b []byte, t *Tree, u int, v int64, i int) bool {
		key, existed = k, true
		return false
	})
	return
}
func (self *Tree) PrevMarkerIndex(index int) (key []byte, existed bool) {
	if self == nil {
		return
	}
	self.lock.RLock()
	defer self.lock.RUnlock()
	self.root.reverseEachBetweenIndex(nil, 0, nil, &index, 0, func(k, b []byte, t *Tree, u int, v int64, i int) bool {
		key, existed = k, true
		return false
	})
	return
}
func (self *Tree) NextIndex(index int) (key, value []byte, timestamp int64, ind int, existed bool) {
	if self == nil {
		return
	}
	self.lock.RLock()
	defer self.lock.RUnlock()
	self.root.eachBetweenIndex(nil, 0, &index, nil, byteValue, func(k, b []byte, t *Tree, u int, v int64, i int) bool {
		key, value, timestamp, ind, existed = k, b, v, i, u != 0
		return false
	})
	return
}
func (self *Tree) PrevIndex(index int) (key, value []byte, timestamp int64, ind int, existed bool) {
	if self == nil {
		return
	}
	self.lock.RLock()
	defer self.lock.RUnlock()
	self.root.reverseEachBetweenIndex(nil, 0, nil, &index, byteValue, func(k, b []byte, t *Tree, u int, v int64, i int) bool {
		key, value, timestamp, ind, existed = k, b, v, i, u != 0
		return false
	})
	return
}
func (self *Tree) First() (key []byte, byteValue []byte, timestamp int64, existed bool) {
	self.Each(func(k []byte, b []byte, ver int64) bool {
		key, byteValue, timestamp, existed = k, b, ver, true
		return false
	})
	return
}
func (self *Tree) Last() (key []byte, byteValue []byte, timestamp int64, existed bool) {
	self.ReverseEach(func(k []byte, b []byte, ver int64) bool {
		key, byteValue, timestamp, existed = k, b, ver, true
		return false
	})
	return
}
func (self *Tree) Index(n int) (key []byte, byteValue []byte, timestamp int64, existed bool) {
	self.EachBetweenIndex(&n, &n, func(k []byte, b []byte, ver int64, index int) bool {
		key, byteValue, timestamp, existed = k, b, ver, true
		return false
	})
	return
}
func (self *Tree) ReverseIndex(n int) (key []byte, byteValue []byte, timestamp int64, existed bool) {
	self.ReverseEachBetweenIndex(&n, &n, func(k []byte, b []byte, ver int64, index int) bool {
		key, byteValue, timestamp, existed = k, b, ver, true
		return false
	})
	return
}
func (self *Tree) Clear(timestamp int64) (result int) {
	self.lock.Lock()
	defer self.lock.Unlock()
	result = self.root.fakeClear(nil, byteValue, timestamp, self.timer.ContinuousTime())
	if self.logger != nil {
		self.logger.Clear()
	}
	return
}
func (self *Tree) del(key []Nibble) (oldBytes []byte, existed bool) {
	var ex int
	self.root, oldBytes, _, _, ex = self.root.del(nil, key, byteValue, self.timer.ContinuousTime())
	existed = ex&byteValue != 0
	return
}

func (self *Tree) Del(key []byte) (oldBytes []byte, existed bool) {
	self.lock.Lock()
	defer self.lock.Unlock()
	oldBytes, existed = self.del(rip(key))
	if existed {
		self.log(persistence.Op{
			Key: key,
		})
	}
	return
}

func (self *Tree) SubReverseIndexOf(key, subKey []byte) (index int, existed bool) {
	self.lock.RLock()
	defer self.lock.RUnlock()
	if _, subTree, _, ex := self.root.get(rip(key)); ex&treeValue != 0 && subTree != nil {
		index, existed = subTree.ReverseIndexOf(subKey)
	}
	return
}
func (self *Tree) SubIndexOf(key, subKey []byte) (index int, existed bool) {
	self.lock.RLock()
	defer self.lock.RUnlock()
	if _, subTree, _, ex := self.root.get(rip(key)); ex&treeValue != 0 && subTree != nil {
		index, existed = subTree.IndexOf(subKey)
	}
	return
}
func (self *Tree) SubPrevIndex(key []byte, index int) (foundKey, foundValue []byte, foundTimestamp int64, foundIndex int, existed bool) {
	self.lock.RLock()
	defer self.lock.RUnlock()
	if _, subTree, _, ex := self.root.get(rip(key)); ex&treeValue != 0 && subTree != nil {
		foundKey, foundValue, foundTimestamp, foundIndex, existed = subTree.PrevIndex(index)
	}
	return
}
func (self *Tree) SubNextIndex(key []byte, index int) (foundKey, foundValue []byte, foundTimestamp int64, foundIndex int, existed bool) {
	self.lock.RLock()
	defer self.lock.RUnlock()
	if _, subTree, _, ex := self.root.get(rip(key)); ex&treeValue != 0 && subTree != nil {
		foundKey, foundValue, foundTimestamp, foundIndex, existed = subTree.NextIndex(index)
	}
	return
}
func (self *Tree) SubFirst(key []byte) (firstKey []byte, firstBytes []byte, firstTimestamp int64, existed bool) {
	self.lock.RLock()
	defer self.lock.RUnlock()
	if _, subTree, _, ex := self.root.get(rip(key)); ex&treeValue != 0 && subTree != nil {
		firstKey, firstBytes, firstTimestamp, existed = subTree.First()
	}
	return
}
func (self *Tree) SubLast(key []byte) (lastKey []byte, lastBytes []byte, lastTimestamp int64, existed bool) {
	self.lock.RLock()
	defer self.lock.RUnlock()
	if _, subTree, _, ex := self.root.get(rip(key)); ex&treeValue != 0 && subTree != nil {
		lastKey, lastBytes, lastTimestamp, existed = subTree.Last()
	}
	return
}
func (self *Tree) SubPrev(key, subKey []byte) (prevKey, prevValue []byte, prevTimestamp int64, existed bool) {
	self.lock.RLock()
	defer self.lock.RUnlock()
	if _, subTree, _, ex := self.root.get(rip(key)); ex&treeValue != 0 && subTree != nil {
		prevKey, prevValue, prevTimestamp, existed = subTree.Prev(subKey)
	}
	return
}
func (self *Tree) SubNext(key, subKey []byte) (nextKey, nextValue []byte, nextTimestamp int64, existed bool) {
	self.lock.RLock()
	defer self.lock.RUnlock()
	if _, subTree, _, ex := self.root.get(rip(key)); ex&treeValue != 0 && subTree != nil {
		nextKey, nextValue, nextTimestamp, existed = subTree.Next(subKey)
	}
	return
}
func (self *Tree) SubSize(key []byte) (result int) {
	self.lock.RLock()
	defer self.lock.RUnlock()
	if _, subTree, _, ex := self.root.get(rip(key)); ex&treeValue != 0 && subTree != nil {
		result = subTree.Size()
	}
	return
}
func (self *Tree) SubSizeBetween(key, min, max []byte, mininc, maxinc bool) (result int) {
	self.lock.RLock()
	defer self.lock.RUnlock()
	if _, subTree, _, ex := self.root.get(rip(key)); ex&treeValue != 0 && subTree != nil {
		result = subTree.SizeBetween(min, max, mininc, maxinc)
	}
	return
}
func (self *Tree) SubGet(key, subKey []byte) (byteValue []byte, timestamp int64, existed bool) {
	self.lock.RLock()
	defer self.lock.RUnlock()
	if _, subTree, _, ex := self.root.get(rip(key)); ex&treeValue != 0 && subTree != nil {
		byteValue, timestamp, existed = subTree.Get(subKey)
	}
	return
}
func (self *Tree) SubReverseEachBetween(key, min, max []byte, mininc, maxinc bool, f TreeIterator) {
	self.lock.RLock()
	defer self.lock.RUnlock()
	if _, subTree, _, ex := self.root.get(rip(key)); ex&treeValue != 0 && subTree != nil {
		subTree.ReverseEachBetween(min, max, mininc, maxinc, f)
	}
}
func (self *Tree) SubEachBetween(key, min, max []byte, mininc, maxinc bool, f TreeIterator) {
	self.lock.RLock()
	defer self.lock.RUnlock()
	if _, subTree, _, ex := self.root.get(rip(key)); ex&treeValue != 0 && subTree != nil {
		subTree.EachBetween(min, max, mininc, maxinc, f)
	}
}
func (self *Tree) SubReverseEachBetweenIndex(key []byte, min, max *int, f TreeIndexIterator) {
	self.lock.RLock()
	defer self.lock.RUnlock()
	if _, subTree, _, ex := self.root.get(rip(key)); ex&treeValue != 0 && subTree != nil {
		subTree.ReverseEachBetweenIndex(min, max, f)
	}
}
func (self *Tree) SubEachBetweenIndex(key []byte, min, max *int, f TreeIndexIterator) {
	self.lock.RLock()
	defer self.lock.RUnlock()
	if _, subTree, _, ex := self.root.get(rip(key)); ex&treeValue != 0 && subTree != nil {
		subTree.EachBetweenIndex(min, max, f)
	}
}
func (self *Tree) SubPut(key, subKey []byte, byteValue []byte, timestamp int64) (oldBytes []byte, existed bool) {
	self.lock.Lock()
	defer self.lock.Unlock()
	ripped := rip(key)
	_, subTree, subTreeTimestamp, ex := self.root.get(ripped)
	if ex&treeValue == 0 || subTree == nil {
		subTree = self.newTreeWith(rip(subKey), byteValue, timestamp)
	} else {
		oldBytes, existed = subTree.Put(subKey, byteValue, timestamp)
	}
	self.put(ripped, nil, subTree, treeValue, subTreeTimestamp)
	self.log(persistence.Op{
		Key:       key,
		SubKey:    subKey,
		Value:     byteValue,
		Timestamp: timestamp,
		Put:       true,
	})
	return
}
func (self *Tree) SubDel(key, subKey []byte) (oldBytes []byte, existed bool) {
	self.lock.Lock()
	defer self.lock.Unlock()
	ripped := rip(key)
	if _, subTree, subTreeTimestamp, ex := self.root.get(ripped); ex&treeValue != 0 && subTree != nil {
		oldBytes, existed = subTree.Del(subKey)
		if subTree.RealSize() == 0 {
			self.del(ripped)
		} else {
			self.put(ripped, nil, subTree, treeValue, subTreeTimestamp)
		}
	}
	if existed {
		self.log(persistence.Op{
			Key:    key,
			SubKey: subKey,
		})
	}
	return
}
func (self *Tree) SubFakeDel(key, subKey []byte, timestamp int64) (oldBytes []byte, existed bool) {
	self.lock.Lock()
	defer self.lock.Unlock()
	ripped := rip(key)
	if _, subTree, subTreeTimestamp, ex := self.root.get(ripped); ex&treeValue != 0 && subTree != nil {
		oldBytes, _, existed = subTree.FakeDel(subKey, timestamp)
		self.put(ripped, nil, subTree, treeValue, subTreeTimestamp)
	}
	if existed {
		self.log(persistence.Op{
			Key:    key,
			SubKey: subKey,
		})
	}
	return
}
func (self *Tree) SubClear(key []byte, timestamp int64) (removed int) {
	self.lock.Lock()
	defer self.lock.Unlock()
	ripped := rip(key)
	if _, subTree, subTreeTimestamp, ex := self.root.get(ripped); ex&treeValue != 0 && subTree != nil {
		removed = subTree.Clear(timestamp)
		self.put(ripped, nil, subTree, treeValue, subTreeTimestamp)
	}
	if removed > 0 {
		self.log(persistence.Op{
			Key:   key,
			Clear: true,
		})
	}
	return
}

func (self *Tree) Finger(key []Nibble) *Print {
	self.lock.RLock()
	defer self.lock.RUnlock()
	return self.root.finger(&Print{}, key)
}
func (self *Tree) GetTimestamp(key []Nibble) (bValue []byte, timestamp int64, present bool) {
	self.lock.RLock()
	defer self.lock.RUnlock()
	bValue, _, timestamp, ex := self.root.get(key)
	present = ex&byteValue != 0
	return
}
func (self *Tree) putTimestamp(key []Nibble, bValue []byte, treeValue *Tree, nodeUse, insertUse int, expected, timestamp int64) (result bool) {
	if _, _, current, _ := self.root.get(key); current == expected {
		result = true
		self.root, _, _, _, _ = self.root.insertHelp(nil, newNode(key, bValue, treeValue, timestamp, false, nodeUse), insertUse, self.timer.ContinuousTime())
	}
	return
}
func (self *Tree) PutTimestamp(key []Nibble, bValue []byte, present bool, expected, timestamp int64) (result bool) {
	self.lock.Lock()
	defer self.lock.Unlock()
	nodeUse := 0
	if present {
		nodeUse = byteValue
	}
	result = self.putTimestamp(key, bValue, nil, nodeUse, byteValue, expected, timestamp)
	if result {
		self.log(persistence.Op{
			Key:       stitch(key),
			Value:     bValue,
			Timestamp: timestamp,
			Put:       true,
		})
	}
	return
}
func (self *Tree) delTimestamp(key []Nibble, use int, expected int64) (result bool) {
	if _, _, current, _ := self.root.get(key); current == expected {
		result = true
		self.root, _, _, _, _ = self.root.del(nil, key, use, self.timer.ContinuousTime())
	}
	return
}
func (self *Tree) DelTimestamp(key []Nibble, expected int64) (result bool) {
	self.lock.Lock()
	defer self.lock.Unlock()
	result = self.delTimestamp(key, byteValue, expected)
	if result {
		self.log(persistence.Op{
			Key: stitch(key),
		})
	}
	return
}

func (self *Tree) SubFinger(key, subKey []Nibble) (result *Print) {
	self.lock.RLock()
	defer self.lock.RUnlock()
	if _, subTree, _, ex := self.root.get(key); ex&treeValue != 0 && subTree != nil {
		result = subTree.Finger(subKey)
	} else {
		result = &Print{}
	}
	return
}
func (self *Tree) SubGetTimestamp(key, subKey []Nibble) (byteValue []byte, timestamp int64, present bool) {
	self.lock.RLock()
	defer self.lock.RUnlock()
	if _, subTree, _, ex := self.root.get(key); ex&treeValue != 0 && subTree != nil {
		byteValue, timestamp, present = subTree.GetTimestamp(subKey)
	}
	return
}
func (self *Tree) SubPutTimestamp(key, subKey []Nibble, bValue []byte, present bool, subExpected, subTimestamp int64) (result bool) {
	self.lock.Lock()
	defer self.lock.Unlock()
	_, subTree, subTreeTimestamp, _ := self.root.get(key)
	if subTree == nil {
		result = true
		subTree = self.newTreeWith(subKey, bValue, subTimestamp)
	} else {
		result = subTree.PutTimestamp(subKey, bValue, present, subExpected, subTimestamp)
	}
	self.putTimestamp(key, nil, subTree, treeValue, treeValue, subTreeTimestamp, subTreeTimestamp)
	if result {
		self.log(persistence.Op{
			Key:       stitch(key),
			SubKey:    stitch(subKey),
			Value:     bValue,
			Timestamp: subTimestamp,
			Put:       true,
		})
	}
	return
}
func (self *Tree) SubDelTimestamp(key, subKey []Nibble, subExpected int64) (result bool) {
	self.lock.Lock()
	defer self.lock.Unlock()
	if _, subTree, subTreeTimestamp, ex := self.root.get(key); ex&treeValue != 0 && subTree != nil {
		result = subTree.DelTimestamp(subKey, subExpected)
		if subTree.Size() == 0 {
			self.delTimestamp(key, treeValue, subTreeTimestamp)
		} else {
			self.putTimestamp(key, nil, subTree, treeValue, treeValue, subTreeTimestamp, subTreeTimestamp)
		}
	}
	if result {
		self.log(persistence.Op{
			Key:    stitch(key),
			SubKey: stitch(subKey),
		})
	}
	return
}
