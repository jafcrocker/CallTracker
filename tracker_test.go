package tracker

import (
    "fmt"
    "testing"
    "time"
)

func TestXXX(t *testing.T) {
    var c Config
    c.SetDefault()

    tracker := c.NewTracker()

    tracker.startTracking(time.Now())
    tracker.captureTrace()

    fmt.Printf("%+v\n", tracker)


    var f = func () () {
        tracker.captureTrace()
    }

    for i := 0; i < 2; i +=1 {
        f()
    }

    tracker.writeReport(time.Now())

}