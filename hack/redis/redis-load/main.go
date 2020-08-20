package main

import (
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/sirupsen/logrus"
	flag "github.com/spf13/pflag"
)

type LoadOptions struct {
	host        string
	port        int
	entries     int
	connections int
}

var (
	host        string
	port        int
	connections int
	entries     int
	loadData    bool
	loadCPU     bool
)

func loadDataFunc(options *LoadOptions) func() {
	return func() {
		logger := logrus.WithFields(logrus.Fields{"action": "load-data"})
		connectionString := fmt.Sprintf("%s:%v", options.host, options.port)
		logger.Infof("trying to connect to %s", connectionString)
		conn, err := redis.Dial("tcp", connectionString)
		if err != nil {
			logger.Fatal(err)
		}
		logger.Infof("connection made to %s", connectionString)
		defer conn.Close()

		count := 0
		for count < options.entries {
			entry := fmt.Sprintf("stuff:%s", randomString(42))

			logger.Infof("adding stuff entry %s", entry)
			_, err = conn.Do("HMSET", entry, "stuff", "all the stuff", "n thangs", "mo stuff", "no stuff", 42)
			if err != nil {
				logger.Fatalf("%+v", err)
			}

			count++
		}
	}
}

func loadCPUFunc(options *LoadOptions) func() {
	return func() {
		logger := logrus.WithFields(logrus.Fields{"action": "load-cpu. Hit ctrl + C to cancel"})
		connectionString := fmt.Sprintf("%s:%v", options.host, options.port)
		logger.Infof("trying to connect to %s", connectionString)
		conn, err := redis.Dial("tcp", connectionString)
		if err != nil {
			logger.Fatal(err)
		}
		logger.Infof("connection made to %s", connectionString)
		defer conn.Close()

		for {
			// https://redis.io/commands/keys
			// find all keys matching the a patten. O(n) where n is number of keys in Redis
			// it runs fast but it's CPU intensive
			logger.Info("Running keys command")
			_, err := conn.Do("KEYS", "*FOO*")
			if err != nil {
				logger.Fatalf("%+v", err)
			}

			logger.Info("keys command successful")
		}
	}
}

func main() {
	logger := logrus.WithFields(logrus.Fields{"action": "running redis load"})
	logger.Info("starting redis load pre-reqs")

	options, err := prerequisites()
	if err != nil {
		logger.Fatalf("prerequisites failed %v", err)
	}
	logger.Info("prerequisites passed")
	if loadData == true {
		logger.Info("starting load-data action")
		concurrentFuncs(loadDataFunc(options), options.connections)
	}
	if loadCPU == true {
		logger.Info("starting load-cpu action")
		concurrentFuncs(loadCPUFunc(options), options.connections)
	}
	logger.Info("Done")
}

func prerequisites() (*LoadOptions, error) {
	logger := logrus.WithFields(logrus.Fields{"action": "running prerequisites"})
	logger.Info("starting redis load pre-reqs")

	flag.StringVarP(&host, "host", "h", "", "The hostname of the redis instance (Required)")
	flag.IntVarP(&connections, "connections", "c", 100, "The number of simultaneous connections made to the redis server")
	flag.IntVarP(&entries, "num-requests", "n", 1000, "The number of requests that will be created by each connection")
	flag.IntVarP(&port, "port", "p", 6379, "the port of the redis instance")
	flag.BoolVar(&loadData, "load-data", false, "if true, bulk inserts data into redis. Number of insertions is connections * num-requests")
	flag.BoolVar(&loadCPU, "load-cpu", false, "if true, intensive redis KEYS queries will be run to spike CPU utilization. This runs indefinitely. Hit CTRL+C to cancel.")
	flag.Parse()

	if host == "" {
		logger.Error("host value missing missing")
		flag.PrintDefaults()
		return nil, errors.New("host option missing")
	}

	return &LoadOptions{
		host:        host,
		port:        port,
		entries:     entries,
		connections: connections,
	}, nil
}

func concurrentFuncs(funcToCall func(), callCount int) {
	logger := logrus.WithFields(logrus.Fields{"action": "concurrentFuncs"})

	var waitGroup sync.WaitGroup
	waitGroup.Add(callCount)
	defer waitGroup.Wait()

	count := 0
	for count < callCount {
		logger.Infof("running func no %d", count)
		go func(copy func()) {
			defer waitGroup.Done()
			copy()
		}(funcToCall)
		count++
	}
}

func stringFromBytes(length int) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = byte(65 + rand.Intn(90-65))
	}
	return string(b)
}

func randomString(length int) string {
	rand.Seed(time.Now().UTC().UnixNano())
	return stringFromBytes(length)
}
