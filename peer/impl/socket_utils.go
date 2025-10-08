package impl

import (
	"log"
	"os"

	"go.dedis.ch/cs438/transport"
	"golang.org/x/xerrors"
)

// shouldStop checks if the node should stop and returns true if it should
func (n *node) shouldStop() bool {
	select {
	case <-n.stopCh:
		return true
	default:
		return false
	}
}

// handleSocketError handles socket errors and returns true if the node should stop
func (n *node) handleSocketError(err error, errorCount *int) bool {
	if xerrors.Is(err, transport.TimeoutError(0)) {
		return false
	}
	
	*errorCount++
	if *errorCount > 10 {
		n.stopNodeOnHighErrorCount()
		return true
	}
	
	n.logSocketError(*errorCount, err)
	return false
}

// stopNodeOnHighErrorCount stops the node when error count is too high
func (n *node) stopNodeOnHighErrorCount() {
	go func() {
		_ = n.Stop()
	}()
	if os.Getenv("GLOG") != "no" {
		log.Printf("High error count, stopping node")
	}
}

// logSocketError logs socket errors if appropriate
func (n *node) logSocketError(errorCount int, err error) {
	if os.Getenv("GLOG") != "no" && errorCount <= 5 {
		log.Printf("Socket error (attempt %d): %v", errorCount, err)
	}
}

// resetErrorCount resets the error count when a successful packet is received
func (n *node) resetErrorCount(errorCount *int) {
	if *errorCount > 0 {
		*errorCount = 0
	}
}
