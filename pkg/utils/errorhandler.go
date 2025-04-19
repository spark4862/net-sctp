package utils

import (
	"fmt"
	"log"
	"runtime"
)

func ErrorHandler(err error, depth int) bool {
	if err != nil {
		_, file, line, ok := runtime.Caller(depth)
		if !ok {
			panic("error when get caller")
		}
		err = fmt.Errorf("%s err at %d: %w", file, line, err)
		log.Println(err)
		return true
	}
	return false
}
