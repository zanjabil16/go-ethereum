package bzz

import (
	"bytes"
	// "fmt"
	"io"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/bzz/test"
)

/*
Tests TreeChunker by splitting and joining a random byte slice
*/

type chunkerTester struct {
	errors  []error
	chunks  []*Chunk
	timeout bool
}

func (self *chunkerTester) checkChunks(t *testing.T, want int) {
	l := len(self.chunks)
	if l != want {
		t.Errorf("expected %v chunks, got %v", want, l)
	}
}

func (self *chunkerTester) Split(chunker *TreeChunker, l int) (key Key, input []byte) {
	// reset
	self.errors = nil
	self.chunks = nil
	self.timeout = false

	data, slice := testDataReader(l)
	input = slice
	key = make([]byte, 32)
	chunkC := make(chan *Chunk, 1000)
	errC := chunker.Split(key, data, chunkC, nil)
	quitC := make(chan bool)
	timeout := time.After(600 * time.Second)

	go func() {
	LOOP:
		for {
			select {
			case <-timeout:
				self.timeout = true
				break LOOP

			case chunk := <-chunkC:
				if chunk != nil {
					self.chunks = append(self.chunks, chunk)
				} else {
					break LOOP
				}

			case err, ok := <-errC:
				if err != nil {
					self.errors = append(self.errors, err)
				}
				// fmt.Printf("err %v", err)
				if !ok {
					close(chunkC)
					errC = nil
				}
			}
		}
		close(quitC)
	}()
	<-quitC // waiting for it to finish
	return
}

func (self *chunkerTester) Join(chunker *TreeChunker, key Key, c int) SectionReader {
	// reset but not the chunks
	self.errors = nil
	self.timeout = false
	chunkC := make(chan *Chunk, 1000)

	reader := chunker.Join(key, chunkC)

	quitC := make(chan bool)
	timeout := time.After(600 * time.Second)
	i := 0
	go func() {
	LOOP:
		for {
			select {
			case <-quitC:
				break LOOP

			case <-timeout:
				self.timeout = true
				break LOOP

			case chunk := <-chunkC:
				i++
				// dpaLogger.DebugDetailf("TESTER: chunk request %x", chunk.Key[:4])
				// this just mocks the behaviour of a chunk store retrieval
				var found bool
				for _, ch := range self.chunks {
					if bytes.Equal(chunk.Key, ch.Key) {
						found = true
						chunk.Data = ch.Data
						chunk.Size = ch.Size
						break
					}
				}
				if !found {
					// fmt.Printf("TESTER: chunk unknown for %x", chunk.Key[:4])
				}
				close(chunk.C)
				// dpaLogger.DebugDetailf("TESTER: chunk request served %x", chunk.Key[:4])
			}
		}
	}()
	return reader
}

func testRandomData(chunker *TreeChunker, tester *chunkerTester, n int, chunks int, t *testing.T) {
	key, input := tester.Split(chunker, n)

	tester.checkChunks(t, chunks)
	time.Sleep(100 * time.Millisecond)

	reader := tester.Join(chunker, key, 0)
	output := make([]byte, n)
	_, err := reader.Read(output)
	if err != io.EOF {
		t.Errorf("read error %v\n", err)
	}
	// t.Logf(" IN: %x\nOUT: %x\n", input, output)
	if !bytes.Equal(output, input) {
		t.Errorf("input and output mismatch\n IN: %x\nOUT: %x\n", input, output)
	}
}

func TestRandomData(t *testing.T) {
	test.LogInit()
	chunker := &TreeChunker{
		Branches:     2,
		SplitTimeout: 10 * time.Second,
		JoinTimeout:  10 * time.Second,
	}
	chunker.Init()
	tester := &chunkerTester{}
	testRandomData(chunker, tester, 70, 3, t)
	testRandomData(chunker, tester, 179, 5, t)
	testRandomData(chunker, tester, 253, 7, t)
	// t.Logf("chunks %v", tester.chunks)
}

func chunkerAndTester() (chunker *TreeChunker, tester *chunkerTester) {
	chunker = &TreeChunker{
		Branches:     2,
		SplitTimeout: 10 * time.Second,
		JoinTimeout:  10 * time.Second,
	}
	chunker.Init()
	tester = &chunkerTester{}
	return
}

func readAll(reader SectionReader, result []byte) {
	size := int64(len(result))

	var end int64
	for pos := int64(0); pos < size; pos += 1000 {
		if pos+1000 > size {
			end = size
		} else {
			end = pos + 1000
		}
		reader.ReadAt(result[pos:end], pos)
	}
}

func benchmarkJoinRandomData(n int, chunks int, t *testing.B) {
	t.StopTimer()
	for i := 0; i < t.N; i++ {
		// fmt.Printf("round %v\n", i)
		chunker, tester := chunkerAndTester()
		key, slice := tester.Split(chunker, n)
		// fmt.Printf("split done %v, joining...\n", i)
		t.StartTimer()
		reader := tester.Join(chunker, key, i)
		// fmt.Printf("join done %v, reading...\n", i)
		result := make([]byte, n)
		readAll(reader, result)
		// fmt.Printf("read done %v\n", i)
		t.StopTimer()
		if !bytes.Equal(slice, result) {
			t.Errorf("input output mismatch")
		}
	}
}

func benchmarkSplitRandomData(n int, chunks int, t *testing.B) {
	defer test.Benchlog(t).Detach()
	for i := 0; i < t.N; i++ {
		chunker, tester := chunkerAndTester()
		tester.Split(chunker, n)
	}
}

func BenchmarkJoinRandomData_100_2(t *testing.B)     { benchmarkJoinRandomData(100, 3, t) }
func BenchmarkJoinRandomData_1000_2(t *testing.B)    { benchmarkJoinRandomData(1000, 3, t) }
func BenchmarkJoinRandomData_10000_2(t *testing.B)   { benchmarkJoinRandomData(10000, 3, t) }
func BenchmarkJoinRandomData_100000_2(t *testing.B)  { benchmarkJoinRandomData(100000, 3, t) }
func BenchmarkJoinRandomData_1000000_2(t *testing.B) { benchmarkJoinRandomData(1000000, 3, t) }

func BenchmarkSplitRandomData_100_2(t *testing.B)      { benchmarkSplitRandomData(100, 3, t) }
func BenchmarkSplitRandomData_1000_2(t *testing.B)     { benchmarkSplitRandomData(1000, 3, t) }
func BenchmarkSplitRandomData_10000_2(t *testing.B)    { benchmarkSplitRandomData(10000, 3, t) }
func BenchmarkSplitRandomData_100000_2(t *testing.B)   { benchmarkSplitRandomData(100000, 3, t) }
func BenchmarkSplitRandomData_1000000_2(t *testing.B)  { benchmarkSplitRandomData(1000000, 3, t) }
func BenchmarkSplitRandomData_10000000_2(t *testing.B) { benchmarkSplitRandomData(10000000, 3, t) }

// go test -bench ./bzz -cpuprofile cpu.out -memprofile mem.out
