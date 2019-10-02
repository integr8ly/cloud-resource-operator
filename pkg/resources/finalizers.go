package resources

import (
	controllerruntime "sigs.k8s.io/controller-runtime"
)

func HasFinalizer(om *controllerruntime.ObjectMeta, finalizer string) bool {
	return Contains(om.GetFinalizers(), finalizer)
}

func AddFinalizer(om *controllerruntime.ObjectMeta, finalizer string) {
	if !HasFinalizer(om, finalizer) {
		om.SetFinalizers([]string{finalizer})
	}
}

func RemoveFinalizer(om *controllerruntime.ObjectMeta, finalizer string) {
	om.SetFinalizers(remove(om.GetFinalizers(), finalizer))
}

func Contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func remove(list []string, s string) []string {
	for i, v := range list {
		if v == s {
			list = append(list[:i], list[i+1:]...)
		}
	}
	return list
}
