package utils

import "context"

func CancelContext(cancel context.CancelFunc) {
	if cancel != nil {
		cancel()
	}
}
