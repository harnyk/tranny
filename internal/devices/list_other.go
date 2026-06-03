//go:build !darwin && !linux

package devices

import (
	"fmt"
	"runtime"
)

func ListInputDevices(_ string) ([]InputDevice, error) {
	return nil, fmt.Errorf("tran devices list is not supported on %s", runtime.GOOS)
}
