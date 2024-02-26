package agi

import (
	"errors"
)

// ErrHangup indicates the channel hung up during processing
var ErrHangup = errors.New("hangup")

// ErrTimeout indicates the get data command ends with timeout during processing
var ErrTimeout = errors.New("timeout")

// Err511CommandNotPermitted indicates we have received error 511 Command Not Permitted
var Err511CommandNotPermitted = errors.New("Command Not Permitted on a dead channel or intercept routine")

// Err511GenericError indicates we have received generic 511 error
var Err511GenericError = errors.New("Generic 511 Error")
