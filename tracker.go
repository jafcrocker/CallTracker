package tracker

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Config struct {
	// interval for checking threshold
	interval time.Duration
	// requests per second which will trigger tracking
	thresholdStartRate float64
	// requests per second which will stop tracking
	thresholdStopRate float64
	// how often to drop a report
	reportInterval time.Duration
	// how often to capture a trace
	captureInterval time.Duration
	// write reports to writer
	writer io.Writer
}

func (c *Config) SetDefault() {
	c.thresholdStartRate = 5000
	c.thresholdStopRate = 4000
	c.interval = 5 * time.Second
	c.reportInterval = 15 * time.Second
	c.captureInterval = 200 * time.Millisecond
	c.writer = os.Stdout
}

func (c *Config) NewTracker() *StackTracker {
	t := &StackTracker{
		config: c,
	}
	go t.intervalTester()
	return t
}

type StackTracker struct {
	config *Config

	count  int32

	tracking uint32
	tracesMutex  sync.RWMutex
	traceChannel chan string

	lastCaptureMutex sync.Mutex
	lastCapture      time.Time
}

func (t *StackTracker) TrackCall(now time.Time) {
	atomic.AddInt32(&t.count, 1)

	if t.isTracking() {
		doCapture := now.Sub(t.lastCapture) > t.config.captureInterval
		if doCapture {
			t.lastCaptureMutex.Lock()
			doCapture := now.Sub(t.lastCapture) > t.config.captureInterval
			if doCapture {
				t.lastCapture = now
			}
			t.lastCaptureMutex.Unlock()

			if doCapture {
				t.captureTrace()
			}
		}
	}
}

func (t *StackTracker) captureTrace() {
	var pc []uintptr = make([]uintptr, 64)
	l := runtime.Callers(2, pc)
	pc = pc[:l]
	key := fmt.Sprintf("%X", pc)
	// trim brackets from string of form "[0123 4567 89ab cdef]"
	key = key[1 : len(key)-1]

	t.tracesMutex.RLock()
	if t.traceChannel != nil {
		t.traceChannel <- key
	}
	t.tracesMutex.RUnlock()

}

func (t *StackTracker) intervalTester() {
	intervalStartTime := time.Now()
	intervalStartCount := int32(0)
	var finalizeReporter chan interface{}
	for {
		time.Sleep(t.config.interval)
		now := time.Now()
		deltaT := now.Sub(intervalStartTime)
		deltaC := atomic.LoadInt32(&t.count) - intervalStartCount
		var rate float64 = float64(deltaC) * float64(time.Second) / float64(deltaT)
		if !t.isTracking() {
			if rate > t.config.thresholdStartRate {
				finalizeReporter = t.startTracking()
			}
		} else {
			if rate < t.config.thresholdStopRate {
				t.stopTracking(finalizeReporter)
				finalizeReporter = nil
			}
		}

		intervalStartTime = now
		intervalStartCount = t.count
	}
}

func (t *StackTracker) isTracking() bool {
	return atomic.LoadUint32(&t.tracking) != 0
}

func (t *StackTracker) startTracking() chan interface{} {
	traceChannel := make(chan string, 128)
	finalizeReporter := make(chan interface{})
	go reportWriter(t.config, traceChannel, finalizeReporter)
	t.tracesMutex.Lock()
	t.traceChannel = traceChannel
	atomic.StoreUint32(&t.tracking, 1)
	t.tracesMutex.Unlock()
	return finalizeReporter
}

func (t *StackTracker) stopTracking(finalizeReporter chan interface{}) {
	t.tracesMutex.Lock()
	t.traceChannel = nil
	atomic.StoreUint32(&t.tracking, 0)
	t.tracesMutex.Unlock()
	close(finalizeReporter)
}

type traceMap map[string]uint32

func reportWriter(config *Config, traceChan chan string, final chan interface{}) {
	traces := make(traceMap)
	timer := time.After(config.reportInterval)
	for {
		select {
		case key := <-traceChan:
			traces[key] += 1
		case <-timer:
			tmp := traces
			traces = make(traceMap)
			timer = time.After(config.reportInterval)
			go writeReport(tmp, config.writer)
		case <-final:
		ReadAllTraces:
			for {
				select {
				case key := <-traceChan:
					traces[key] += 1
				default:
					break ReadAllTraces
				}
			}

			writeReport(traces, config.writer)
			return
		}
	}
}

func writeReport(traces traceMap, writer io.Writer) {
	var totalCalls = uint32(0)
	var traceSlice traceCountSlice = make(traceCountSlice, len(traces))
	traceSlice = traceSlice[0:0]
	for trace, count := range traces {
		traceSlice = append(traceSlice, traceCount{count, trace})
		totalCalls += count
	}

	sort.Sort(traceSlice)

	var buffer bytes.Buffer
	for _, p := range traceSlice {
		buffer.WriteString(fmt.Sprintf("Calls %d/%d\n", p.count, totalCalls))
		for _, i := range strings.Split(p.trace, " ") {
			pc, _ := strconv.ParseUint(i, 16, 64)
			f := runtime.FuncForPC(uintptr(pc))
			file, line := f.FileLine(uintptr(pc))
			buffer.WriteString(fmt.Sprintf("   %s:%d\n", file, line))
		}
	}

	writer.Write(buffer.Bytes())
}

// sort traces based on count
type traceCount struct {
	count uint32
	trace string
}
type traceCountSlice []traceCount

func (s traceCountSlice) Len() int           { return len(s) }
func (s traceCountSlice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s traceCountSlice) Less(i, j int) bool { return s[i].count > s[j].count }
