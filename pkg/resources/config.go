package resources

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	errorUtil "github.com/pkg/errors"
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

func GeneratePassword() (string, error) {
	generatedPassword, err := uuid.NewRandom()
	if err != nil {
		return "", errorUtil.Wrap(err, "error generating password")
	}
	return strings.Replace(generatedPassword.String(), "-", "", 10), nil
}
