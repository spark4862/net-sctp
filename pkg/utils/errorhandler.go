package utils

import (
	"fmt"
	"log"
	"runtime"
)

func ErrorHandler(err error) bool {
	if err != nil {
		_, file, line, ok := runtime.Caller(0)
		if !ok {
			panic("error when get current func")
		}
		err = fmt.Errorf("%s err at %d: %w", file, line, err)
		log.Println(err)
		return true
	}
	return false
}
