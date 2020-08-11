package main

import (
	"errors"
	"fmt"
	"github.com/gomodule/redigo/redis"
	"github.com/integr8ly/cloud-resource-operator/hack/redis/redis-load/pkg/concurrency"
	croString "github.com/integr8ly/cloud-resource-operator/hack/redis/redis-load/pkg/string"
	"github.com/sirupsen/logrus"
	"os"
	"strconv"
)

type LoadOptions struct {
	host    string
	port    string
	entries int
	users   int
}

func main() {
	logger := logrus.WithFields(logrus.Fields{"action": "running redis load"})
	logger.Info("starting redis load pre-reqs")

	options, err := prerequisites()
	if err != nil {
		logger.Fatalf("prerequisites failed %v", err)
	}
	logger.Info("prerequisites passed")

	makeNoise := func() {
		logger := logrus.WithFields(logrus.Fields{"action": "makeNoise"})
		connectionString := fmt.Sprintf("%s:%s", options.host, options.port)
		logger.Infof("trying to connect to %s", connectionString)
		conn, err := redis.Dial("tcp", connectionString)
		if err != nil {
			logger.Fatal(err)
		}
		logger.Infof("connection made to %s", connectionString)
		defer conn.Close()

		count := 0
		for count < options.entries {
			entry := fmt.Sprintf("stuff:%s", croString.RandomString(42))

			logger.Infof("adding stuff entry %s", entry)
			_, err = conn.Do("HMSET", entry, "stuff", "all the stuff", "n thangs", "mo stuff", "no stuff", 42)
			if err != nil {
				logger.Fatal(err)
				logrus.Info("stuff added")
			}

			count++
		}
	}

	count := 0
	var funcs []func()
	for count < options.users {
		funcs = append(funcs, makeNoise)
		count++
	}

	concurrency.ConcurrentFuncs(funcs)
	logger.Info("no more noise")
}

func prerequisites() (*LoadOptions, error) {
	logger := logrus.WithFields(logrus.Fields{"action": "running prerequisites"})
	logger.Info("starting redis load pre-reqs")

	host, hostExists := os.LookupEnv("HOST")
	port, portExists := os.LookupEnv("PORT")
	entries, entriesExists := os.LookupEnv("ENTRIES")
	users, usersExist := os.LookupEnv("USERS")

	if !hostExists || !portExists || !entriesExists || !usersExist {
		logger.Error("host, port, entries or users env missing")
		return nil, errors.New("host, port, entries or users env missing")
	}

	noEntries, err := strconv.Atoi(entries)
	if err != nil {
		return nil, fmt.Errorf("can not parse number of entries to int %w", err)
	}

	noUsers, err := strconv.Atoi(users)
	if err != nil {
		return nil, fmt.Errorf("can not parse number of users to int %w", err)
	}

	return &LoadOptions{
		host:    host,
		port:    port,
		entries: noEntries,
		users:   noUsers,
	}, nil
}
