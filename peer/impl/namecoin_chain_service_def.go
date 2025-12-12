package impl

import "fmt"

type ChainFollowError struct {
	Msg string
}

func (e *ChainFollowError) Error() string {
	return fmt.Sprintf("validation failed: %s", e.Msg)
}
