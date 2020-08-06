package concurrency

import (
	"github.com/sirupsen/logrus"
	"sync"
)

func ConcurrentFuncs(functions []func()) {
	logger := logrus.WithFields(logrus.Fields{"action": "concurrentFuncs"})

	var waitGroup sync.WaitGroup
	waitGroup.Add(len(functions))
	defer waitGroup.Wait()

	for index, function := range functions {
		logger.Infof("running noise no %d", index)
		go func(copy func()) {
			defer waitGroup.Done()
			copy()
		}(function)
	}
}
