package tracker

import (
    "testing"
)

func TestXXX(t *testing.T) {
    var c Config
    c.SetDefault()

    tracker := c.NewTracker()

    finalizeReporter := tracker.startTracking()
    tracker.captureTrace()

    var f = func () () {
        tracker.captureTrace()
    }

    for i := 0; i < 2; i +=1 {
        f()
    }

    close(finalizeReporter)

}