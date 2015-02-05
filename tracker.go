package tracker

import (
	"bytes"
	"io"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
	"strconv"
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
	// capture a trace for every n'th request
	captureMod int32
	// write reports to writer
	writer io.Writer
}

func (c *Config) SetDefault() {
	c.thresholdStartRate = 5000
	c.thresholdStopRate = 4000
	c.interval = 5 * time.Second
	c.reportInterval = 15 * time.Second
	c.captureMod = 1000
	c.writer = os.Stdout
}

func (c *Config) NewTracker() *StackTracker {
	return &StackTracker{
		config: c,
	}
}

type StackTracker struct {
	config             *Config
	intervalStartTime  time.Time
	lastReport         time.Time
	intervalStartCount int32
	count              int32
	traces             map[string]uint32
}

func (t *StackTracker) TrackCall(now time.Time) {
	if t.intervalStartTime.IsZero() {
		t.intervalStartTime = now
	}
	t.count += 1

	deltaT := now.Sub(t.intervalStartTime)
	deltaC := t.count - t.intervalStartCount

	if deltaT > t.config.interval {
		var rate float64 = float64(deltaC) * float64(time.Second) / float64(deltaT)
		if t.isTracking() {
			if rate < t.config.thresholdStopRate {
				t.stopTracking(now)
			}
		} else {
			if rate > t.config.thresholdStartRate {
				t.startTracking(now)
			}
		}

		t.intervalStartTime = now
		t.intervalStartCount = t.count
	}

	if t.isTracking() {
		if deltaC%t.config.captureMod == 0 {
			t.captureTrace()
		}
		if now.Sub(t.lastReport) < t.config.reportInterval {
			t.writeReport(now)
		}
	}
}

func (t *StackTracker) captureTrace() {
	var pc []uintptr = make ([]uintptr, 64)
	l := runtime.Callers(2, pc)
	pc = pc[:l]
	key := fmt.Sprintf("%X", pc)
	// trim brackets from string of form "[0123 4567 89ab cdef]"
	key = key[1:len(key)-1]
	t.traces[key]+= 1
}

func (t *StackTracker) isTracking() bool {
	return t.traces != nil
}
func (t *StackTracker) startTracking(now time.Time) {
	t.traces = make(map[string]uint32)
	t.lastReport = now
}
func (t *StackTracker) stopTracking(now time.Time) {
	t.writeReport(now)
	t.traces = nil
}


func (t *StackTracker) writeReport(now time.Time) {
	t.lastReport = now
	var buffer bytes.Buffer

	var totalCalls = uint32(0)
	var traces traceCountSlice = make(traceCountSlice, len(t.traces))
	traces = traces[0:0]
	for trace, count := range t.traces {
		traces = append(traces, traceCount{count, trace})
		totalCalls += count
	}

	sort.Sort(traces)

	for _, p := range traces {
		buffer.WriteString(fmt.Sprintf("Calls %d/%d\n", p.count, totalCalls))
		for _, i := range strings.Split(p.trace, " ") {
			pc, _ := strconv.ParseUint(i, 16, 64)
			f := runtime.FuncForPC(uintptr(pc))
			file, line := f.FileLine(uintptr(pc))
			buffer.WriteString(fmt.Sprintf("   %s:%d\n", file, line))
		}
	}

	t.config.writer.Write(buffer.Bytes())
}


// sort traces based on count
type traceCount struct{
	count uint32;
	trace string;
}
type traceCountSlice []traceCount
func (s traceCountSlice) Len() int {return len(s)}
func (s traceCountSlice) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s traceCountSlice) Less(i, j int) bool { return s[i].count > s[j].count }
