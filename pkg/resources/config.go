package resources

import (
	"os"
	"strconv"
	"time"
)

const DefaulReconcileTime = 300

// returns envar for reconcile time else returns default time
func GetReconcileTime() time.Duration {
	recTime, exist := os.LookupEnv("RECTIME")
	if exist {
		rt, err := strconv.ParseInt(recTime, 10, 64)
		if err != nil {
			return time.Duration(DefaulReconcileTime)
		}
		return time.Duration(rt)
	}
	return time.Duration(DefaulReconcileTime)
}
