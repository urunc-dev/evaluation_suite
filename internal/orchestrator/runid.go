package orchestrator

import (
	"fmt"
	"time"
)

func NewRunID(t time.Time) string {
	return fmt.Sprintf("run-%s", t.UTC().Format("20060102T150405Z"))
}
